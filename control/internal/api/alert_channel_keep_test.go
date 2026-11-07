package api

import (
	"context"
	"encoding/json"
	"testing"

	"corvus/control/internal/alerts"
	"corvus/control/internal/seal"
	"corvus/control/internal/store"
)

// Switching a Mattermost channel on or off (the list toggle sends no target fields)
// keeps its username and channel overrides.
func TestAlertChannelToggleKeepsMattermostOverrides(t *testing.T) {
	e := newTestEnv(t)
	e.s.DataKey = make([]byte, 32)
	ctx := context.Background()
	c := &store.AlertChannel{}
	if err := e.s.buildChannel(channelInput{Name: "mm", Kind: "mattermost", URL: "https://chat.example.com/hooks/abc", Username: ptr("Corvus"), Channel: ptr("ops")}, c); err != nil {
		t.Fatal(err)
	}
	off := false
	if err := e.s.buildChannel(channelInput{Name: "mm", Kind: "mattermost", Enabled: &off}, c); err != nil {
		t.Fatal(err)
	}
	raw, err := seal.Open(e.s.DataKey, c.Target)
	if err != nil {
		t.Fatal(err)
	}
	var tg alerts.Target
	_ = json.Unmarshal([]byte(raw), &tg)
	if tg.Username != "Corvus" || tg.Channel != "ops" || c.Enabled {
		t.Fatalf("target %+v enabled %v", tg, c.Enabled)
	}
	_ = ctx
}

func ptr(s string) *string { return &s }
