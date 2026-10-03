package services

import (
	"errors"
	"log/slog"
)

// ErrInvalid matches errors whose message is safe to return to clients
// (rejected input, missing access). Other service errors may carry storage
// internals and must not be exposed.
var ErrInvalid = errors.New("invalid request")

type invalidError struct{ error }

func (invalidError) Is(target error) bool { return target == ErrInvalid }

// PublicMessage returns err's message when it matches ErrInvalid; otherwise
// it logs err and returns fallback.
func PublicMessage(err error, fallback string) string {
	if errors.Is(err, ErrInvalid) {
		return err.Error()
	}
	slog.Error(fallback, slog.Any("error", err))
	return fallback
}

// invalid marks err as client-safe; nil stays nil.
func invalid(err error) error {
	if err == nil {
		return nil
	}
	return invalidError{err}
}
