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
