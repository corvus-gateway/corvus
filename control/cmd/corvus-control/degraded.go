package main

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"corvus/control/internal/store"
)

// fallbackDeployer is the part of deploy.Deployer the boot-failure policy uses.
type fallbackDeployer interface {
	Fallback(ctx context.Context, skipLive bool, reason string) (*store.ConfigVersion, error)
	SafeMode(ctx context.Context, reason string) (*store.ConfigVersion, error)
}

// gatewayBootFailure is the gateway supervisor's policy for a gateway that exits right
// after starting (for example bind() on a port that another program holds; `openresty
// -t` does not open sockets, so such a configuration deploys fine and fails at boot):
//   - 1st and 2nd failure: retry the same configuration (a port held briefly, e.g. by a
//     container that starts at the same time, gets two more tries);
//   - 3rd: fall back to the newest earlier configuration that passes the test, recorded
//     as a rollback (Fallback, which ends in safe mode when none is left);
//   - 4th: safe mode (default server only), unless the fallback already was;
//   - later: keep retrying with backoff; the console stays available to fix the settings.
//
// A configuration deployed from the console meanwhile starts the count again.
func gatewayBootFailure(ctx context.Context, dep fallbackDeployer) func(failures int) bool {
	safe := false
	return func(failures int) bool {
		var v *store.ConfigVersion
		var err error
		switch {
		case failures < 3:
			safe = false
			return false
		case failures == 3:
			v, err = dep.Fallback(ctx, true, "the gateway did not start with the active configuration")
		case failures == 4 && !safe:
			v, err = dep.SafeMode(ctx, "the gateway did not start with an earlier configuration either")
		default:
			return false
		}
		if err != nil {
			slog.Error("gateway fallback failed", "err", err)
			return false
		}
		safe = strings.HasPrefix(v.Message, "safe mode")
		return true
	}
}

// redisStarter is the part of gateway.Redis that startRedis uses.
type redisStarter interface {
	Start(ctx context.Context) error
	Supervised() bool
}

// redisRetryFirst and redisRetryMax bound the delay between attempts to start Redis.
var redisRetryFirst, redisRetryMax = 5 * time.Second, 2 * time.Minute

// startRedis starts the embedded Redis without making it a reason to stop: until it is
// up, access queues apply their fail policy (open or closed) and the console reports
// Redis as down. A failed Start leaves nothing running (the process it started is
// ended, whatever the error), and the supervisor that restarts Redis only runs once a
// start succeeded, so starting is retried with a growing delay until then.
func startRedis(ctx context.Context, rd redisStarter) {
	err := rd.Start(ctx)
	if err == nil {
		return
	}
	slog.Error("embedded redis unavailable; access queues apply their fail policy until it is up", "err", err)
	go func() {
		backoff := redisRetryFirst
		for !rd.Supervised() {
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			err := rd.Start(ctx)
			if err == nil {
				slog.Info("embedded redis started after retrying")
				return
			}
			if ctx.Err() != nil {
				return
			}
			slog.Error("embedded redis still unavailable", "err", err, "retry_in", backoff)
			backoff = min(backoff*2, redisRetryMax)
		}
	}()
}
