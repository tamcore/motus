package watch

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"testing"
)

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("bad hex: %v", err)
	}
	return b
}

// scanFrames runs SplitFunc over input the way the protocol server does.
func scanFrames(t *testing.T, input []byte) [][]byte {
	t.Helper()
	scanner := bufio.NewScanner(bytes.NewReader(input))
	scanner.Buffer(make([]byte, 0, 1024), 1<<20)
	scanner.Split(SplitFunc)
	var frames [][]byte
	for scanner.Scan() {
		frames = append(frames, append([]byte(nil), scanner.Bytes()...))
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scanner error: %v", err)
	}
	return frames
}

// Test vectors ported from Traccar's WatchFrameDecoderTest.
func TestSplitFunc_TraccarVectors(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name: "four concatenated frames without newlines",
			input: "5b33472a393730353134313734302a303030392a4c4b2c302c302c35335d" +
				"5b33472a393730353134313734302a303035412a55442c3139303732332c3139303730372c412c33362e3831353130392c4e2c31302e313739323331322c452c382e32342c3132372e392c32312e302c352c3130302c35332c302c302c30303030303030302c302c302c35382e305d" +
				"5b33472a393730353134313734302a303030332a544b515d" +
				"5b33472a393730353134313734302a303030392a4c4b2c302c302c35335d",
			want: []string{
				"[3G*9705141740*0009*LK,0,0,53]",
				"[3G*9705141740*005A*UD,190723,190707,A,36.815109,N,10.1792312,E,8.24,127.9,21.0,5,100,53,0,0,00000000,0,0,58.0]",
				"[3G*9705141740*0003*TKQ]",
				"[3G*9705141740*0009*LK,0,0,53]",
			},
		},
		{
			name:  "single LK",
			input: "5b33472a3335323636313039303134333135302a303030412a4c4b2c302c302c3130305d",
			want:  []string{"[3G*352661090143150*000A*LK,0,0,100]"},
		},
		{
			name:  "indexed LK",
			input: "5b5a4a2a3031343131313030313335303330342a303033342a303030392a4c4b2c302c302c31395d",
			want:  []string{"[ZJ*014111001350304*0034*0009*LK,0,0,19]"},
		},
		{
			name:  "rcapture",
			input: "5b33472a383330383430363237392a303030382a72636170747572655d",
			want:  []string{"[3G*8308406279*0008*rcapture]"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frames := scanFrames(t, mustHex(t, tt.input))
			if len(frames) != len(tt.want) {
				t.Fatalf("got %d frames, want %d: %q", len(frames), len(tt.want), frames)
			}
			for i, f := range frames {
				if string(f) != tt.want[i] {
					t.Errorf("frame %d: got %q, want %q", i, f, tt.want[i])
				}
			}
		})
	}
}

func TestSplitFunc_WiFiNamesWithBrackets(t *testing.T) {
	// Traccar vector: a Wi-Fi SSID "[LG_Wall-Mount A/C]" contains literal
	// brackets, which must be balanced rather than terminate the frame.
	raw := "[3G*880900242*013D*UD,120623,140020,A,48.949273,N, 4.3783060,E,18.56,43.8,0.0,12,100,76,226120,0,00000000,2,255,204,8,3110,55025,146,3130,49297,124,5,BangingWifi,34:a1:ed:e1:91:4f,-71,BAR WiFi,36:a2:e1:ed:a1:de,-72,NetworkWifi,26:de:a1:ed:e1:a0,-73,Fiber,36:a1:ed:e1:91:4f,-75,[LG_Wall-Mount A/C]e725,66:a1:ed:e1:e7:25,-82,15.0]"
	frames := scanFrames(t, []byte(raw))
	if len(frames) != 1 || string(frames[0]) != raw {
		t.Fatalf("got %q, want single frame %q", frames, raw)
	}
}

func TestSplitFunc_PartialFrameWaitsForMoreData(t *testing.T) {
	adv, tok, err := SplitFunc([]byte("[3G*1234567890*0002*L"), false)
	if err != nil || tok != nil || adv != 0 {
		t.Fatalf("got adv=%d tok=%q err=%v, want request for more data", adv, tok, err)
	}
}

func TestSplitFunc_SkipsGarbageAndNewlines(t *testing.T) {
	frames := scanFrames(t, []byte("junk\r\n[3G*1*0002*LK]\r\n\r\n[3G*1*0003*TKQ]\n"))
	want := []string{"[3G*1*0002*LK]", "[3G*1*0003*TKQ]"}
	if len(frames) != len(want) {
		t.Fatalf("got %q, want %q", frames, want)
	}
	for i := range want {
		if string(frames[i]) != want[i] {
			t.Errorf("frame %d: got %q, want %q", i, frames[i], want[i])
		}
	}
}

func TestSplitFunc_IncompleteFrameAtEOFIsDropped(t *testing.T) {
	frames := scanFrames(t, []byte("[3G*1*0002*LK][3G*1*00"))
	if len(frames) != 1 || string(frames[0]) != "[3G*1*0002*LK]" {
		t.Fatalf("got %q", frames)
	}
}

func TestSplitFunc_EscapedBytesDoNotAffectBrackets(t *testing.T) {
	// Traccar vector: escaped ] [ inside binary TK payload.
	input := append([]byte("[CS*1234567890*000e*TK,#!AMR"), mustHex(t, "7d017d027d037d047d05ff")...)
	input = append(input, ']')
	frames := scanFrames(t, input)
	if len(frames) != 1 || !bytes.Equal(frames[0], input) {
		t.Fatalf("got %q, want %q", frames, input)
	}
}

func TestUnescape(t *testing.T) {
	input := append([]byte("[CS*1234567890*000e*TK,#!AMR"), mustHex(t, "7d017d027d037d047d05ff")...)
	input = append(input, ']')
	want := append([]byte("[CS*1234567890*000e*TK,#!AMR"), mustHex(t, "7d5b5d2c2aff")...)
	want = append(want, ']')

	got, err := Unescape(input)
	if err != nil {
		t.Fatalf("unescape: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("got %x, want %x", got, want)
	}
}

func TestUnescape_Errors(t *testing.T) {
	for _, in := range []string{"[3G*1*0002*LK}]", "[3G*1*0002*LK}\x09]"} {
		if _, err := Unescape([]byte(in)); err == nil {
			t.Errorf("Unescape(%q): expected error", in)
		}
	}
}
