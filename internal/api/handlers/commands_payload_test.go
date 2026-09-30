package handlers_test

import (
	"testing"

	"github.com/go-faster/jx"
	oas "github.com/tamcore/motus/internal/api/oas"
)

// TestSendCommandRequest_DecodesUIPayloads pins the request bodies built by
// the web UI (web/src/lib/utils/commands.ts): attributes carry the command
// type as oneOf discriminator, and parameterless commands omit attributes.
func TestSendCommandRequest_DecodesUIPayloads(t *testing.T) {
	valid := map[string]string{
		"positionPeriodic": `{"deviceId":1,"type":"positionPeriodic","attributes":{"frequency":60,"type":"positionPeriodic"}}`,
		"sosNumber":        `{"deviceId":1,"type":"sosNumber","attributes":{"phoneNumber":"+49123","type":"sosNumber"}}`,
		"setSpeedAlarm":    `{"deviceId":1,"type":"setSpeedAlarm","attributes":{"speed":80,"type":"setSpeedAlarm"}}`,
		"custom":           `{"deviceId":1,"type":"custom","attributes":{"text":"UPLOAD,60","type":"custom"}}`,
		"rebootDevice":     `{"deviceId":1,"type":"rebootDevice"}`,
		"positionSingle":   `{"deviceId":1,"type":"positionSingle"}`,
		"factoryReset":     `{"deviceId":1,"type":"factoryReset"}`,
	}
	for name, body := range valid {
		var req oas.SendCommandRequest
		if err := req.Decode(jx.DecodeStr(body)); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}

	// The payloads the UI sent before the fix are rejected by the API.
	for _, body := range []string{
		`{"deviceId":1,"type":"positionPeriodic","attributes":{"frequency":60}}`,
		`{"deviceId":1,"type":"rebootDevice","attributes":{}}`,
	} {
		var req oas.SendCommandRequest
		if err := req.Decode(jx.DecodeStr(body)); err == nil {
			t.Errorf("expected decode error for %s", body)
		}
	}
}
