package protocol_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/protocol"
	"github.com/tamcore/motus/internal/storage/repository"
)

// submitCommandRepo records the commands created and status transitions.
type submitCommandRepo struct {
	repository.CommandRepo // unused methods panic
	created                []*model.Command
	statuses               []string
	createErr              error
}

func (r *submitCommandRepo) Create(_ context.Context, cmd *model.Command) error {
	if r.createErr != nil {
		return r.createErr
	}
	cmd.ID = int64(len(r.created) + 1)
	r.created = append(r.created, cmd)
	return nil
}

func (r *submitCommandRepo) UpdateStatus(_ context.Context, _ int64, status string) error {
	r.statuses = append(r.statuses, status)
	return nil
}

func h02Device() *model.Device {
	return &model.Device{ID: 5, UniqueID: "9000000000001", Protocol: "h02", Name: "Pet"}
}

func TestCommandSubmitter_OfflineDeviceQueuesPending(t *testing.T) {
	repo := &submitCommandRepo{}
	registry := protocol.NewDeviceRegistry()
	s := &protocol.CommandSubmitter{Commands: repo, Encoders: protocol.NewEncoderRegistry(registry), Registry: registry}

	cmd, err := s.Submit(context.Background(), h02Device(), model.CommandPositionPeriodic, map[string]any{"frequency": 20})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if cmd.Status != model.CommandStatusPending {
		t.Errorf("status = %q, want pending", cmd.Status)
	}
	if len(repo.created) != 1 || repo.created[0].DeviceID != 5 || repo.created[0].Type != model.CommandPositionPeriodic {
		t.Fatalf("expected one persisted command for device 5, got %+v", repo.created)
	}
	if len(repo.statuses) != 0 {
		t.Errorf("offline device must not change status, got %v", repo.statuses)
	}
}

func TestCommandSubmitter_OnlineDeviceSendsImmediately(t *testing.T) {
	repo := &submitCommandRepo{}
	registry := protocol.NewDeviceRegistry()
	ch := make(chan []byte, 1)
	registry.Register("9000000000001", ch)
	s := &protocol.CommandSubmitter{Commands: repo, Encoders: protocol.NewEncoderRegistry(registry), Registry: registry}

	cmd, err := s.Submit(context.Background(), h02Device(), model.CommandPositionPeriodic, map[string]any{"frequency": 300})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if cmd.Status != model.CommandStatusSent {
		t.Errorf("status = %q, want sent", cmd.Status)
	}
	select {
	case payload := <-ch:
		if string(payload) != "*HQ,9000000000001,time,300#" {
			t.Errorf("payload = %q", payload)
		}
	default:
		t.Fatal("expected payload on the device channel")
	}
	if len(repo.statuses) != 1 || repo.statuses[0] != model.CommandStatusSent {
		t.Errorf("statuses = %v, want [sent]", repo.statuses)
	}
}

func TestCommandSubmitter_UnsupportedByProtocol(t *testing.T) {
	repo := &submitCommandRepo{}
	s := &protocol.CommandSubmitter{Commands: repo, Encoders: protocol.NewEncoderRegistry(protocol.NewDeviceRegistry()), Registry: protocol.NewDeviceRegistry()}
	dev := &model.Device{ID: 1, UniqueID: "w1", Protocol: "watch"}

	_, err := s.Submit(context.Background(), dev, model.CommandSetSpeedAlarm, map[string]any{"speed": 80})
	if !errors.Is(err, protocol.ErrCommandUnsupported) {
		t.Fatalf("err = %v, want ErrCommandUnsupported", err)
	}
	if !strings.Contains(err.Error(), "watch") {
		t.Errorf("error should name the protocol: %v", err)
	}
	if len(repo.created) != 0 {
		t.Error("unsupported command must not be persisted")
	}
}

func TestCommandSubmitter_EncodeError(t *testing.T) {
	repo := &submitCommandRepo{}
	s := &protocol.CommandSubmitter{Commands: repo, Encoders: protocol.NewEncoderRegistry(protocol.NewDeviceRegistry()), Registry: protocol.NewDeviceRegistry()}

	_, err := s.Submit(context.Background(), h02Device(), model.CommandPositionPeriodic, nil)
	if !errors.Is(err, protocol.ErrCommandEncode) {
		t.Fatalf("err = %v, want ErrCommandEncode", err)
	}
	if len(repo.created) != 0 {
		t.Error("unencodable command must not be persisted")
	}
}

func TestCommandSubmitter_NoEncoderForProtocol(t *testing.T) {
	repo := &submitCommandRepo{}
	s := &protocol.CommandSubmitter{Commands: repo, Encoders: protocol.NewEncoderRegistry(protocol.NewDeviceRegistry()), Registry: protocol.NewDeviceRegistry()}
	dev := &model.Device{ID: 1, UniqueID: "o1", Protocol: "osmand"}

	// osmand supports no commands at all.
	_, err := s.Submit(context.Background(), dev, model.CommandPositionSingle, nil)
	if !errors.Is(err, protocol.ErrCommandUnsupported) {
		t.Fatalf("err = %v, want ErrCommandUnsupported", err)
	}
}

func TestCommandSubmitter_StoreError(t *testing.T) {
	dbErr := errors.New("db down")
	repo := &submitCommandRepo{createErr: dbErr}
	s := &protocol.CommandSubmitter{Commands: repo, Encoders: protocol.NewEncoderRegistry(protocol.NewDeviceRegistry()), Registry: protocol.NewDeviceRegistry()}

	_, err := s.Submit(context.Background(), h02Device(), model.CommandPositionSingle, nil)
	if !errors.Is(err, dbErr) {
		t.Fatalf("err = %v, want wrapped store error", err)
	}
}
