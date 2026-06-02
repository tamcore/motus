package mcp

import (
	"strings"
	"testing"
	"time"
)

func TestBuildICalendar_OneShot(t *testing.T) {
	start := time.Date(2026, 6, 6, 18, 0, 0, 0, time.UTC)
	end := time.Date(2026, 6, 6, 23, 59, 0, 0, time.UTC)
	spec := CalendarSpec{Name: "Friday night", StartTime: &start, EndTime: &end}

	ical, err := BuildICalendar(spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(ical, "BEGIN:VCALENDAR") {
		t.Error("missing BEGIN:VCALENDAR")
	}
	if !strings.Contains(ical, "BEGIN:VEVENT") {
		t.Error("missing BEGIN:VEVENT")
	}
	if !strings.Contains(ical, "DTSTART:20260606T180000Z") {
		t.Errorf("missing expected DTSTART, got:\n%s", ical)
	}
	if !strings.Contains(ical, "DTEND:20260606T235900Z") {
		t.Errorf("missing expected DTEND, got:\n%s", ical)
	}
	if strings.Contains(ical, "RRULE") {
		t.Error("one-shot should not have RRULE")
	}
}

func TestBuildICalendar_WeeklyRecurring(t *testing.T) {
	ds := "08:00"
	de := "18:00"
	spec := CalendarSpec{
		Name:           "Work week",
		Weekdays:       []string{"MO", "TU", "WE", "TH", "FR"},
		DailyStartTime: &ds,
		DailyEndTime:   &de,
	}

	ical, err := BuildICalendar(spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(ical, "RRULE:FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR") {
		t.Errorf("missing expected RRULE, got:\n%s", ical)
	}
}

func TestBuildICalendar_WeekdaysCaseInsensitive(t *testing.T) {
	ds := "09:00"
	de := "17:00"
	spec := CalendarSpec{
		Name:           "Weekend",
		Weekdays:       []string{"sa", "su"},
		DailyStartTime: &ds,
		DailyEndTime:   &de,
	}

	ical, err := BuildICalendar(spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(ical, "BYDAY=SA,SU") {
		t.Errorf("weekdays should be uppercased, got:\n%s", ical)
	}
}

func TestBuildICalendar_InvalidWeekday(t *testing.T) {
	ds := "09:00"
	de := "17:00"
	spec := CalendarSpec{
		Name:           "Bad days",
		Weekdays:       []string{"MO", "XX"},
		DailyStartTime: &ds,
		DailyEndTime:   &de,
	}
	_, err := BuildICalendar(spec)
	if err == nil {
		t.Fatal("expected error for invalid weekday")
	}
}

func TestBuildICalendar_EndBeforeStart(t *testing.T) {
	start := time.Date(2026, 6, 6, 20, 0, 0, 0, time.UTC)
	end := time.Date(2026, 6, 6, 18, 0, 0, 0, time.UTC)
	spec := CalendarSpec{Name: "Bad window", StartTime: &start, EndTime: &end}
	_, err := BuildICalendar(spec)
	if err == nil {
		t.Fatal("expected error when end_time <= start_time")
	}
}

func TestBuildICalendar_DailyEndBeforeStart(t *testing.T) {
	ds := "18:00"
	de := "08:00"
	spec := CalendarSpec{
		Name:           "Inverted daily",
		Weekdays:       []string{"MO"},
		DailyStartTime: &ds,
		DailyEndTime:   &de,
	}
	_, err := BuildICalendar(spec)
	if err == nil {
		t.Fatal("expected error when daily_end_time <= daily_start_time")
	}
}

func TestBuildICalendar_MissingName(t *testing.T) {
	start := time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 6, 7, 0, 0, 0, 0, time.UTC)
	spec := CalendarSpec{StartTime: &start, EndTime: &end}
	_, err := BuildICalendar(spec)
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestBuildICalendar_NoMode(t *testing.T) {
	_, err := BuildICalendar(CalendarSpec{Name: "orphan"})
	if err == nil {
		t.Fatal("expected error when neither one-shot nor recurring args provided")
	}
}

func TestBuildICalendar_InvalidHHMM(t *testing.T) {
	bad := "25:99"
	spec := CalendarSpec{
		Name:           "Bad time",
		Weekdays:       []string{"MO"},
		DailyStartTime: &bad,
		DailyEndTime:   &bad,
	}
	_, err := BuildICalendar(spec)
	if err == nil {
		t.Fatal("expected error for out-of-range HH:MM")
	}
}

func TestIcalEscape(t *testing.T) {
	cases := []struct{ in, want string }{
		{"hello", "hello"},
		{"a;b", "a\\;b"},
		{"a,b", "a\\,b"},
		{"a\\b", "a\\\\b"},
		{"a\nb", "a\\nb"},
	}
	for _, c := range cases {
		if got := icalEscape(c.in); got != c.want {
			t.Errorf("icalEscape(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
