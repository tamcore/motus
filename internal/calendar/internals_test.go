package calendar

import (
	"testing"
	"time"

	ics "github.com/arran4/golang-ical"
)

func TestIsDateOnly(t *testing.T) {
	// nil property → false
	if isDateOnly(nil) {
		t.Error("isDateOnly(nil) should return false")
	}

	// Property with VALUE=DATE parameter → true
	withValueDate := &ics.IANAProperty{
		BaseProperty: ics.BaseProperty{
			ICalParameters: map[string][]string{
				"VALUE": {"DATE"},
			},
			Value: "20260115",
		},
	}
	if !isDateOnly(withValueDate) {
		t.Error("isDateOnly with VALUE=DATE should return true")
	}

	// Property with 8-char value and no T → true (DATE format by length)
	byLength := &ics.IANAProperty{
		BaseProperty: ics.BaseProperty{
			ICalParameters: map[string][]string{},
			Value:          "20260115",
		},
	}
	if !isDateOnly(byLength) {
		t.Error("isDateOnly with 8-char value without T should return true")
	}

	// Property with DATE-TIME value (contains T) → false
	withTime := &ics.IANAProperty{
		BaseProperty: ics.BaseProperty{
			ICalParameters: map[string][]string{},
			Value:          "20260115T090000Z",
		},
	}
	if isDateOnly(withTime) {
		t.Error("isDateOnly with DATE-TIME value should return false")
	}

	// Property with VALUE=DATE-TIME parameter → false
	withValueDateTime := &ics.IANAProperty{
		BaseProperty: ics.BaseProperty{
			ICalParameters: map[string][]string{
				"VALUE": {"DATE-TIME"},
			},
			Value: "20260115T090000Z",
		},
	}
	if isDateOnly(withValueDateTime) {
		t.Error("isDateOnly with VALUE=DATE-TIME should return false")
	}
}

func TestParseICalTime_Nil(t *testing.T) {
	_, err := parseICalTime(nil)
	if err == nil {
		t.Error("expected error for nil property")
	}
}

func TestParseICalTime_InvalidFormat(t *testing.T) {
	prop := &ics.IANAProperty{
		BaseProperty: ics.BaseProperty{
			ICalParameters: map[string][]string{},
			Value:          "not-a-date",
		},
	}
	_, err := parseICalTime(prop)
	if err == nil {
		t.Error("expected error for unparseable time value")
	}
}

func TestParseICalTime_WithValidTZID(t *testing.T) {
	prop := &ics.IANAProperty{
		BaseProperty: ics.BaseProperty{
			ICalParameters: map[string][]string{
				"TZID": {"America/New_York"},
			},
			Value: "20260115T090000",
		},
	}
	got, err := parseICalTime(prop)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	loc, _ := time.LoadLocation("America/New_York")
	want := time.Date(2026, 1, 15, 9, 0, 0, 0, loc)
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestParseICalTime_WithInvalidTZID(t *testing.T) {
	// Invalid TZID should fall back to UTC.
	prop := &ics.IANAProperty{
		BaseProperty: ics.BaseProperty{
			ICalParameters: map[string][]string{
				"TZID": {"Not/ATimezone"},
			},
			Value: "20260115T090000",
		},
	}
	got, err := parseICalTime(prop)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Location() != time.UTC {
		t.Errorf("expected UTC fallback, got %v", got.Location())
	}
}

func TestParseDuration_EdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    time.Duration
		wantErr bool
	}{
		{"empty", "", 0, true},
		{"no P prefix", "1H", 0, true},
		{"negative PT1H", "-PT1H", -time.Hour, false},
		{"P1DT2H3M4S", "P1DT2H3M4S", 26*time.Hour + 3*time.Minute + 4*time.Second, false},
		{"P2W", "P2W", 14 * 24 * time.Hour, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseDuration(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseDuration(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("parseDuration(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseBYDAY_NumericPrefix(t *testing.T) {
	// "1MO" means first Monday of the month; the numeric prefix should be stripped.
	days := parseBYDAY("1MO,2TU,-1FR")
	if len(days) != 3 {
		t.Fatalf("expected 3 days, got %d: %v", len(days), days)
	}
	if days[0] != time.Monday {
		t.Errorf("expected Monday, got %v", days[0])
	}
	if days[1] != time.Tuesday {
		t.Errorf("expected Tuesday, got %v", days[1])
	}
	if days[2] != time.Friday {
		t.Errorf("expected Friday, got %v", days[2])
	}
}

func TestIsActiveInRecurrence_MissingFREQ(t *testing.T) {
	dtstart := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	dtend := dtstart.Add(time.Hour)
	_, err := isActiveInRecurrence("COUNT=5", dtstart, dtend, dtstart.Add(30*time.Minute))
	if err == nil {
		t.Error("expected error for RRULE missing FREQ")
	}
}

func TestIsActiveInRecurrence_WithINTERVAL(t *testing.T) {
	// Every 2 days starting Jan 15.
	dtstart := time.Date(2026, 1, 15, 9, 0, 0, 0, time.UTC)
	dtend := dtstart.Add(time.Hour)

	// Jan 17 9:30 should be active (2nd occurrence: Jan 15 + 2 days).
	active, err := isActiveInRecurrence("FREQ=DAILY;INTERVAL=2", dtstart, dtend,
		time.Date(2026, 1, 17, 9, 30, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !active {
		t.Error("expected active on Jan 17 9:30 (INTERVAL=2)")
	}

	// Jan 16 9:30 should be inactive (skipped interval).
	active, err = isActiveInRecurrence("FREQ=DAILY;INTERVAL=2", dtstart, dtend,
		time.Date(2026, 1, 16, 9, 30, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if active {
		t.Error("expected inactive on Jan 16 9:30 (skipped by INTERVAL=2)")
	}
}

func TestAdvanceOccurrence(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		freq     string
		interval int
		wantDate time.Time
	}{
		{"DAILY", 1, base.AddDate(0, 0, 1)},
		{"WEEKLY", 1, base.AddDate(0, 0, 7)},
		{"MONTHLY", 1, base.AddDate(0, 1, 0)},
		{"YEARLY", 1, base.AddDate(1, 0, 0)},
		{"UNKNOWN", 1, base.AddDate(0, 0, 1)}, // default fallback
	}

	for _, tt := range tests {
		got := advanceOccurrence(base, tt.freq, tt.interval)
		if !got.Equal(tt.wantDate) {
			t.Errorf("advanceOccurrence(%s, %d): got %v, want %v", tt.freq, tt.interval, got, tt.wantDate)
		}
	}
}
