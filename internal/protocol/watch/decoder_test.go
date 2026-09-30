package watch

import (
	"math"
	"strings"
	"testing"
	"time"
)

// Position vectors ported from Traccar's WatchProtocolDecoderTest. Every one
// of them must decode to a position.
var traccarPositionVectors = []string{
	"[3G*6907919734*003e*AL_LTE,170525,214118,V,0,N,0,E,0,0,0,0,0,22,0,0,00010000,0,0,0]",
	"[3G*9705141740*00C2*UD_LTE,260723,185105,V,00.000000,,00.0000000,,0.00,0.0,0.0,0,100,67,0,0,00000000,2,0,605,1,10006,65799,14,10020,4104,4,3,,34:60:f9:ec:19:f8,-82,,98:48:27:55:18:20,-96,,34:e8:94:e4:06:18,-104,0.0]",
	"[SG*9059011020*0067*AL,240123,181628,V,54.427538,N,6.409275,W,0.00,0,0,0,19,90,0,0,00000000,1,1,234,10,55C0,3B882A2,132,,10]",
	"[SG*9059011020*006b*UD2,240123,162011,A,54.427621,N,6.409190,W,0.00,0,0,8,19,88,0,0,00000000,1,1,FFFF,FFFF,FFFE,3B882A2,132,,00]",
	"[ZJ*689466020014198*0003*0113*AL,221121,085515,V,00.000000,N,000.000000,E,0,0,0,0,0,44,0,0,00100000,1,255,460,0,16399,234887445,0,6,WIFI00,68:77:24:1b:e7:a7,-59,WIFI01,68:77:24:1b:e3:30,-75,WIFI02,68:77:24:1b:e3:27,-75,WIFI03,00:41:d2:c0:f2:f1,-76,WIFI04,00:41:d2:c0:f2:f0,-77,WIFI05,68:77:24:1b:e3:d8,-82]",
	"[3G*9031853319*004E*UD2,220322,055105,A,22.761162,N,114.360192,E,0,0,47,14,100,64,0,0,00000008,0,0]",
	"[SG*9159059735*0066*UD2,230322,082138,A,59.55285,N,016.66185,E,0.0,000,26,14,80,70,0,50,00000000,1,1,240,7,34505,80806406,,00]",
	"[SG*9059056143*0053*UD,251021,223408,A,41.46500,N,081.53128,W,0.926,000,0,00,70,70,0,50,00000000,0,1,,,,00]",
	"[3G*2104326058*00E9*UD_LTE,300621,135101,A,32.162652,N,34.888748,E,30.84,265.158,65.621,18,100,83,0,0,00000000,1,1,425,01,10223,8012811,100,3,ES4104,22:74:1d:39:64:ff,-46,metropoline-wifi,a8:3f:a1:e0:66:ba,-89,Egged.co.il,00:0c:42:51:cf:cd,-81,1.7055488]",
	"[3G*358839237678820*0122*ALCUSTOMER1,251120,081821,V,0.0,N,0.0,E,2.58,317.462,35.147,14,100,2,11089,0,00100008,1,1,460,01,42308,101992452,100,5,shizhou1,44:56:e2:03:ea:2a,-69,FART3,30:0d:9e:bb:fa:4d,-70,ZKY-A209,88:c3:97:c1:f4:7f,-73,ChinaNet-HNeD,e8:84:c6:21:7c:dc,-77,,30:45:96:10:14:5d,-79,1.2035439]",
	"[3G*0304187088*0100*UD_WCDMA,100720,094202,V,0.0,N,0.0,E,22.0,0,-1,21,75,92,0,0,00000000,1,1,425,01,10192,1282125,75,5,Inet,04:f0:21:46:1f:57,-54,iNetSecurity,00:1e:42:25:2f:3e,-71,Gilad,58:d5:6e:9d:1b:af,-80,weekend,14:ae:db:cb:99:25,-82,advancemed1,04:f0:21:4c:c8:3e,-89,0.0]",
	"[3G*8809008845*00C0*AL,271219,094744,V,00.000000,N, 0.0000000,E,0.00,0.0,0.0,0,100,81,0,0,00010000,7,0,460,0,9336,3981,141,9336,3912,141,9336,3982,140,9765,4233,134,9765,4071,134,9765,4321,134,9336,4353,132,0,0.0]",
	"[3G*2104134718*00A1*UD_WCDMA,161019,134938,A,43.373367,N,71.157615,W,22.0,350.206,279.717,17,28,79,0,0,00000000,1,1,310,410,23999,132013696,28,1,Home2,60:45:cb:cb:34:68,-93,8.263865]",
	"[ZJ*014111001332708*0075*0064*AL,040418,052156,A,22.536207,N,113.938673,E,0,0,0,5,100,82,1000,50,00100000,1,255,460,0,9340,3663,35]",
	"[SG*352661090143150*006C*UD,150817,132115,V,28.435142,N,81.354333,W,2.2038,000,99,00,70,100,0,50,00000000,1,1,310,260,1091,30082,139,,00]",
	"[3G*8308373902*0080*AL,230817,095346,A,47.083950,N,15.4821850,E,7.60,273.8,0.0,4,15,44,0,0,00200010,2,255,232,1,7605,42530,118,7605,58036,119,0,65.8]",
	"[3G*6005412902*011F*WT,170517,133811,V,18.512200,N,73.7750283,E,0.00,0.0,0.0,0,92,82,4262,0,00000010,2,1,404,22,10125,8301,141,10125,13921,122,5,Skynet,28:c6:8e:be:87:c0,-60,Intel Wi-Fi,4c:60:de:32:3d:38,-70,Nirvanic-2,40:e3:d6:4a:d9:c2,-73,A4-Guest,40:e3:d6:4a:d9:c4,-73,A4Idatix,40:e3:d6:4a:d9:c3,-73,13.8]",
	"[3G*8308406279*00CC*UD3,170417,190930,V,54.739618,N,25.273213,E,0.0,323.53,175.1,6,51,83,0,0,00000000,1,1,246,01,200,13242758,51,3,TEO-189835,00:8c:54:58:1d:64,-84,Cgates_7137,78:54:2e:e3:71:37,-85,ASUS,9c:5c:8e:b8:d4:78,-93]",
	"[SG*9051004074*0058*AL,120117,145602,V,40.058413,N,76.336618,W,11.519,188,99,00,01,80,0,50,00000000,0,1,0,0,,10]",
	"[SG*9051000884*009B*UD,030117,161129,V,52.745450,N,0.369512,,0.1481,000,99,00,70,5,0,50,00000000,5,1,234,15,893,3611,135,893,3612,132,893,3993,131,893,30986,129,893,40088,126,,00]",
	"[3G*6430073509*00E7*UD2,241016,081622,V,09.951861,N,-84.1422119,W,0.00,0.0,0.0,0,39,94,0,0,00000000,1,0,712,3,2007,18961,123,4,Luz,00:23:6a:34:ee:76,-70,familia,b0:c5:54:b9:90:ef,-78,fam salas delgado,fc:b4:e6:5d:50:ea,-81,QWERTY,c8:3a:35:43:0f:e8,-93]",
	"[3G*6105117105*008D*UD2,210716,231601,V,-33.480366,N,-70.7630692,E,0.00,0.0,0.0,0,100,34,0,0,00000000,3,255,730,2,29731,54315,167,29731,54316,162,29731,54317,145]",
	"[3G*4700222306*0077*UD,120316,140610,V,48.779045,N, 9.1574736,E,0.00,0.0,0.0,0,25,83,0,0,00000000,2,255,262,1,21041,9067,121,21041,5981,116]",
	"[3G*4700222306*011F*UD2,120316,140444,A,48.779045,N, 9.1574736,E,0.57,12.8,0.0,7,28,77,0,0,00000000,2,2,262,1,21041,9067,121,21041,5981,116,5,WG-Superlativ,34:31:c4:c8:a9:22,-67,EasyBox-28E858,18:83:bf:28:e8:f4,-70,MoMaXXg,be:05:43:b7:19:15,-72,MoMaXX2,bc:05:43:b7:19:15,-72,Gastzugang,18:83:bf:28:e8:f5,-72]",
	"[SG*9081000548*00A9*UD,110116,113639,V,16.479064,S,68.119072,,0.7593,000,99,00,80,80,0,50,00000000,5,1,736,2,10103,10732,153,10103,11061,152,10103,11012,152,10103,10151,150,10103,10731,143,,00]",
	"[3G*2256002206*0079*UD2,100116,153723,A,38.000000,N,-9.000000,W,0.44,299.3,0.0,7,100,86,0,0,00000008,2,0,268,3,3010,51042,146,3010,51043,132]",
	"[3G*4700186508*00B1*UD,301015,084840,V,45.853100,N,14.6224899,E,0.00,0.0,0.0,0,84,61,0,11,00000008,7,255,293,70,60,6453,139,60,6432,139,60,6431,132,60,6457,127,60,16353,126,60,6451,121,60,16352,118]",
	"[SG*8800000015*0087*UD,220414,134652,A,22.571707,N,113.8613968,E,0.1,0.0,100,7,60,90,1000,50,0000,4,1,460,0,9360,4082,131,9360,4092,148,9360,4091,143,9360,4153,141]",
	"[SG*8800000015*0088*UD2,220414,134652,A,22.571707,N,113.8613968,E,0.1,0.0,100,7,60,90,1000,50,0000,4,1,460,0,9360,4082,131,9360,4092,148,9360,4091,143,9360,4153,141]",
	"[SG*8800000015*0087*AL,220414,134652,A,22.571707,N,113.8613968,E,0.1,0.0,100,7,60,90,1000,50,0001,4,1,460,0,9360,4082,131,9360,4092,148,9360,4091,143,9360,4153,141]",
	"[ZJ*014111001350304*0033*0064*UD,070318,020827,V,00.000000,N,000.000000,E,0,0,0,0,100,19,1000,50,00000000,1,255,460,0,9346,5223,42]",
	"[ZJ*014111001350304*0035*0097*UD,070318,020857,V,00.000000,N,000.000000,E,0,0,0,0,100,19,1000,50,00000000,5,255,460,0,9346,5223,42,9346,5214,21,9784,4083,13,9346,5222,11,9346,5221,8]",
	"[ZJ*014111001350304*0038*008a*UD,070318,021027,V,00.000000,N,000.000000,E,0,0,0,0,100,18,1000,50,00000000,4,255,460,0,9346,5223,42,9346,5214,20,9784,4083,11,9346,5221,5]",
	"[3G*8800000015*00DD*UD,010120,025946,V,0.0,N,0.0,E,22.0,0,-1,0,100,98,0,0,00000000,0,5,eduroam,f4:db:e6:d2:a8:00,-53,eduroam,f4:db:e6:da:d0:80,-79,eduroam,78:0c:f0:24:f9:80,-82,Lions,b0:be:76:0a:05:9a,-82,tubs-guest,f4:db:e6:d2:a8:01,-53,0.0]",
}

