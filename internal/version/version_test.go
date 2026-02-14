package version

import "testing"

func TestInfo(t *testing.T) {
	info := Info()

	requiredKeys := []string{"version", "commit", "buildDate", "branch"}
	for _, key := range requiredKeys {
		if _, ok := info[key]; !ok {
			t.Errorf("Info() missing key %q", key)
		}
	}

	if info["version"] != Version {
		t.Errorf("Info()[version] = %q, want %q", info["version"], Version)
	}
	if info["commit"] != Commit {
		t.Errorf("Info()[commit] = %q, want %q", info["commit"], Commit)
	}
	if info["buildDate"] != BuildDate {
		t.Errorf("Info()[buildDate] = %q, want %q", info["buildDate"], BuildDate)
	}
	if info["branch"] != Branch {
		t.Errorf("Info()[branch] = %q, want %q", info["branch"], Branch)
	}
}

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
