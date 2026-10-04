package services

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/tamcore/motus/internal/audit"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/storage/repository/testutil"
)

type memRuleRepo struct {
	repository.NotificationRepo
	rules map[int64]*model.NotificationRule
}

func (m *memRuleRepo) GetByID(_ context.Context, id int64) (*model.NotificationRule, error) {
	r, ok := m.rules[id]
	if !ok {
		return nil, errors.New("no rows")
	}
	cp := *r
	return &cp, nil
}

func (m *memRuleRepo) Create(_ context.Context, r *model.NotificationRule) error {
	r.ID = int64(len(m.rules) + 1)
	m.rules[r.ID] = r
	return nil
}

func (m *memRuleRepo) Update(_ context.Context, r *model.NotificationRule) error {
	m.rules[r.ID] = r
	return nil
}

func (m *memRuleRepo) Delete(_ context.Context, id int64) error {
	delete(m.rules, id)
	return nil
}

type allowGeofenceRepo struct {
	repository.GeofenceRepo
	allowed []int64
}

func (a allowGeofenceRepo) UserHasAccess(_ context.Context, _ *model.User, id int64) bool {
	return slices.Contains(a.allowed, id)
}

func webhookRuleInput() NotificationRuleInput {
	return NotificationRuleInput{
		Name:       "Home",
		EventTypes: []string{model.EventTypeGeofenceEnter},
		Channel:    model.NotificationChannelWebhook,
		Config:     map[string]any{"webhookUrl": "http://127.0.0.1:9/hook"},
		Template:   "{{device.name}}",
		Enabled:    true,
	}
}

func newRuleServiceWith(rules ...*model.NotificationRule) (*NotificationRuleService, *memRuleRepo) {
	repo := &memRuleRepo{rules: map[int64]*model.NotificationRule{}}
	for _, r := range rules {
		repo.rules[r.ID] = r
	}
	return NewNotificationRuleService(repo, allowGeofenceRepo{allowed: []int64{5}}, nil), repo
}

func TestNotificationRuleService_CreateValidation(t *testing.T) {
	svc, _ := newRuleServiceWith()
	user := &model.User{ID: 1}
	for name, mutate := range map[string]func(*NotificationRuleInput){
		"missing name":     func(in *NotificationRuleInput) { in.Name = "" },
		"no event types":   func(in *NotificationRuleInput) { in.EventTypes = nil },
		"bad event type":   func(in *NotificationRuleInput) { in.EventTypes = []string{"nope"} },
		"bad channel":      func(in *NotificationRuleInput) { in.Channel = "ntfy" },
		"missing template": func(in *NotificationRuleInput) { in.Template = "" },
		"bad webhook url":  func(in *NotificationRuleInput) { in.Config = map[string]any{"webhookUrl": "ftp://x"} },
		"missing webhook":  func(in *NotificationRuleInput) { in.Config = nil },
		"geofence denied":  func(in *NotificationRuleInput) { in.GeofenceIDs = []int64{6} },
		"bad command type": func(in *NotificationRuleInput) {
			in.Channel, in.Config = model.NotificationChannelCommand, map[string]any{"commandType": model.CommandFactoryReset}
		},
		"geofence without geofence event": func(in *NotificationRuleInput) {
			in.EventTypes, in.GeofenceIDs = []string{"alarm"}, []int64{5}
		},
	} {
		in := webhookRuleInput()
		mutate(&in)
		if _, err := svc.CreateForUser(t.Context(), user, in); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: got %v, want ErrInvalid", name, err)
		}
	}
}

func TestNotificationRuleService_CreateChecksTemplateBeforeWebhookURL(t *testing.T) {
	svc, _ := newRuleServiceWith()
	in := webhookRuleInput()
	in.Template, in.Config = "", nil
	_, err := svc.CreateForUser(t.Context(), &model.User{ID: 1}, in)
	if err == nil || !strings.Contains(err.Error(), "template is required") {
		t.Fatalf("got %v, want template is required", err)
	}
}

func TestNotificationRuleService_CreateNormalizesGeofenceFilter(t *testing.T) {
	svc, repo := newRuleServiceWith()
	in := webhookRuleInput()
	in.GeofenceIDs = []int64{5, 5}
	r, err := svc.CreateForUser(t.Context(), &model.User{ID: 1}, in)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if got := repo.rules[r.ID]; got.UserID != 1 || !slices.Equal(got.GeofenceIDs, []int64{5}) {
		t.Fatalf("stored rule = %+v, want user 1 and geofences [5]", got)
	}
}