func TestDecode_TraccarPositionVectors(t *testing.T) {
	for _, raw := range traccarPositionVectors {
		msg, err := Decode(raw)
		if err != nil {
			t.Errorf("Decode(%q): %v", raw, err)
			continue
		}
		if !msg.HasPosition() {
			t.Errorf("Decode(%q): type %q not treated as position", raw, msg.Type)
		}
		if msg.Position == nil {
			t.Errorf("Decode(%q): no position: %v", raw, msg.PositionErr)
		}
	}
}

func TestDecode_UD_Fields(t *testing.T) {
	msg, err := Decode("[SG*8800000015*0087*UD,220414,134652,A,22.571707,N,113.8613968,E,0.1,0.0,100,7,60,90,1000,50,0000,4,1,460,0,9360,4082,131,9360,4092,148,9360,4091,143,9360,4153,141]")
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if msg.Manufacturer != "SG" || msg.DeviceID != "8800000015" || msg.Index != "" || msg.Type != "UD" {
		t.Errorf("header: got %q/%q/%q/%q", msg.Manufacturer, msg.DeviceID, msg.Index, msg.Type)
	}
	if msg.Response != "" {
		t.Errorf("UD must not be acknowledged, got %q", msg.Response)
	}
	p := msg.Position
	if p == nil {
		t.Fatalf("no position: %v", msg.PositionErr)
	}
	if want := time.Date(2014, 4, 22, 13, 46, 52, 0, time.UTC); !p.Timestamp.Equal(want) {
		t.Errorf("Timestamp: got %v, want %v", p.Timestamp, want)
	}
	if !p.Valid {
		t.Error("expected valid fix")
	}
	if math.Abs(p.Latitude-22.57171) > 1e-5 || math.Abs(p.Longitude-113.86140) > 1e-5 {
		t.Errorf("location: got %f,%f", p.Latitude, p.Longitude)
	}
	if p.Speed != 0.1 || p.Course != 0 || p.Altitude != 100 {
		t.Errorf("speed/course/alt: got %v/%v/%v", p.Speed, p.Course, p.Altitude)
	}
	wantAttrs := map[string]any{
		"satellites": 7, "rssi": 60, "batteryLevel": 90, "steps": 1000, "status": int64(0),
		"mcc": 460, "mnc": 0, "lac": 9360, "cellId": 4082,
	}
	for k, v := range wantAttrs {
		if p.Attributes[k] != v {
			t.Errorf("attribute %s: got %v (%T), want %v (%T)", k, p.Attributes[k], p.Attributes[k], v, v)
		}
	}
	if _, ok := p.Attributes["alarm"]; ok {
		t.Errorf("unexpected alarm %v", p.Attributes["alarm"])
	}
	cells, _ := p.Network["cellTowers"].([]map[string]any)
	if len(cells) != 4 {
		t.Fatalf("cellTowers: got %d, want 4", len(cells))
	}
	if cells[3]["cellId"] != 4153 || cells[3]["signalStrength"] != 141 {
		t.Errorf("cell[3]: got %v", cells[3])
	}
}

