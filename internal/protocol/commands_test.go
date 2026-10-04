package protocol_test

import (
	"errors"
	"strings"
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
	reg := protocol.NewEncoderRegistry(nil)

	if _, err := reg.Encode("h02", &model.Command{Type: model.CommandRebootDevice}, testIMEI); err != nil {
		t.Fatalf("expected h02 encoder to be registered: %v", err)
	}

	if _, err := reg.Encode("unknown", &model.Command{Type: model.CommandRebootDevice}, testIMEI); !errors.Is(err, protocol.ErrNoEncoder) {
		t.Errorf("expected ErrNoEncoder for unknown protocol, got %v", err)
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
	enc := &protocol.WatchCommandEncoder{}
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
	enc := &protocol.WatchCommandEncoder{}
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

		data, err := protocol.NewEncoderRegistry(registry).Encode("watch", &model.Command{Type: model.CommandPositionSingle}, watchID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(data) != tt.frame {
			t.Errorf("session %+v: got %q, want %q", tt.session, data, tt.frame)
		}
	}
}

func TestEncoderRegistry_SupportedCommands(t *testing.T) {
	reg := protocol.NewEncoderRegistry(nil)
	tests := []struct {
		protocol string
		want     []string
	}{
		{"h02", []string{model.CommandRebootDevice, model.CommandPositionPeriodic, model.CommandPositionSingle,
			model.CommandSosNumber, model.CommandCustom, model.CommandSetSpeedAlarm, model.CommandFactoryReset}},
		{"watch", []string{model.CommandRebootDevice, model.CommandPositionPeriodic, model.CommandPositionSingle,
			model.CommandSosNumber, model.CommandCustom}},
		// Protocols without an encoder (e.g. OsmAnd over HTTP) take no commands.
		{"osmand", []string{}},
		// Protocol not known yet (device never connected): offer everything.
		{"", model.SupportedCommandTypes()},
	}
	for _, tt := range tests {
		got := reg.SupportedCommands(tt.protocol)
		if got == nil || strings.Join(got, ",") != strings.Join(tt.want, ",") {
			t.Errorf("%q: got %v, want %v", tt.protocol, got, tt.want)
		}
	}

	var nilReg *protocol.EncoderRegistry
	if got := nilReg.SupportedCommands("watch"); strings.Join(got, ",") != strings.Join(model.SupportedCommandTypes(), ",") {
		t.Errorf("nil registry: got %v", got)
	}
}

// Every command type an encoder advertises must encode, and every other
// type must be rejected, so the UI never offers a command that fails.
func TestEncoders_SupportedCommandsMatchEncoding(t *testing.T) {
	sample := map[string]map[string]any{
		model.CommandPositionPeriodic: {"frequency": 60},
		model.CommandSosNumber:        {"phoneNumber": "+49123"},
		model.CommandCustom:           {"text": "CR"},
		model.CommandSetSpeedAlarm:    {"speed": 80},
	}
	reg := protocol.NewEncoderRegistry(nil)
	for _, name := range []string{"h02", "watch"} {
		supported := map[string]bool{}
		for _, typ := range reg.SupportedCommands(name) {
			supported[typ] = true
		}
		for _, typ := range model.SupportedCommandTypes() {
			_, err := reg.Encode(name, &model.Command{Type: typ, Attributes: sample[typ]}, testIMEI)
			if supported[typ] && err != nil {
				t.Errorf("%s: advertised %s fails to encode: %v", name, typ, err)
			}
			if !supported[typ] && err == nil {
				t.Errorf("%s: %s encodes but is not advertised", name, typ)
			}
		}
	}
}
