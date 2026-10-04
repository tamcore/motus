package repository

import "testing"

func TestConnect_Errors(t *testing.T) {
	for _, url := range []string{"://bad", "postgres://u:p@127.0.0.1:1/db?connect_timeout=1"} {
		if pool, err := Connect(t.Context(), url); err == nil {
			pool.Close()
			t.Errorf("Connect(%q) succeeded, want error", url)
		}
	}
}