func TestDecode_NegativeCoordinates(t *testing.T) {
	tests := []struct {
		raw      string
		lat, lon float64
	}{
		// Traccar expects -33.48037, -70.76307 (sign in value, N/E hemisphere).
		{"[3G*6105117105*008D*UD2,210716,231601,V,-33.480366,N,-70.7630692,E,0.00,0.0,0.0,0,100,34,0,0,00000000,3,255,730,2,29731,54315,167,29731,54316,162,29731,54317,145]", -33.480366, -70.7630692},
		// Sign in value and W hemisphere must not cancel out.
		{"[3G*6430073509*00E7*UD2,241016,081622,V,09.951861,N,-84.1422119,W,0.00,0.0,0.0,0,39,94,0,0,00000000,1,0,712,3,2007,18961,123]", 9.951861, -84.1422119},
		{"[SG*9081000548*00A9*UD,110116,113639,V,16.479064,S,68.119072,,0.7593,000,99,00,80,80,0,50,00000000,0]", -16.479064, 68.119072},
	}
	for _, tt := range tests {
		msg, err := Decode(tt.raw)
		if err != nil || msg.Position == nil {
			t.Fatalf("decode %q: %v / %v", tt.raw, err, msg.PositionErr)
		}
		if msg.Position.Latitude != tt.lat || msg.Position.Longitude != tt.lon {
			t.Errorf("%q: got %f,%f want %f,%f", tt.raw, msg.Position.Latitude, msg.Position.Longitude, tt.lat, tt.lon)
		}
	}
}

