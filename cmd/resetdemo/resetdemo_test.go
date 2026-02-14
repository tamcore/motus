package resetdemo

import (
	"testing"
)

func TestNewCmd(t *testing.T) {
	cmd := NewCmd()
	if cmd == nil {
		t.Fatal("NewCmd returned nil")
	}
	if cmd.Use != "reset-demo" {
		t.Errorf("Use = %q, want %q", cmd.Use, "reset-demo")
	}
	if cmd.Run == nil {
		t.Error("Run should be set")
	}
	if cmd.Flags().Lookup("db-url") == nil {
		t.Error("flag 'db-url' not registered")
	}
}
