// Package docs embeds the OpenAPI spec and the Scalar API reference page.
package docs

import "embed"

//go:embed openapi.yaml scalar.html scalar.js
var FS embed.FS
