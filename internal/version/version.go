package version

// Build information - populated at build time via ldflags
var (
	// Version is the git tag, or "dev" if not building from a tag
	Version = "dev"
	// Commit is the git commit hash
	Commit = "unknown"
	// BuildDate is when the binary was built
	BuildDate = "unknown"
	// Branch is the git branch name
	Branch = "unknown"
)

// Info returns build information as a map
func Info() map[string]string {
	return map[string]string{
		"version":   Version,
		"commit":    Commit,
		"buildDate": BuildDate,
		"branch":    Branch,
	}
}
