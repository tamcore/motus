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

// CalendarInput holds calendar fields. On update, empty fields keep their
// stored value.
type CalendarInput struct {
	Name string
	Data string // valid iCalendar (RFC 5545) text
}

// validateCalendarFields checks the non-empty fields of in.
func validateCalendarFields(in CalendarInput) error {
	if in.Name != "" {
		if err := validation.ValidateDisplayName(in.Name); err != nil {
			return err
		}
	}
	if in.Data != "" {
		if err := calendar.Validate(in.Data); err != nil {
			return fmt.Errorf("invalid iCalendar data: %w", err)
		}
	}
	return nil
}

func validateCalendarInput(in CalendarInput) error {
	if in.Name == "" {
		return errors.New("name is required")
	}
	if in.Data == "" {
		return errors.New("data is required")
	}
	return validateCalendarFields(in)
}

// UpdateForUser applies the non-empty fields of in to a calendar user can
// access and emits an audit entry.
func (s *CalendarService) UpdateForUser(ctx context.Context, user *model.User, calendarID int64, in CalendarInput) (*model.Calendar, error) {
	if !s.repo.UserHasAccess(ctx, user, calendarID) {
		return nil, invalid(fmt.Errorf("calendar %w", ErrNotFound))
	}
	existing, err := s.repo.GetByID(ctx, calendarID)
	if err != nil || existing == nil {
		return nil, invalid(fmt.Errorf("calendar %w", ErrNotFound))
	}
	if err := invalid(validateCalendarFields(in)); err != nil {
		return nil, err
	}

	updated := *existing
	if in.Name != "" {
		updated.Name = in.Name
	}
	if in.Data != "" {
		updated.Data = in.Data
	}
	if err := s.repo.Update(ctx, &updated); err != nil {
		return nil, fmt.Errorf("update calendar: %w", err)
	}

	s.auditLogger.Log(ctx, &user.ID,
		audit.ActionCalendarUpdate, audit.ResourceCalendar, &updated.ID,
		map[string]any{"name": updated.Name})
	return &updated, nil
}

// CreateForUser validates, persists, and audits a new calendar for user.
func (s *CalendarService) CreateForUser(ctx context.Context, user *model.User, in CalendarInput) (*model.Calendar, error) {
	if err := invalid(validateCalendarInput(in)); err != nil {
		return nil, err
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
		map[string]any{"name": c.Name})
	return c, nil
}
