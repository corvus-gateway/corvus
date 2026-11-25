package validate

import (
	"regexp"
	"strings"

	"corvus/control/internal/store"
)

// Sign-in users and the login page in front of proxy hosts (#294).

const (
	DefaultSessionHours = 168 // 7 days
	MaxSessionHours     = 720 // 30 days
	MaxExcludePaths     = 20
	MinPINLength        = 4
	MinSigninPassword   = 8
	MaxSecretBytes      = 72 // bcrypt
)

var (
	signinNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._@-]{0,63}$`)
	groupNameRe  = regexp.MustCompile(`^[\p{L}\p{N}][\p{L}\p{N} ._-]{0,63}$`)
)

// hostLogin normalizes and validates a proxy host's login page settings. A disabled
// login page keeps its settings (so it can be turned on again) but is not checked.
func hostLogin(errs *Errors, l *store.HostLogin) {
	if l.GroupIDs == nil {
		l.GroupIDs = []int64{}
	}
	if l.ExcludePaths == nil {
		l.ExcludePaths = []string{}
	}
	if l.SessionHours == 0 {
		l.SessionHours = DefaultSessionHours
	}
	if !l.Enabled {
		return
	}
	switch l.Mode {
	case store.LoginPIN:
		if l.PINHash == "" {
			errs.add("login.pin", "required", "set a PIN or password")
		}
		l.PassUser = false // a PIN has no user name to pass on
	case store.LoginUsers:
		if len(l.GroupIDs) == 0 {
			errs.add("login.group_ids", "required", "allow at least one group")
		}
	default:
		errs.add("login.mode", "invalid", "mode must be pin or users")
	}
	if l.SessionHours < 1 || l.SessionHours > MaxSessionHours {
		errs.add("login.session_hours", "invalid", "a sign-in must last 1-%d hours", MaxSessionHours)
	}
	seen := map[int64]bool{}
	ids := []int64{}
	for _, id := range l.GroupIDs {
		if id > 0 && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	l.GroupIDs = ids
	if len(l.ExcludePaths) > MaxExcludePaths {
		errs.add("login.exclude_paths", "too_many", "at most %d excluded paths", MaxExcludePaths)
		return
	}
	paths := []string{}
	dup := map[string]bool{}
	for _, p := range l.ExcludePaths {
		p = strings.TrimSpace(p)
		switch {
		case p == "":
			continue
		case p == "/":
			errs.add("login.exclude_paths", "everything", "excluding / would turn the login page off; disable it instead")
		case !pathRe.MatchString(p) || strings.Contains(p, "//") || strings.Contains(p, "/../"):
			errs.add("login.exclude_paths", "invalid", "%q: a path must start with / and contain only letters, digits and ._~/-", p)
		case strings.HasPrefix(p, "/__corvus"):
			errs.add("login.exclude_paths", "reserved", "/__corvus is reserved")
		case dup[p]:
		default:
			dup[p] = true
			paths = append(paths, p)
		}
	}
	l.ExcludePaths = paths
}

// PIN checks a new shared PIN or password of a login page.
func PIN(pin string) error {
	var errs Errors
	switch {
	case len([]rune(pin)) < MinPINLength:
		errs.add("login.pin", "weak", "a PIN must have at least %d characters", MinPINLength)
	case len(pin) > MaxSecretBytes:
		errs.add("login.pin", "too_long", "a PIN must be at most %d bytes", MaxSecretBytes)
	}
	return errs.OrNil()
}

// SigninUser normalizes and validates a sign-in user. password is the new password
// ("" = unchanged; required when the user has none yet).
func SigninUser(u *store.SigninUser, password string) error {
	var errs Errors
	u.Username = strings.ToLower(strings.TrimSpace(u.Username))
	if !signinNameRe.MatchString(u.Username) {
		errs.add("username", "invalid", "a user name is 1-64 letters, digits and ._@- (starting with a letter or digit)")
	}
	switch {
	case password == "" && u.PasswordHash == "":
		errs.add("password", "required", "a password is required")
	case password != "" && len(password) < MinSigninPassword:
		errs.add("password", "weak", "a password must have at least %d characters", MinSigninPassword)
	case len(password) > MaxSecretBytes:
		errs.add("password", "too_long", "a password must be at most %d bytes", MaxSecretBytes)
	}
	notes(&errs, &u.Note)
	if u.GroupIDs == nil {
		u.GroupIDs = []int64{}
	}
	return errs.OrNil()
}

// SigninGroup normalizes and validates a group of sign-in users.
func SigninGroup(g *store.SigninGroup) error {
	var errs Errors
	g.Name = strings.TrimSpace(g.Name)
	if !groupNameRe.MatchString(g.Name) {
		errs.add("name", "invalid", "a group name is 1-64 letters, digits, spaces and ._-")
	}
	if g.UserIDs == nil {
		g.UserIDs = []int64{}
	}
	return errs.OrNil()
}
