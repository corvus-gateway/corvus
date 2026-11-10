package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"corvus/control/internal/recovery"
	"corvus/control/internal/store"
)

const resetAdminUsage = `usage: corvus-control reset-admin [--email ADDRESS] [--disable-mfa]

Gives an administrator a new random password, ends their sessions and deletes the API
tokens they created. Run it inside the container, for example:

  docker compose exec corvus corvus-control reset-admin

  --email ADDRESS   the account to reset (default: the only active administrator)
  --disable-mfa     also turn off two-step verification for the account
`

// resetAdmin implements "corvus-control reset-admin". It works while Corvus is running.
func resetAdmin(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("reset-admin", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { fmt.Fprint(stderr, resetAdminUsage) }
	email := fs.String("email", "", "")
	disableMFA := fs.Bool("disable-mfa", false, "")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 0 {
		fs.Usage()
		return 2
	}
	dataDir := env("CORVUS_DATA_DIR", "/data")
	target := env("CORVUS_DB_URL", filepath.Join(dataDir, "corvus.db"))
	if !isPostgres(target) {
		if _, err := os.Stat(target); err != nil {
			fmt.Fprintf(stderr, "reset-admin: no database at %s (is this the Corvus container?)\n", target)
			return 1
		}
	}
	// docker exec runs as root: act as the owner of the data so that files SQLite
	// creates next to the database stay writable for Corvus.
	if err := dropToOwner(dataDir); err != nil {
		fmt.Fprintf(stderr, "reset-admin: %v\n", err)
		return 1
	}
	st, err := store.Open(target)
	if err != nil {
		fmt.Fprintf(stderr, "reset-admin: open the database: %v\n", err)
		return 1
	}
	defer st.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	res, err := recovery.ResetAdmin(ctx, st, *email, *disableMFA)
	if err != nil {
		fmt.Fprintf(stderr, "reset-admin: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Account:      %s\nNew password: %s\n", res.Email, res.Password)
	if res.MFADisabled {
		fmt.Fprintln(stdout, "Two-step verification is off; set it up again on the Account page.")
	}
	if res.TokensDeleted > 0 {
		fmt.Fprintf(stdout, "Deleted %d API token(s) created by this account.\n", res.TokensDeleted)
	}
	fmt.Fprintln(stdout, "Sign in and change the password on the Account page.")
	return 0
}

func isPostgres(target string) bool {
	return strings.HasPrefix(target, "postgres://") || strings.HasPrefix(target, "postgresql://")
}