func TestDecode_Alarms(t *testing.T) {
	tests := []struct {
		raw      string
		alarm    string
		response string
	}{
		{"[3G*6907919734*003e*AL_LTE,170525,214118,V,0,N,0,E,0,0,0,0,0,22,0,0,00010000,0,0,0]", "sos", "[3G*6907919734*0002*AL]"},
		{"[ZJ*689466020014198*0003*0113*AL,221121,085515,V,00.000000,N,000.000000,E,0,0,0,0,0,44,0,0,00100000,1,255,460,0,16399,234887445,0]", "removing", "[ZJ*689466020014198*0003*0002*AL]"},
		{"[3G*8308373902*0080*AL,230817,095346,A,47.083950,N,15.4821850,E,7.60,273.8,0.0,4,15,44,0,0,00200010,2,255,232,1,7605,42530,118,7605,58036,119,0,65.8]", "fallDown", "[3G*8308373902*0002*AL]"},
		{"[SG*8800000015*0087*AL,220414,134652,A,22.571707,N,113.8613968,E,0.1,0.0,100,7,60,90,1000,50,0001,4]", "lowBattery", "[SG*8800000015*0002*AL]"},
		// No status bit set: AL messages get a general alarm.
		{"[SG*9051004074*0058*AL,120117,145602,V,40.058413,N,76.336618,W,11.519,188,99,00,01,80,0,50,00000000,0,1,0,0,,10]", "general", "[SG*9051004074*0002*AL]"},
		// Status alarms are reported on UD messages too, without an ack.
		{"[3G*1234567890*003e*UD,170525,214118,A,1,N,1,E,0,0,0,0,0,22,0,0,00010000,0]", "sos", ""},
	}
	for _, tt := range tests {
		msg, err := Decode(tt.raw)
		if err != nil || msg.Position == nil {
			t.Fatalf("decode %q: %v / %v", tt.raw, err, msg.PositionErr)
		}
		if got := msg.Position.Alarm(); got != tt.alarm {
			t.Errorf("%q: alarm got %q, want %q", tt.raw, got, tt.alarm)
		}
		if msg.Response != tt.response {
			t.Errorf("%q: response got %q, want %q", tt.raw, msg.Response, tt.response)
		}
	}
}

