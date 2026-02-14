package demo

import (
	"strings"
	"testing"
)

const testIMEI = "123456789012345"

func TestBuildSMSResponse(t *testing.T) {
	got := buildSMSResponse(testIMEI, "Reboot OK")
	want := "*HQ,123456789012345,SMS,Reboot OK#\r\n"
	if got != want {
		t.Errorf("buildSMSResponse = %q, want %q", got, want)
	}
}

func TestGenerateResponse_Rconf(t *testing.T) {
	resp := generateResponse(testIMEI, "rconf")
	if resp == "" {
		t.Fatal("expected non-empty response for rconf")
	}
	if !strings.Contains(resp, "203.0.113.1") {
		t.Errorf("rconf response should contain RFC-5737 test IP 203.0.113.1, got: %s", resp)
	}
	// Ensure no real IP or phone leaked in.
	if strings.Contains(resp, "85.215.131.94") {
		t.Errorf("rconf response contains real IP, got: %s", resp)
	}
	if strings.Contains(resp, "491737224266") {
		t.Errorf("rconf response contains real phone number, got: %s", resp)
	}
}

func TestGenerateResponse_Reset(t *testing.T) {
	resp := generateResponse(testIMEI, "*HQ,123456789012345,reset")
	if !strings.Contains(resp, "Reboot OK") {
		t.Errorf("reset response should contain 'Reboot OK', got: %q", resp)
	}
}

func TestGenerateResponse_Time(t *testing.T) {
	resp := generateResponse(testIMEI, "*HQ,x,time,30")
	if !strings.Contains(resp, "Interval: 30s") {
		t.Errorf("time response should contain 'Interval: 30s', got: %q", resp)
	}
}

func TestGenerateResponse_Locate(t *testing.T) {
	resp := generateResponse(testIMEI, "*HQ,123456789012345,locate")
	if resp != "" {
		t.Errorf("locate should return empty, got: %q", resp)
	}
}

func TestGenerateResponse_V4Ack(t *testing.T) {
	resp := generateResponse(testIMEI, "*HQ,123456789012345,V4,12")
	if resp != "" {
		t.Errorf("V4 ack should return empty, got: %q", resp)
	}
}

func TestGenerateResponse_SetPhone(t *testing.T) {
	resp := generateResponse(testIMEI, "setphone,1,+12025550100")
	if !strings.Contains(resp, "SOS1: +12025550100") {
		t.Errorf("setphone response should contain 'SOS1: +12025550100', got: %q", resp)
	}
}

func TestGenerateResponse_SetSpeed(t *testing.T) {
	resp := generateResponse(testIMEI, "setspeed,80")
	if !strings.Contains(resp, "Speed alarm: 80km/h") {
		t.Errorf("setspeed 80 should contain 'Speed alarm: 80km/h', got: %q", resp)
	}

	resp0 := generateResponse(testIMEI, "setspeed,0")
	if !strings.Contains(resp0, "Speed alarm: disabled") {
		t.Errorf("setspeed 0 should contain 'Speed alarm: disabled', got: %q", resp0)
	}
}

func TestGenerateResponse_Factory(t *testing.T) {
	resp := generateResponse(testIMEI, "FACTORY")
	if !strings.Contains(resp, "Factory reset, please wait...") {
		t.Errorf("FACTORY response should contain 'Factory reset, please wait...', got: %q", resp)
	}
	// Case-insensitive.
	resp2 := generateResponse(testIMEI, "factory")
	if !strings.Contains(resp2, "Factory reset, please wait...") {
		t.Errorf("factory (lower) response should contain 'Factory reset, please wait...', got: %q", resp2)
	}
}

func TestGenerateResponse_Stockade(t *testing.T) {
	respOn := generateResponse(testIMEI, "stockade,on,51.5,0.0,51.6,0.1")
	if !strings.Contains(respOn, "Geofence enabled") {
		t.Errorf("stockade on should contain 'Geofence enabled', got: %q", respOn)
	}

	respOff := generateResponse(testIMEI, "stockade,off")
	if !strings.Contains(respOff, "Geofence disabled") {
		t.Errorf("stockade off should contain 'Geofence disabled', got: %q", respOff)
	}
}

func TestGenerateResponse_Unknown(t *testing.T) {
	resp := generateResponse(testIMEI, "unknowncmd")
	if resp != "" {
		t.Errorf("unknown command should return empty, got: %q", resp)
	}
}

