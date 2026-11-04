package npmimport

import (
	"context"
	"testing"
)

func TestOpenMySQLValidates(t *testing.T) {
	if _, err := open(context.Background(), Source{MySQL: &MySQL{Host: "db"}}); err == nil {
		t.Fatal("missing user/database accepted")
	}
	if _, err := open(context.Background(), Source{MySQL: &MySQL{Host: "db", User: "npm", Database: "npm", Port: 70000}}); err == nil {
		t.Fatal("invalid port accepted")
	}
}
