package services

import (
	"context"
	"errors"
	"fmt"

	"github.com/tamcore/motus/internal/audit"
	"github.com/tamcore/motus/internal/calendar"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/validation"
)

// CalendarService bundles calendar creation with validation and audit logging.
type CalendarService struct {
	repo        repository.CalendarRepo
	auditLogger *audit.Logger
}

// NewCalendarService returns a CalendarService backed by the given repo.
// auditLogger may be nil (audit entries are silently skipped).
func NewCalendarService(repo repository.CalendarRepo, auditLogger *audit.Logger) *CalendarService {
	return &CalendarService{repo: repo, auditLogger: auditLogger}
}

// CreateCalendarInput holds the validated inputs for creating a calendar.
type CreateCalendarInput struct {
	Name string
	Data string // valid iCalendar (RFC 5545) text
}

// CreateForUser validates, persists, and audits a new calendar for user.
func (s *CalendarService) CreateForUser(ctx context.Context, user *model.User, in CreateCalendarInput) (*model.Calendar, error) {
	if in.Name == "" {
		return nil, errors.New("name is required")
	}
	if err := validation.ValidateDisplayName(in.Name); err != nil {
		return nil, err
	}
	if in.Data == "" {
		return nil, errors.New("data is required")
	}
	if err := calendar.Validate(in.Data); err != nil {
		return nil, fmt.Errorf("invalid iCalendar data: %w", err)
	}

	c := &model.Calendar{
		UserID: user.ID,
		Name:   in.Name,
		Data:   in.Data,
	}
	if err := s.repo.Create(ctx, c); err != nil {
		return nil, err
	}

	s.auditLogger.Log(ctx, &user.ID,
		audit.ActionCalendarCreate, audit.ResourceCalendar, &c.ID,
		map[string]any{"name": c.Name}, "", "")
	return c, nil
}
