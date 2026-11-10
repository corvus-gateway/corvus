//go:build !linux

package main

// dropToOwner is a no-op outside Linux (development builds).
func dropToOwner(string) error { return nil }
