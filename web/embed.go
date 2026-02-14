// Package web provides the embedded SvelteKit frontend build output.
// The build/ directory is populated by "npm run build" in web/.
// A .gitkeep placeholder ensures go:embed succeeds even without a build.
package web

import "embed"

// BuildFS contains the SvelteKit build output embedded at compile time.
// Use fs.Sub(BuildFS, "build") to get the root of the static files.
//
//go:embed all:build
var BuildFS embed.FS
