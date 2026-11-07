package live

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Commands the access queue needs: the atomic script (EVALSHA/EVAL) and the commands
// it runs, DEL for the test-only reset endpoint and for removed queues, and connection housekeeping. Redis 7
// checks ACLs for commands run inside scripts too, so this list is the whole surface.
var aclCommands = []string{
	"evalsha", "eval", "time",
	"zadd", "zcard", "zpopmin", "zrangebyscore", "zrank", "zrem", "zremrangebyscore", "zscore",
	"hmget", "hset", "incr", "pexpire", "del",
	"ping", "hello", "client|setinfo",
}

// ACLFile renders a Redis ACL file (H12): the default user is disabled, the gateway
// user may only touch access-queue keys with the commands above, and "health" may only
// PING (for the container healthcheck). Only a SHA-256 of the password is stored.
func ACLFile(user, password string) string {
	sum := sha256.Sum256([]byte(password))
	cmds := make([]string, len(aclCommands))
	for i, c := range aclCommands {
		cmds[i] = "+" + c
	}
	return fmt.Sprintf("user default off resetkeys resetchannels -@all\n"+
		"user %s on #%s resetkeys ~corvus:wr:* resetchannels -@all %s\n"+
		"user health on nopass resetkeys resetchannels -@all +ping\n",
		user, hex.EncodeToString(sum[:]), strings.Join(cmds, " "))
}

// WriteACL atomically writes the ACL file for the Redis container (shared volume).
func WriteACL(path, user, password string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	// Readable by the Redis container's user; contains only a password hash.
	if err := os.WriteFile(tmp, []byte(ACLFile(user, password)), 0o644); err != nil {
		return err
	}
	_ = os.Chmod(tmp, 0o644)
	return os.Rename(tmp, path)
}
