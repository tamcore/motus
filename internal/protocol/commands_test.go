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
		Attributes: map[string]any{"frequency": 30},
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
		Attributes: map[string]any{"phoneNumber": "+4915112345"},
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
		Attributes: map[string]any{"speed": 120},
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

func TestH02CommandEncoder_Custom(t *testing.T) {
	enc := &protocol.H02CommandEncoder{}
	cmd := &model.Command{Type: model.CommandCustom, Attributes: map[string]any{"text": "rconf"}}

	data, err := enc.EncodeCommand(cmd, testIMEI)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != "rconf" {
		t.Errorf("expected raw text, got %q", string(data))
	}
}

const watchID = "8800000015"

// Expected frames follow Traccar's WatchProtocolEncoder.
func TestWatchCommandEncoder_Commands(t *testing.T) {
	tests := []struct {
		name  string
		cmd   *model.Command
		frame string
	}{
		{"reboot", &model.Command{Type: model.CommandRebootDevice}, "[CS*8800000015*0005*RESET]"},
		{"position single", &model.Command{Type: model.CommandPositionSingle}, "[CS*8800000015*0002*CR]"},
		{"position periodic", &model.Command{Type: model.CommandPositionPeriodic, Attributes: map[string]any{"frequency": 60}}, "[CS*8800000015*0009*UPLOAD,60]"},
		{"position periodic from JSON", &model.Command{Type: model.CommandPositionPeriodic, Attributes: map[string]any{"frequency": float64(300)}}, "[CS*8800000015*000a*UPLOAD,300]"},
		{"sos number", &model.Command{Type: model.CommandSosNumber, Attributes: map[string]any{"phoneNumber": "+49123456789"}}, "[CS*8800000015*0011*SOS1,+49123456789]"},
		{"sos number with index", &model.Command{Type: model.CommandSosNumber, Attributes: map[string]any{"phoneNumber": "123", "index": 3}}, "[CS*8800000015*0008*SOS3,123]"},
		{"custom", &model.Command{Type: model.CommandCustom, Attributes: map[string]any{"text": "LZ,1,+1"}}, "[CS*8800000015*0007*LZ,1,+1]"},
	}
	enc := protocol.NewWatchCommandEncoder(nil)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := enc.EncodeCommand(tt.cmd, watchID)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if string(data) != tt.frame {
				t.Errorf("got %q, want %q", data, tt.frame)
			}
		})
	}
}

func TestWatchCommandEncoder_Errors(t *testing.T) {
	enc := protocol.NewWatchCommandEncoder(nil)
	for _, cmd := range []*model.Command{
		{Type: model.CommandPositionPeriodic},
		{Type: model.CommandSosNumber},
		{Type: model.CommandCustom},
		{Type: model.CommandCustom, Attributes: map[string]any{"text": ""}},
		{Type: model.CommandSetSpeedAlarm, Attributes: map[string]any{"speed": 80}},
		{Type: model.CommandFactoryReset},
		{Type: "unknownCommand"},
	} {
		if _, err := enc.EncodeCommand(cmd, watchID); err == nil {
			t.Errorf("%s %v: expected error", cmd.Type, cmd.Attributes)
		}
	}
}

// The frame header mirrors the device's live connection: its manufacturer
// code (3G is answered as SG, like Traccar) and whether it uses indexed frames.
func TestWatchCommandEncoder_UsesConnectionSession(t *testing.T) {
	tests := []struct {
		session protocol.DeviceSession
		frame   string
	}{
		{protocol.DeviceSession{Manufacturer: "3G"}, "[SG*8800000015*0002*CR]"},
		{protocol.DeviceSession{Manufacturer: "SG"}, "[SG*8800000015*0002*CR]"},
		{protocol.DeviceSession{Manufacturer: "ZJ", Indexed: true}, "[ZJ*8800000015*0001*0002*CR]"},
		{protocol.DeviceSession{}, "[CS*8800000015*0002*CR]"},
	}
	for _, tt := range tests {
		registry := protocol.NewDeviceRegistry()
		registry.Register(watchID, make(chan []byte, 1))
		registry.SetSession(watchID, tt.session)

		data, err := protocol.NewWatchCommandEncoder(registry).EncodeCommand(&model.Command{Type: model.CommandPositionSingle}, watchID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(data) != tt.frame {
			t.Errorf("session %+v: got %q, want %q", tt.session, data, tt.frame)
		}
	}
}

func TestEncoderRegistry_Watch(t *testing.T) {
	enc := protocol.NewEncoderRegistry().Get("watch")
	if enc == nil || enc.Protocol() != "watch" {
		t.Fatalf("expected watch encoder, got %v", enc)
	}
}