func TestDecode_AL_UnparseableStillAcknowledged(t *testing.T) {
	msg, err := Decode("[3G*1234567890*0006*AL,foo]")
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if msg.Position != nil || msg.PositionErr == nil {
		t.Errorf("expected position error, got pos=%v err=%v", msg.Position, msg.PositionErr)
	}
	if msg.Response != "[3G*1234567890*0002*AL]" {
		t.Errorf("response: got %q", msg.Response)
	}
}

func TestDecode_Network(t *testing.T) {
	t.Run("cells and wifi", func(t *testing.T) {
		msg, _ := Decode("[3G*9705141740*00C2*UD_LTE,260723,185105,V,00.000000,,00.0000000,,0.00,0.0,0.0,0,100,67,0,0,00000000,2,0,605,1,10006,65799,14,10020,4104,4,3,,34:60:f9:ec:19:f8,-82,,98:48:27:55:18:20,-96,,34:e8:94:e4:06:18,-104,0.0]")
		n := msg.Position.Network
		cells, _ := n["cellTowers"].([]map[string]any)
		wifis, _ := n["wifiAccessPoints"].([]map[string]any)
		if len(cells) != 2 || len(wifis) != 3 {
			t.Fatalf("got %d cells, %d wifis: %v", len(cells), len(wifis), n)
		}
		if cells[0]["mobileCountryCode"] != 605 || cells[0]["mobileNetworkCode"] != 1 {
			t.Errorf("cell[0]: %v", cells[0])
		}
		if wifis[2]["macAddress"] != "34:e8:94:e4:06:18" || wifis[2]["signalStrength"] != -104 {
			t.Errorf("wifi[2]: %v", wifis[2])
		}
	})

	t.Run("hex lac and cid", func(t *testing.T) {
		msg, _ := Decode("[SG*9059011020*0067*AL,240123,181628,V,54.427538,N,6.409275,W,0.00,0,0,0,19,90,0,0,00000000,1,1,234,10,55C0,3B882A2,132,,10]")
		cells, _ := msg.Position.Network["cellTowers"].([]map[string]any)
		if len(cells) != 1 || cells[0]["locationAreaCode"] != 0x55C0 || cells[0]["cellId"] != 0x3B882A2 {
			t.Errorf("cells: %v", cells)
		}
	})

	t.Run("alternative layout skipped", func(t *testing.T) {
		msg, _ := Decode("[SG*9059011020*006b*UD2,240123,162011,A,54.427621,N,6.409190,W,0.00,0,0,8,19,88,0,0,00000000,1,1,FFFF,FFFF,FFFE,3B882A2,132,,00]")
		if msg.Position == nil || msg.Position.Network != nil {
			t.Errorf("expected position without network, got %+v", msg.Position)
		}
	})

	t.Run("empty wifi entries ignored", func(t *testing.T) {
		msg, _ := Decode("[SG*9059056143*0053*UD,251021,223408,A,41.46500,N,081.53128,W,0.926,000,0,00,70,70,0,50,00000000,0,1,,,,00]")
		if msg.Position == nil || msg.Position.Network != nil {
			t.Errorf("expected position without network, got %+v", msg.Position)
		}
	})

	t.Run("truncated network keeps position", func(t *testing.T) {
		msg, _ := Decode("[3G*1*0001*UD,220414,134652,A,22.5,N,113.8,E,0.1,0.0,100,7,60,90,1000,50,0000,4,1,460,0,9360]")
		if msg.Position == nil || msg.Position.Network != nil {
			t.Errorf("expected position without network, got %+v / %v", msg.Position, msg.PositionErr)
		}
	})
}

