package api_test

import (
	"testing"

	"github.com/tamcore/motus/internal/api"
)

func TestDocsFS(t *testing.T) {
	for _, name := range []string{
		"docs/openapi.yaml",
		"docs/scalar.html",
		"docs/scalar.js",
	} {
		if _, err := api.DocsFS.Open(name); err != nil {
			t.Errorf("DocsFS missing %s: %v", name, err)
		}
	}
}
