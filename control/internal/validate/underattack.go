package validate

import "corvus/control/internal/store"

// "Under attack" mode defaults (see store.UnderAttackConfig).
const (
	DefaultUnderAttackMode       = "js"
	DefaultUnderAttackDifficulty = 16
	DefaultUnderAttackTTL        = 3600 // 1 h
	DefaultUnderAttackBind       = "ip"
	DefaultUnderAttackRPS        = 200
	DefaultUnderAttackBlockedPct = 50
	DefaultUnderAttackWindow     = 30
	DefaultUnderAttackDuration   = 900 // 15 min

	MinUnderAttackWindow   = 10
	MaxUnderAttackWindow   = 600
	MinUnderAttackDuration = 60
	MaxUnderAttackDuration = 86400
)

// UnderAttackDefaults fills unset (zero / nil) parameters with the defaults. Hosts
// stored before the mode existed have an empty configuration.
func UnderAttackDefaults(c *store.UnderAttackConfig) {
	if c.Mode == "" {
		c.Mode = DefaultUnderAttackMode
	}
	if c.Difficulty == 0 {
		c.Difficulty = DefaultUnderAttackDifficulty
	}
	if c.TTL == 0 {
		c.TTL = DefaultUnderAttackTTL
	}
	if c.Bind == "" {
		c.Bind = DefaultUnderAttackBind
	}
	if c.RPS == 0 {
		c.RPS = DefaultUnderAttackRPS
	}
	if c.BlockedPct == nil {
		v := float64(DefaultUnderAttackBlockedPct)
		c.BlockedPct = &v
	}
	if c.WindowS == 0 {
		c.WindowS = DefaultUnderAttackWindow
	}
	if c.DurationS == 0 {
		c.DurationS = DefaultUnderAttackDuration
	}
}

// underAttack normalises the host's under-attack mode and parameters. Parameters are
// validated whatever the mode, so switching modes later never meets a stored value
// the gateway cannot use.
func underAttack(errs *Errors, h *store.ProxyHost) {
	switch h.UnderAttack {
	case "":
		h.UnderAttack = "off"
	case "off", "on", "auto":
	default:
		errs.add("under_attack", "invalid", "under attack mode must be off, on or auto")
	}
	c := &h.UnderAttackConfig
	UnderAttackDefaults(c)
	if c.Mode != "js" && c.Mode != "cookie" {
		errs.add("under_attack_config.mode", "invalid", "challenge type must be js or cookie")
	}
	if c.Difficulty < MinChallengeDifficulty || c.Difficulty > MaxChallengeDifficulty {
		errs.add("under_attack_config.difficulty", "invalid", "difficulty must be %d-%d bits", MinChallengeDifficulty, MaxChallengeDifficulty)
	}
	if c.TTL < MinChallengeTTL || c.TTL > MaxChallengeTTL {
		errs.add("under_attack_config.ttl_s", "invalid", "pass lifetime must be %d-%d seconds (5 minutes to 7 days)", MinChallengeTTL, MaxChallengeTTL)
	}
	if c.Bind != "ip" && c.Bind != "ip_ua" {
		errs.add("under_attack_config.bind", "invalid", "binding must be ip or ip_ua")
	}
	if c.RPS < 1 || c.RPS > 1e6 {
		errs.add("under_attack_config.rps", "invalid", "requests per second must be 1-1000000")
	}
	if *c.BlockedPct < 0 || *c.BlockedPct > 100 {
		errs.add("under_attack_config.blocked_pct", "invalid", "blocked share must be 0-100 percent (0 = not used)")
	}
	if c.WindowS < MinUnderAttackWindow || c.WindowS > MaxUnderAttackWindow {
		errs.add("under_attack_config.window_s", "invalid", "measuring window must be %d-%d seconds", MinUnderAttackWindow, MaxUnderAttackWindow)
	}
	if c.DurationS < MinUnderAttackDuration || c.DurationS > MaxUnderAttackDuration {
		errs.add("under_attack_config.duration_s", "invalid", "activation period must be %d-%d seconds", MinUnderAttackDuration, MaxUnderAttackDuration)
	}
}
