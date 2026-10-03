package api

import "embed"

// DocsFS embeds the OpenAPI documentation files (openapi.yaml, scalar.html, scalar.js).
// The files are copies of docs/, synced by `make generate` (go:embed cannot use "..").
//
//go:embed docs/openapi.yaml docs/scalar.html docs/scalar.js
var DocsFS embed.FS