func TestDecode_Heartbeat(t *testing.T) {
	tests := []struct {
		raw      string
		attrs    map[string]any
		response string
	}{
		{"[3G*4700186508*000B*LK,0,10,100]", map[string]any{"steps": 0, "batteryLevel": 100}, "[3G*4700186508*0002*LK]"},
		{"[3G*1234567890*000D*LK,1234,0,85]", map[string]any{"steps": 1234, "batteryLevel": 85}, "[3G*1234567890*0002*LK]"},
		// Fewer than three values carry no data (Traccar semantics).
		{"[SG*9081000548*0009*LK,0,100]", nil, "[SG*9081000548*0002*LK]"},
		{"[SG*8800000015*0002*LK]", nil, "[SG*8800000015*0002*LK]"},
		// Indexed frames echo the index.
		{"[ZJ*014111001350304*0034*0009*LK,0,0,19]", map[string]any{"steps": 0, "batteryLevel": 19}, "[ZJ*014111001350304*0034*0002*LK]"},
	}
	for _, tt := range tests {
		msg, err := Decode(tt.raw)
		if err != nil {
			t.Fatalf("decode %q: %v", tt.raw, err)
		}
		if msg.Type != "LK" || msg.Position != nil {
			t.Errorf("%q: type %q position %v", tt.raw, msg.Type, msg.Position)
		}
		if len(msg.Attributes) != len(tt.attrs) {
			t.Errorf("%q: attributes got %v, want %v", tt.raw, msg.Attributes, tt.attrs)
		}
		for k, v := range tt.attrs {
			if msg.Attributes[k] != v {
				t.Errorf("%q: %s got %v, want %v", tt.raw, k, msg.Attributes[k], v)
			}
		}
		if msg.Response != tt.response {
			t.Errorf("%q: response got %q, want %q", tt.raw, msg.Response, tt.response)
		}
	}
}

func TestDecode_Acknowledgements(t *testing.T) {
	tests := []struct{ raw, response string }{
		{"[3G*8800000015*0004*INIT]", "[3G*8800000015*0006*INIT,1]"},
		{"[3G*8800000015*0003*TKQ]", "[3G*8800000015*0003*TKQ]"},
		{"[3G*8800000015*0004*TKQ2]", "[3G*8800000015*0004*TKQ2]"},
		{"[3G*8308406279*0008*rcapture]", ""},
		{"[CS*8800000015*0008*PULSE,72]", ""},
	}
	for _, tt := range tests {
		msg, err := Decode(tt.raw)
		if err != nil {
			t.Fatalf("decode %q: %v", tt.raw, err)
		}
		if msg.Response != tt.response {
			t.Errorf("%q: response got %q, want %q", tt.raw, msg.Response, tt.response)
		}
	}
}

func TestDecode_Header(t *testing.T) {
	tests := []struct {
		raw, manufacturer, id, index, typ, content string
	}{
		{"[3G*1234567890*0002*LK]", "3G", "1234567890", "", "LK", ""},
		{"[ZJ*5678901234*0001*0009*TEMP,36.5]", "ZJ", "5678901234", "0001", "TEMP", "36.5"},
		{"[ZJ*357653059860416*0007*000c*BLOOD,109,68]", "ZJ", "357653059860416", "0007", "BLOOD", "109,68"},
		{"  [SG*1*0002*LK]\r\n", "SG", "1", "", "LK", ""},
	}
	for _, tt := range tests {
		msg, err := Decode(tt.raw)
		if err != nil {
			t.Fatalf("decode %q: %v", tt.raw, err)
		}
		got := []string{msg.Manufacturer, msg.DeviceID, msg.Index, msg.Type, msg.Content}
		want := []string{tt.manufacturer, tt.id, tt.index, tt.typ, tt.content}
		if strings.Join(got, "|") != strings.Join(want, "|") {
			t.Errorf("%q: got %q, want %q", tt.raw, got, want)
		}
	}
}

