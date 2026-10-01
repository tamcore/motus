package version

import "testing"

func TestDefaults(t *testing.T) {
	if Version != "dev" {
		t.Errorf("Version default = %q, want %q", Version, "dev")
	}
	if Commit != "unknown" {
		t.Errorf("Commit default = %q, want %q", Commit, "unknown")
	}
	if BuildDate != "unknown" {
		t.Errorf("BuildDate default = %q, want %q", BuildDate, "unknown")
	}
	if Branch != "unknown" {
		t.Errorf("Branch default = %q, want %q", Branch, "unknown")
	}
}
