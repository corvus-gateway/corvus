//go:build linux

package main

import (
	"fmt"
	"os"
	"syscall"
)

// dropToOwner switches a root process to the owner of dir (the data directory).
func dropToOwner(dir string) error {
	if os.Geteuid() != 0 {
		return nil
	}
	fi, err := os.Stat(dir)
	if err != nil {
		return err
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok || st.Uid == 0 {
		return nil
	}
	if err := syscall.Setgroups(nil); err != nil {
		return fmt.Errorf("drop supplementary groups: %w", err)
	}
	if err := syscall.Setgid(int(st.Gid)); err != nil {
		return fmt.Errorf("switch to group %d: %w", st.Gid, err)
	}
	if err := syscall.Setuid(int(st.Uid)); err != nil {
		return fmt.Errorf("switch to user %d: %w", st.Uid, err)
	}
	return nil
}
