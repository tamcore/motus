package protocol_test

import (
	"testing"

	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/protocol"
)

const testIMEI = "123456789012345"

func TestH02CommandEncoder_RebootDevice(t *testing.T) {
	enc := &protocol.H02CommandEncoder{}
	cmd := &model.Command{Type: model.CommandRebootDevice}

	data, err := enc.EncodeCommand(cmd, testIMEI)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "*HQ," + testIMEI + ",reset#"
	if string(data) != want {
		t.Errorf("expected %q, got %q", want, string(data))
	}
}

func TestH02CommandEncoder_PositionSingle(t *testing.T) {
	enc := &protocol.H02CommandEncoder{}
	cmd := &model.Command{Type: model.CommandPositionSingle}

	data, err := enc.EncodeCommand(cmd, testIMEI)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "*HQ," + testIMEI + ",locate#"
	if string(data) != want {
		t.Errorf("expected %q, got %q", want, string(data))
	}
}

func TestH02CommandEncoder_PositionPeriodic(t *testing.T) {
	enc := &protocol.H02CommandEncoder{}
	cmd := &model.Command{
		Type:       model.CommandPositionPeriodic,
		Attributes: map[string]interface{}{"frequency": 30},
	}

	data, err := enc.EncodeCommand(cmd, testIMEI)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "*HQ," + testIMEI + ",time,30#"
	if string(data) != want {
		t.Errorf("expected %q, got %q", want, string(data))
	}
}

func TestH02CommandEncoder_PositionPeriodic_MissingFrequency(t *testing.T) {
	enc := &protocol.H02CommandEncoder{}
	cmd := &model.Command{Type: model.CommandPositionPeriodic}

	_, err := enc.EncodeCommand(cmd, testIMEI)
	if err == nil {
		t.Error("expected error for missing frequency attribute")
	}
}

func TestH02CommandEncoder_UnsupportedType(t *testing.T) {
	enc := &protocol.H02CommandEncoder{}
	cmd := &model.Command{Type: "unknownCommand"}

	_, err := enc.EncodeCommand(cmd, testIMEI)
	if err == nil {
		t.Error("expected error for unsupported command type")
	}
}

func TestH02CommandEncoder_SosNumber(t *testing.T) {
	enc := &protocol.H02CommandEncoder{}
	cmd := &model.Command{
		Type:       model.CommandSosNumber,
		Attributes: map[string]interface{}{"phoneNumber": "+4915112345"},
	}

	data, err := enc.EncodeCommand(cmd, testIMEI)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != "setphone,1,+4915112345" {
		t.Errorf("unexpected output: %q", string(data))
	}
}

func TestH02CommandEncoder_SosNumber_MissingPhone(t *testing.T) {
	enc := &protocol.H02CommandEncoder{}
	cmd := &model.Command{Type: model.CommandSosNumber}

	_, err := enc.EncodeCommand(cmd, testIMEI)
	if err == nil {
		t.Error("expected error for missing phoneNumber attribute")
	}
}

func TestH02CommandEncoder_SetSpeedAlarm(t *testing.T) {
	enc := &protocol.H02CommandEncoder{}
	cmd := &model.Command{
		Type:       model.CommandSetSpeedAlarm,
		Attributes: map[string]interface{}{"speed": 120},
	}

	data, err := enc.EncodeCommand(cmd, testIMEI)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != "setspeed,120" {
		t.Errorf("unexpected output: %q", string(data))
	}
}

func TestH02CommandEncoder_SetSpeedAlarm_MissingSpeed(t *testing.T) {
	enc := &protocol.H02CommandEncoder{}
	cmd := &model.Command{Type: model.CommandSetSpeedAlarm}

	_, err := enc.EncodeCommand(cmd, testIMEI)
	if err == nil {
		t.Error("expected error for missing speed attribute")
	}
}

func TestH02CommandEncoder_FactoryReset(t *testing.T) {
	enc := &protocol.H02CommandEncoder{}
	cmd := &model.Command{Type: model.CommandFactoryReset}

	data, err := enc.EncodeCommand(cmd, testIMEI)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != "FACTORY" {
		t.Errorf("expected 'FACTORY', got %q", string(data))
	}
}

func TestEncoderRegistry(t *testing.T) {
	reg := protocol.NewEncoderRegistry()

	enc := reg.Get("h02")
	if enc == nil {
		t.Fatal("expected h02 encoder to be registered")
	}
	if enc.Protocol() != "h02" {
		t.Errorf("expected protocol 'h02', got %q", enc.Protocol())
	}

	if reg.Get("unknown") != nil {
		t.Error("expected nil for unknown protocol")
	}
}
