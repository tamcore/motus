package api_test

import (
	"testing"

	"github.com/tamcore/motus/docs"
)

func TestDocsFS(t *testing.T) {
	for _, name := range []string{"openapi.yaml", "scalar.html", "scalar.js"} {
		if _, err := docs.FS.Open(name); err != nil {
			t.Errorf("docs.FS missing %s: %v", name, err)
		}
	}
}
