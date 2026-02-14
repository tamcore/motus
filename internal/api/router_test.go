package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLimitRequestBody(t *testing.T) {
	// Handler that reads the entire body.
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "body too large", http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	handler := limitRequestBody(inner)

	t.Run("small body accepted", func(t *testing.T) {
		body := strings.NewReader("hello")
		req := httptest.NewRequest(http.MethodPost, "/", body)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rr.Code)
		}
	})

	t.Run("oversized body rejected", func(t *testing.T) {
		// Create a body larger than maxRequestBodySize (1 MB).
		bigBody := strings.NewReader(strings.Repeat("x", maxRequestBodySize+1))
		req := httptest.NewRequest(http.MethodPost, "/", bigBody)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusRequestEntityTooLarge {
			t.Errorf("expected 413, got %d", rr.Code)
		}
	})
}
