package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"strings"
	"time"

	"corvus/control/internal/gateway"
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

// startRedis starts the embedded Redis without making it a reason to stop: until it is
// up, access queues apply their fail policy (open or closed) and the console reports
// Redis as down. If the process could not even be started (its supervisor only runs once
// it did), starting is retried with a growing delay.
func startRedis(ctx context.Context, rd *gateway.Redis) {
	err := rd.Start(ctx)
	if err == nil {
		return
	}
	slog.Error("embedded redis unavailable; access queues apply their fail policy until it is up", "err", err)
	if redisSpawned(err) {
		return // it runs but did not answer in time: its supervisor takes care of it
	}
	go func() {
		backoff := 5 * time.Second
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			err := rd.Start(ctx)
			if err == nil || redisSpawned(err) {
				if err == nil {
					slog.Info("embedded redis started after retrying")
				}
				return
			}
			slog.Error("embedded redis still unavailable", "err", err, "retry_in", backoff)
			backoff = min(backoff*2, 2*time.Minute)
		}
	}()
}

// redisSpawned reports whether a Redis.Start error came after the process was started
// (waiting for it to accept connections), rather than from starting it.
func redisSpawned(err error) bool {
	var oe *net.OpError
	return errors.As(err, &oe) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}