func TestDecode_EscapedContent(t *testing.T) {
	msg, err := Decode("[CS*1234567890*000e*TK,#!AMR}\x01}\x02}\x03}\x04}\x05]")
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if msg.Type != "TK" || msg.Content != "#!AMR}[],*" {
		t.Errorf("got type %q content %q", msg.Type, msg.Content)
	}
}

func TestDecode_InvalidFormat(t *testing.T) {
	for _, raw := range []string{
		"",
		"not a watch message",
		"[3G]",
		"[3G*1234567890]",
		"[3G*1234567890*00]",
		"[3G*1234567890*0002*]",
		"[3G*1234567890*0002*LK}]",
	} {
		if _, err := Decode(raw); err == nil {
			t.Errorf("Decode(%q): expected error", raw)
		}
	}
}

func TestDecode_PositionContentErrors(t *testing.T) {
	for _, raw := range []string{
		// Legacy DDMMYYYY dates are not part of the protocol.
		"[3G*1234567890*0078*UD,14022026,153045,A,49.814998,N,9.970177,E,15.50,270.0,0.0,8,100,460,0,9527,3661]",
		"[3G*1234567890*0010*UD,220414,134652,A,abc,N,113.8,E]",
		"[3G*1234567890*0002*UD]",
	} {
		msg, err := Decode(raw)
		if err != nil {
			t.Fatalf("Decode(%q): unexpected frame error %v", raw, err)
		}
		if msg.Position != nil || msg.PositionErr == nil {
			t.Errorf("Decode(%q): expected position error", raw)
		}
	}
}

func TestEncodeResponse(t *testing.T) {
	if got := EncodeResponse("3G", "1234567890", "", "LK"); got != "[3G*1234567890*0002*LK]" {
		t.Errorf("got %q", got)
	}
	if got := EncodeResponse("ZJ", "1", "0034", "INIT,1"); got != "[ZJ*1*0034*0006*INIT,1]" {
		t.Errorf("got %q", got)
	}
	// Length is lowercase hex.
	if got := EncodeResponse("3G", "1", "", strings.Repeat("x", 26)); !strings.HasPrefix(got, "[3G*1*001a*") {
		t.Errorf("got %q", got)
	}
}

func TestDecode_HealthMessages(t *testing.T) {
	tests := []struct {
		raw   string
		attrs map[string]any
	}{
		// Vectors from Traccar's WatchProtocolDecoderTest.
		{"[3G*9705141740*000B*oxygen,0,98]", map[string]any{"bloodOxygen": 98}},
		{"[ZJ*5678901234*0001*0009*TEMP,36.5]", map[string]any{"temp1": 36.5}},
		{"[3G*2104326058*000E*btemp2,1,35.29]", map[string]any{"temp1": 35.29}},
		{"[3G*4700609403*0013*bphrt,120,79,73,,,,]", map[string]any{"pressureHigh": "120", "pressureLow": "79", "heartRate": 73}},
		{"[ZJ*357653059860416*0007*000c*BLOOD,109,68]", map[string]any{"pressureHigh": "109", "pressureLow": "68"}},
		{"[CS*8800000015*0008*PULSE,72]", map[string]any{"heartRate": 72}},
		{"[3G*6005412902*0007*heart,0]", map[string]any{"heartRate": 0}},
		{"[3G*6005412902*0008*heart,71]", map[string]any{"heartRate": 71}},
		// Disabled body temperature measurement carries no value.
		{"[3G*2104326058*0009*btemp2,0]", nil},
		// No content or malformed values carry no data.
		{"[3G*6005412902*0005*HEART]", nil},
		{"[3G*6005412902*0009*heart,abc]", nil},
		{"[3G*9705141740*0008*oxygen,0]", nil},
	}
	for _, tt := range tests {
		msg, err := Decode(tt.raw)
		if err != nil {
			t.Fatalf("decode %q: %v", tt.raw, err)
		}
		if msg.Response != "" || msg.Position != nil {
			t.Errorf("%q: unexpected response %q / position %v", tt.raw, msg.Response, msg.Position)
		}
		if len(msg.Attributes) != len(tt.attrs) {
			t.Errorf("%q: attributes got %v, want %v", tt.raw, msg.Attributes, tt.attrs)
			continue
		}
		for k, v := range tt.attrs {
			if msg.Attributes[k] != v {
				t.Errorf("%q: %s got %v (%T), want %v (%T)", tt.raw, k, msg.Attributes[k], msg.Attributes[k], v, v)
			}
		}
	}
}

