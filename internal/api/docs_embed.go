package api

import "embed"

// DocsFS embeds the OpenAPI documentation files (openapi.yaml, scalar.html, scalar.js).
// Files are symlinked into internal/api/docs/ to allow embedding without ".." paths.
//
//go:embed docs/openapi.yaml docs/scalar.html docs/scalar.js
var DocsFS embed.FS