func TestNotificationRuleService_Ownership(t *testing.T) {
	svc, _ := newRuleServiceWith(&model.NotificationRule{ID: 1, UserID: 2})
	user := &model.User{ID: 1}
	if _, err := svc.UpdateForUser(t.Context(), user, 1, webhookRuleInput()); !errors.Is(err, ErrAccessDenied) || !errors.Is(err, ErrInvalid) {
		t.Errorf("update foreign rule: got %v, want access denied", err)
	}
	if err := svc.DeleteForUser(t.Context(), user, 1); !errors.Is(err, ErrAccessDenied) {
		t.Errorf("delete foreign rule: got %v, want access denied", err)
	}
	if _, err := svc.GetForUser(t.Context(), user, 99); !errors.Is(err, ErrNotFound) || err.Error() != "notification rule not found" {
		t.Errorf("get missing rule: got %v, want not found", err)
	}
	if _, err := svc.GetForUser(t.Context(), &model.User{ID: 3, Role: model.RoleAdmin}, 1); err != nil {
		t.Errorf("admin get: %v", err)
	}
}

func TestNotificationRuleService_UpdateGeofenceFilter(t *testing.T) {
	stored := func() *model.NotificationRule {
		return &model.NotificationRule{ID: 1, UserID: 1, EventTypes: []string{model.EventTypeGeofenceEnter}, GeofenceIDs: []int64{9}}
	}
	user := &model.User{ID: 1}

	t.Run("absent keeps stored filter even if no longer accessible", func(t *testing.T) {
		svc, repo := newRuleServiceWith(stored())
		if _, err := svc.UpdateForUser(t.Context(), user, 1, webhookRuleInput()); err != nil {
			t.Fatalf("update: %v", err)
		}
		if got := repo.rules[1].GeofenceIDs; !slices.Equal(got, []int64{9}) {
			t.Fatalf("geofences = %v, want [9]", got)
		}
	})
	t.Run("inherited filter dropped without geofence events", func(t *testing.T) {
		svc, repo := newRuleServiceWith(stored())
		in := webhookRuleInput()
		in.EventTypes = []string{model.EventTypeDeviceOnline}
		if _, err := svc.UpdateForUser(t.Context(), user, 1, in); err != nil {
			t.Fatalf("update: %v", err)
		}
		if got := repo.rules[1].GeofenceIDs; len(got) != 0 {
			t.Fatalf("geofences = %v, want []", got)
		}
	})
	t.Run("explicit empty clears", func(t *testing.T) {
		svc, repo := newRuleServiceWith(stored())
		in := webhookRuleInput()
		in.GeofenceIDs = []int64{}
		if _, err := svc.UpdateForUser(t.Context(), user, 1, in); err != nil {
			t.Fatalf("update: %v", err)
		}
		if got := repo.rules[1].GeofenceIDs; len(got) != 0 {
			t.Fatalf("geofences = %v, want []", got)
		}
	})
}

func TestNotificationRuleService_CommandRuleRejectsReconnectLoop(t *testing.T) {
	svc, _ := newRuleServiceWith(&model.NotificationRule{ID: 1, UserID: 1})
	in := webhookRuleInput()
	in.Channel = model.NotificationChannelCommand
	in.Config = map[string]any{"commandType": model.CommandRebootDevice}
	in.EventTypes = []string{model.EventTypeDeviceOnline}
	if _, err := svc.UpdateForUser(t.Context(), &model.User{ID: 1}, 1, in); !errors.Is(err, ErrInvalid) {
		t.Fatalf("got %v, want ErrInvalid", err)
	}
}

func TestNotificationRuleService_AuditsAllMutations(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	ctx := t.Context()
	user := testutil.CreateUser(t, "rule-audit@example.com")
	logger := audit.NewLogger(pool)
	svc := NewNotificationRuleService(repository.NewNotificationRepository(pool), repository.NewGeofenceRepository(pool), logger)

	r, err := svc.CreateForUser(ctx, user, webhookRuleInput())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.UpdateForUser(ctx, user, r.ID, webhookRuleInput()); err != nil {
		t.Fatalf("update: %v", err)
	}
	if err := svc.DeleteForUser(ctx, user, r.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	entries, _, err := logger.Query(ctx, audit.QueryParams{UserID: &user.ID, Action: audit.ActionNotifCreate})
	if err != nil || len(entries) != 1 {
		t.Fatalf("create entries: %d (err %v), want 1", len(entries), err)
	}
	if rt := entries[0].ResourceType; rt == nil || *rt != audit.ResourceNotification {
		t.Errorf("resourceType = %v, want %q", rt, audit.ResourceNotification)
	}
	if name := entries[0].Details["name"]; name != "Home" {
		t.Errorf("details name = %v, want Home", name)
	}

	for _, action := range []string{audit.ActionNotifCreate, audit.ActionNotifUpdate, audit.ActionNotifDelete} {
		_, total, err := logger.Query(ctx, audit.QueryParams{UserID: &user.ID, Action: action})
		if err != nil || total != 1 {
			t.Errorf("%s: %d entries (err %v), want 1", action, total, err)
		}
	}
}