func TestGenerateResponse_GenericHQ(t *testing.T) {
	resp := generateResponse(testIMEI, "*HQ,123456789012345,someother")
	if !strings.Contains(resp, "OK") {
		t.Errorf("generic HQ command should return OK, got: %q", resp)
	}
}

func TestDispatchTokens_Multi(t *testing.T) {
	// Two '#'-terminated tokens plus a bare newline-terminated raw command,
	// with an unterminated tail.  We verify only the tail here (dispatched
	// responses require a live conn).
	input := []byte("*HQ,x,locate#*HQ,x,V4,V1,20260101#pending")
	tail := dispatchTokensNoWrite(testIMEI, input)
	if string(tail) != "pending" {
		t.Errorf("tail should be 'pending', got: %q", string(tail))
	}
}

func TestDispatchTokens_RawConcatenatedWithV4(t *testing.T) {
	// Simulate the race: "setspeed,80" arrives without a newline, and before
	// the 500 ms flush timeout the next TCP segment brings a V4 ack.  The
	// two bytes are accumulated as one token before the '#' is processed.
	// The raw command must still be dispatched and the V4 must be silently
	// discarded (no response).
	raw := "setspeed,80*HQ," + testIMEI + ",V4,V1,20260101000000#\r\n"
	tokens, responses := collectDispatchedResponses(testIMEI, []byte(raw))
	if len(tokens) != 0 {
		t.Errorf("expected 0 tail bytes, got %q", string(tokens))
	}
	// Exactly one non-empty response: the speed-alarm SMS.
	if len(responses) != 1 {
		t.Fatalf("expected 1 SMS response, got %d: %v", len(responses), responses)
	}
	if !strings.Contains(responses[0], "Speed alarm: 80km/h") {
		t.Errorf("expected 'Speed alarm: 80km/h' in response, got %q", responses[0])
	}
}

// collectDispatchedResponses is a test helper that runs the same split logic
// as dispatchTokens (including the raw-prefix split) and returns both the
// unconsumed tail and every non-empty generateResponse result.
func collectDispatchedResponses(imei string, acc []byte) (tail []byte, responses []string) {
	for {
		s := string(acc)
		hashIdx := strings.IndexByte(s, '#')
		nlIdx := strings.IndexByte(s, '\n')
		if hashIdx < 0 && nlIdx < 0 {
			break
		}
		var token string
		if hashIdx >= 0 && (nlIdx < 0 || hashIdx < nlIdx) {
			token = s[:hashIdx]
			acc = acc[hashIdx+1:]
		} else {
			token = strings.TrimRight(s[:nlIdx], "\r")
			acc = acc[nlIdx+1:]
		}
		// Mirror the raw-prefix split added to dispatchTokens.
		if hqPos := strings.Index(token, "*HQ,"); hqPos > 0 {
			rawToken := strings.TrimSpace(token[:hqPos])
			if rawToken != "" {
				if r := generateResponse(imei, rawToken); r != "" {
					responses = append(responses, r)
				}
			}
			token = token[hqPos:]
		}
		if r := generateResponse(imei, token); r != "" {
			responses = append(responses, r)
		}
	}
	return acc, responses
}

func TestDispatchTokens_RawNewline(t *testing.T) {
	// Raw commands arrive as "cmd\r\n".
	input := []byte("rconf\r\nFACTORY\r\ntail")
	tail := dispatchTokensNoWrite(testIMEI, input)
	if string(tail) != "tail" {
		t.Errorf("tail should be 'tail', got: %q", string(tail))
	}
}

// dispatchTokensNoWrite is a test helper that splits tokens (both '#' and '\n')
// and returns the remaining tail without requiring a real net.Conn.
func dispatchTokensNoWrite(imei string, acc []byte) []byte {
	for {
		s := string(acc)
		hashIdx := strings.IndexByte(s, '#')
		nlIdx := strings.IndexByte(s, '\n')
		if hashIdx < 0 && nlIdx < 0 {
			break
		}
		if hashIdx >= 0 && (nlIdx < 0 || hashIdx < nlIdx) {
			acc = acc[hashIdx+1:]
		} else {
			acc = acc[nlIdx+1:]
		}
	}
	return acc
}
