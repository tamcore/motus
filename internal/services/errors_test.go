package services

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/tamcore/motus/internal/model"
)

func TestPublicMessage(t *testing.T) {
	_, err := NewCalendarService(nil, nil).CreateForUser(context.Background(), &model.User{ID: 1}, CreateCalendarInput{})
	if !errors.Is(err, ErrInvalid) || PublicMessage(err, "generic") != "name is required" {
		t.Errorf("validation error: got %v", err)
	}
	if err := invalid(fmt.Errorf("geofence %w", ErrNotFound)); !errors.Is(err, ErrNotFound) || !errors.Is(err, ErrInvalid) {
		t.Errorf("wrapped sentinel not matched: %v", err)
	}
	if got := PublicMessage(errors.New("pq: secret"), "generic"); got != "generic" {
		t.Errorf("storage error: got %q", got)
	}
}