func TestDecode_MediaMessages(t *testing.T) {
	tests := []struct{ raw, response string }{
		// Voice message chunks are acknowledged per chunk (header layout
		// from Traccar's JXTK vector; binary AMR data is not stored).
		{"[ZJ*789468050042692*0034*0439*JXTK,0,watch_7_20220526093954,1,6,#!AMR\n\x0c\x0a<?\x96}\x04\xd9]", "[ZJ*789468050042692*0034*0007*JXTKR,1]"},
		{"[3G*1234567890*0020*JXTK,0,watch_1,6,6,#!AMR]", "[3G*1234567890*0007*JXTKR,1]"},
		// Malformed JXTK headers are not acknowledged.
		{"[3G*1234567890*0008*JXTK,0,x]", ""},
		{"[3G*1234567890*0010*JXTK,0,x,a,b,data]", ""},
		// Voice messages and images are not acknowledged.
		{"[CS*1234567890*000e*TK,#!AMR}\x01}\x02}\x03}\x04}\x05\xff]", ""},
		{"[3G*1234567890*0010*img,5,220414134652,\xff\xd8\xff]", ""},
	}
	for _, tt := range tests {
		msg, err := Decode(tt.raw)
		if err != nil {
			t.Fatalf("decode %q: %v", tt.raw, err)
		}
		if msg.Response != tt.response {
			t.Errorf("%q: response got %q, want %q", tt.raw, msg.Response, tt.response)
		}
		if msg.Position != nil || msg.Attributes != nil {
			t.Errorf("%q: unexpected data %v / %v", tt.raw, msg.Position, msg.Attributes)
		}
	}
}

func TestMessage_IsDeviceInitiated(t *testing.T) {
	tests := map[string]bool{
		// Sent by the device on its own.
		"[3G*1*0002*LK]":          true,
		"[3G*1*0004*INIT]":        true,
		"[3G*1*0003*TKQ]":         true,
		"[3G*1*0004*TKQ2]":        true,
		"[3G*1*0006*UD,foo]":      true,
		"[3G*1*0008*UD_LTE,foo]":  true,
		"[3G*1*0006*AL,foo]":      true,
		"[3G*1*0006*WT,foo]":      true,
		"[3G*1*0008*PULSE,72]":    true,
		"[3G*1*0007*heart,0]":     true,
		"[3G*1*000b*oxygen,0,98]": true,
		"[3G*1*000a*JXTK,0,a,1]":  true,
		"[3G*1*0004*TK,x]":        true,
		"[3G*1*0005*TK2,x]":       true,
		"[3G*1*0005*img,x]":       true,
		// Replies echoing a server command keyword.
		"[3G*1*0006*UPLOAD]":   false,
		"[3G*1*0002*CR]":       false,
		"[3G*1*0005*RESET]":    false,
		"[3G*1*0004*SOS1]":     false,
		"[3G*1*0008*POWEROFF]": false,
		"[3G*1*0008*rcapture]": false,
	}
	for raw, want := range tests {
		msg, err := Decode(raw)
		if err != nil {
			t.Fatalf("decode %q: %v", raw, err)
		}
		if got := msg.IsDeviceInitiated(); got != want {
			t.Errorf("%q: IsDeviceInitiated() = %v, want %v", raw, got, want)
		}
	}
}
