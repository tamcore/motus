package calendar_test

import (
	"testing"
	"time"

	"github.com/tamcore/motus/internal/calendar"
)

// sampleICalOneTime is a simple one-time event from 9:00-17:00 UTC on Jan 15, 2026.
const sampleICalOneTime = `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Motus//Test//EN
BEGIN:VEVENT
DTSTART:20260115T090000Z
DTEND:20260115T170000Z
SUMMARY:Work Hours
END:VEVENT
END:VCALENDAR`

// sampleICalAllDay is an all-day event on Jan 15, 2026.
const sampleICalAllDay = `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Motus//Test//EN
BEGIN:VEVENT
DTSTART;VALUE=DATE:20260115
DTEND;VALUE=DATE:20260116
SUMMARY:All Day Event
END:VEVENT
END:VCALENDAR`

// sampleICalDuration is an event with DURATION instead of DTEND.
const sampleICalDuration = `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Motus//Test//EN
BEGIN:VEVENT
DTSTART:20260115T090000Z
DURATION:PT2H30M
SUMMARY:Meeting
END:VEVENT
END:VCALENDAR`

// sampleICalWeekly is a weekly recurring event on Mon, Wed, Fri from 8:00-18:00 UTC.
const sampleICalWeekly = `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Motus//Test//EN
BEGIN:VEVENT
DTSTART:20260112T080000Z
DTEND:20260112T180000Z
RRULE:FREQ=WEEKLY;BYDAY=MO,WE,FR
SUMMARY:Work Days
END:VEVENT
END:VCALENDAR`

// sampleICalDaily is a daily recurring event from 22:00-06:00 (overnight).
const sampleICalDaily = `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Motus//Test//EN
BEGIN:VEVENT
DTSTART:20260115T220000Z
DTEND:20260116T060000Z
RRULE:FREQ=DAILY;COUNT=30
SUMMARY:Night Shift
END:VEVENT
END:VCALENDAR`

// sampleICalWithUntil has a daily event that stops after Jan 20, 2026.
const sampleICalWithUntil = `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Motus//Test//EN
BEGIN:VEVENT
DTSTART:20260115T090000Z
DTEND:20260115T170000Z
RRULE:FREQ=DAILY;UNTIL=20260120T235959Z
SUMMARY:Limited Event
END:VEVENT
END:VCALENDAR`

func TestIsActiveAt_OneTimeEvent(t *testing.T) {
	tests := []struct {
		name   string
		time   time.Time
		active bool
	}{
		{
			name:   "during event",
			time:   time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC),
			active: true,
		},
		{
			name:   "at start",
			time:   time.Date(2026, 1, 15, 9, 0, 0, 0, time.UTC),
			active: true,
		},
		{
			name:   "before event",
			time:   time.Date(2026, 1, 15, 8, 59, 59, 0, time.UTC),
			active: false,
		},
		{
			name:   "at end (exclusive)",
			time:   time.Date(2026, 1, 15, 17, 0, 0, 0, time.UTC),
			active: false,
		},
		{
			name:   "after event",
			time:   time.Date(2026, 1, 15, 18, 0, 0, 0, time.UTC),
			active: false,
		},
		{
			name:   "wrong day",
			time:   time.Date(2026, 1, 16, 12, 0, 0, 0, time.UTC),
			active: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			active, err := calendar.IsActiveAt(sampleICalOneTime, tt.time)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if active != tt.active {
				t.Errorf("IsActiveAt(%v) = %v, want %v", tt.time, active, tt.active)
			}
		})
	}
}

func TestIsActiveAt_AllDayEvent(t *testing.T) {
	tests := []struct {
		name   string
		time   time.Time
		active bool
	}{
		{
			name:   "start of day",
			time:   time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
			active: true,
		},
		{
			name:   "middle of day",
			time:   time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC),
			active: true,
		},
		{
			name:   "end of day (exclusive, next day start)",
			time:   time.Date(2026, 1, 16, 0, 0, 0, 0, time.UTC),
			active: false,
		},
		{
			name:   "day before",
			time:   time.Date(2026, 1, 14, 23, 59, 59, 0, time.UTC),
			active: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			active, err := calendar.IsActiveAt(sampleICalAllDay, tt.time)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if active != tt.active {
				t.Errorf("IsActiveAt(%v) = %v, want %v", tt.time, active, tt.active)
			}
		})
	}
}

func TestIsActiveAt_DurationEvent(t *testing.T) {
	tests := []struct {
		name   string
		time   time.Time
		active bool
	}{
		{
			name:   "during event (start + 1h)",
			time:   time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC),
			active: true,
		},
		{
			name:   "at end (exclusive, start + 2h30m)",
			time:   time.Date(2026, 1, 15, 11, 30, 0, 0, time.UTC),
			active: false,
		},
		{
			name:   "before event",
			time:   time.Date(2026, 1, 15, 8, 0, 0, 0, time.UTC),
			active: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			active, err := calendar.IsActiveAt(sampleICalDuration, tt.time)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if active != tt.active {
				t.Errorf("IsActiveAt(%v) = %v, want %v", tt.time, active, tt.active)
			}
		})
	}
}

func TestIsActiveAt_WeeklyRecurrence(t *testing.T) {
	tests := []struct {
		name   string
		time   time.Time
		active bool
	}{
		{
			name:   "Monday 10:00 (active day)",
			time:   time.Date(2026, 1, 19, 10, 0, 0, 0, time.UTC), // Monday
			active: true,
		},
		{
			name:   "Wednesday 12:00 (active day)",
			time:   time.Date(2026, 1, 21, 12, 0, 0, 0, time.UTC), // Wednesday
			active: true,
		},
		{
			name:   "Friday 15:00 (active day)",
			time:   time.Date(2026, 1, 23, 15, 0, 0, 0, time.UTC), // Friday
			active: true,
		},
		{
			name:   "Tuesday 12:00 (inactive day)",
			time:   time.Date(2026, 1, 20, 12, 0, 0, 0, time.UTC), // Tuesday
			active: false,
		},
		{
			name:   "Saturday 12:00 (inactive day)",
			time:   time.Date(2026, 1, 24, 12, 0, 0, 0, time.UTC), // Saturday
			active: false,
		},
		{
			name:   "Monday 7:00 (before hours)",
			time:   time.Date(2026, 1, 19, 7, 0, 0, 0, time.UTC), // Monday
			active: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			active, err := calendar.IsActiveAt(sampleICalWeekly, tt.time)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if active != tt.active {
				t.Errorf("IsActiveAt(%v, weekday=%s) = %v, want %v",
					tt.time, tt.time.Weekday(), active, tt.active)
			}
		})
	}
}

func TestIsActiveAt_DailyRecurrence(t *testing.T) {
	tests := []struct {
		name   string
		time   time.Time
		active bool
	}{
		{
			name:   "first occurrence 23:00",
			time:   time.Date(2026, 1, 15, 23, 0, 0, 0, time.UTC),
			active: true,
		},
		{
			name:   "second occurrence next day 03:00",
			time:   time.Date(2026, 1, 16, 3, 0, 0, 0, time.UTC),
			active: true,
		},
		{
			name:   "daytime gap",
			time:   time.Date(2026, 1, 16, 12, 0, 0, 0, time.UTC),
			active: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			active, err := calendar.IsActiveAt(sampleICalDaily, tt.time)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if active != tt.active {
				t.Errorf("IsActiveAt(%v) = %v, want %v", tt.time, active, tt.active)
			}
		})
	}
}

func TestIsActiveAt_WithUntil(t *testing.T) {
	tests := []struct {
		name   string
		time   time.Time
		active bool
	}{
		{
			name:   "within range Jan 17 12:00",
			time:   time.Date(2026, 1, 17, 12, 0, 0, 0, time.UTC),
			active: true,
		},
		{
			name:   "last day Jan 20 12:00",
			time:   time.Date(2026, 1, 20, 12, 0, 0, 0, time.UTC),
			active: true,
		},
		{
			name:   "after UNTIL Jan 21 12:00",
			time:   time.Date(2026, 1, 21, 12, 0, 0, 0, time.UTC),
			active: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			active, err := calendar.IsActiveAt(sampleICalWithUntil, tt.time)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if active != tt.active {
				t.Errorf("IsActiveAt(%v) = %v, want %v", tt.time, active, tt.active)
			}
		})
	}
}

func TestIsActiveAt_EmptyData(t *testing.T) {
	_, err := calendar.IsActiveAt("", time.Now())
	if err == nil {
		t.Error("expected error for empty data")
	}
}

func TestIsActiveAt_InvalidData(t *testing.T) {
	_, err := calendar.IsActiveAt("not a valid ical", time.Now())
	// Note: arran4/golang-ical is lenient; it may not error on garbage input.
	// But it should not return active=true.
	if err == nil {
		// Check it returns false.
		active, _ := calendar.IsActiveAt("not a valid ical", time.Now())
		if active {
			t.Error("expected false for invalid data")
		}
	}
}

func TestValidate_Valid(t *testing.T) {
	err := calendar.Validate(sampleICalOneTime)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidate_ParseError(t *testing.T) {
	// "not valid" is non-empty but is not parseable iCalendar data, causing
	// ics.ParseCalendar to return "parsing calendar line 0".
	err := calendar.Validate("not valid")
	if err == nil {
		t.Error("expected error for unparseable iCalendar data")
	}
}

func TestValidate_Empty(t *testing.T) {
	err := calendar.Validate("")
	if err == nil {
		t.Error("expected error for empty data")
	}
}

func TestValidate_NoEvents(t *testing.T) {
	noEvents := `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Motus//Test//EN
END:VCALENDAR`

	err := calendar.Validate(noEvents)
	if err == nil {
		t.Error("expected error for calendar with no events")
	}
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		name     string
		ical     string
		checkAt  time.Time
		expected bool
	}{
		{
			name: "PT1H duration",
			ical: `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Test//EN
BEGIN:VEVENT
DTSTART:20260115T090000Z
DURATION:PT1H
SUMMARY:One Hour
END:VEVENT
END:VCALENDAR`,
			checkAt:  time.Date(2026, 1, 15, 9, 30, 0, 0, time.UTC),
			expected: true,
		},
		{
			name: "P1D duration",
			ical: `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Test//EN
BEGIN:VEVENT
DTSTART:20260115T000000Z
DURATION:P1D
SUMMARY:Full Day
END:VEVENT
END:VCALENDAR`,
			checkAt:  time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC),
			expected: true,
		},
		{
			name: "P1W duration",
			ical: `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Test//EN
BEGIN:VEVENT
DTSTART:20260115T000000Z
DURATION:P1W
SUMMARY:Week Long
END:VEVENT
END:VCALENDAR`,
			checkAt:  time.Date(2026, 1, 20, 12, 0, 0, 0, time.UTC),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			active, err := calendar.IsActiveAt(tt.ical, tt.checkAt)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if active != tt.expected {
				t.Errorf("IsActiveAt = %v, want %v", active, tt.expected)
			}
		})
	}
}

func TestIsActiveAt_NoStartEvent(t *testing.T) {
	// A VEVENT without DTSTART causes eventActiveAt to return an error; IsActiveAt
	// skips the event (continue) and returns false, nil.
	noStart := `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Test//EN
BEGIN:VEVENT
SUMMARY:No Start Time
END:VEVENT
END:VCALENDAR`

	active, err := calendar.IsActiveAt(noStart, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if active {
		t.Error("expected false for event with no DTSTART")
	}
}

func TestIsActiveAt_NoDTEndNoDuration_DateOnly(t *testing.T) {
	// No DTEND and no DURATION, DATE-only DTSTART → lasts one full day.
	noEnd := `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Test//EN
BEGIN:VEVENT
DTSTART;VALUE=DATE:20260115
SUMMARY:Date Only No End
END:VEVENT
END:VCALENDAR`

	// Middle of Jan 15 should be active.
	active, err := calendar.IsActiveAt(noEnd, time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !active {
		t.Error("expected active on Jan 15 for date-only event with no end")
	}

	// Jan 16 should be inactive (event lasted exactly one day).
	active, err = calendar.IsActiveAt(noEnd, time.Date(2026, 1, 16, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if active {
		t.Error("expected inactive on Jan 16 for date-only event with no end")
	}
}

func TestIsActiveAt_InstantaneousEvent(t *testing.T) {
	// No DTEND and no DURATION, DATE-TIME DTSTART → instantaneous (dtend == dtstart).
	instant := `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Test//EN
BEGIN:VEVENT
DTSTART:20260115T120000Z
SUMMARY:Instant
END:VEVENT
END:VCALENDAR`

	// Exactly at the moment → dtend == dtstart, so t.Before(dtend) is false.
	active, err := calendar.IsActiveAt(instant, time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if active {
		t.Error("expected inactive for instantaneous event (zero-length window)")
	}
}

func TestIsActiveAt_InvalidDuration(t *testing.T) {
	// Invalid DURATION causes getEventEnd to fail; eventActiveAt returns error; IsActiveAt skips.
	badDuration := `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Test//EN
BEGIN:VEVENT
DTSTART:20260115T090000Z
DURATION:invalid
SUMMARY:Bad Duration
END:VEVENT
END:VCALENDAR`

	active, err := calendar.IsActiveAt(badDuration, time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("unexpected error from IsActiveAt: %v", err)
	}
	if active {
		t.Error("expected false for event with invalid DURATION")
	}
}

func TestIsActiveAt_WeeklyWithInterval(t *testing.T) {
	// FREQ=WEEKLY;INTERVAL=2 (biweekly), BYDAY=MO.
	biweekly := `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Test//EN
BEGIN:VEVENT
DTSTART:20260112T080000Z
DTEND:20260112T180000Z
RRULE:FREQ=WEEKLY;INTERVAL=2;BYDAY=MO
SUMMARY:Biweekly Monday
END:VEVENT
END:VCALENDAR`

	// Jan 12 (Mon) 10:00 — first occurrence.
	active, err := calendar.IsActiveAt(biweekly, time.Date(2026, 1, 12, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !active {
		t.Error("expected active on first occurrence Jan 12")
	}

	// Jan 19 (Mon) 10:00 — skipped week (interval=2).
	active, err = calendar.IsActiveAt(biweekly, time.Date(2026, 1, 19, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if active {
		t.Error("expected inactive on Jan 19 (skipped by INTERVAL=2)")
	}

	// Jan 26 (Mon) 10:00 — second occurrence.
	active, err = calendar.IsActiveAt(biweekly, time.Date(2026, 1, 26, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !active {
		t.Error("expected active on second occurrence Jan 26")
	}
}

func TestIsActiveAt_WeeklyBYDAYWithUntil(t *testing.T) {
	// FREQ=WEEKLY;BYDAY=MO,FR;UNTIL=20260120T235959Z
	// Week of Jan 19-25: Monday Jan 19 is within UNTIL, Friday Jan 23 is after UNTIL.
	// The Friday occurrence should be skipped (continue in isActiveInWeeklyByDay).
	limited := `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Test//EN
BEGIN:VEVENT
DTSTART:20260112T080000Z
DTEND:20260112T180000Z
RRULE:FREQ=WEEKLY;BYDAY=MO,FR;UNTIL=20260120T235959Z
SUMMARY:Limited MO/FR
END:VEVENT
END:VCALENDAR`

	// Monday Jan 19 10:00 — within UNTIL.
	active, err := calendar.IsActiveAt(limited, time.Date(2026, 1, 19, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !active {
		t.Error("expected active on Mon Jan 19 (within UNTIL)")
	}

	// Friday Jan 23 10:00 — after UNTIL.
	active, err = calendar.IsActiveAt(limited, time.Date(2026, 1, 23, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if active {
		t.Error("expected inactive on Fri Jan 23 (after UNTIL)")
	}
}

func TestIsActiveAt_RRULEWithDateUNTIL(t *testing.T) {
	// UNTIL in DATE format (no time component) — exercises the fallback parse path.
	untilDate := `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Test//EN
BEGIN:VEVENT
DTSTART:20260115T090000Z
DTEND:20260115T170000Z
RRULE:FREQ=DAILY;UNTIL=20260118
SUMMARY:Date UNTIL
END:VEVENT
END:VCALENDAR`

	// Jan 17 12:00 — within UNTIL range.
	active, err := calendar.IsActiveAt(untilDate, time.Date(2026, 1, 17, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !active {
		t.Error("expected active on Jan 17 (within date UNTIL)")
	}
}

func TestIsActiveAt_RRULEWithUnparseableUNTIL(t *testing.T) {
	// UNTIL value that cannot be parsed in any format → isActiveInRecurrence returns false, nil.
	badUntil := `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Test//EN
BEGIN:VEVENT
DTSTART:20260115T090000Z
DTEND:20260115T170000Z
RRULE:FREQ=DAILY;UNTIL=BADVALUE
SUMMARY:Bad UNTIL
END:VEVENT
END:VCALENDAR`

	// Use Jan 16 12:00 — outside the first occurrence window so RRULE is evaluated.
	active, err := calendar.IsActiveAt(badUntil, time.Date(2026, 1, 16, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if active {
		t.Error("expected false when UNTIL cannot be parsed")
	}
}

func TestIsActiveAt_WeeklyBYDAYPastUNTIL(t *testing.T) {
	// FREQ=WEEKLY;BYDAY=MO;UNTIL=20260119 (Jan 19 = Monday).
	// Checking Jan 28 (more than a week after UNTIL) exercises the
	// "weekStart > UNTIL" break in isActiveInWeeklyByDay.
	pastUntil := `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Test//EN
BEGIN:VEVENT
DTSTART:20260112T080000Z
DTEND:20260112T180000Z
RRULE:FREQ=WEEKLY;BYDAY=MO;UNTIL=20260119T235959Z
SUMMARY:Monday Until Jan 19
END:VEVENT
END:VCALENDAR`

	active, err := calendar.IsActiveAt(pastUntil, time.Date(2026, 1, 28, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if active {
		t.Error("expected inactive on Jan 28 (well after UNTIL=Jan 19)")
	}
}

// Traccar-format iCalendar data (base64-encoded in Traccar, but we store raw).
func TestIsActiveAt_TraccarStyleCalendar(t *testing.T) {
	// Traccar stores calendars as iCalendar data. This tests a realistic
	// Traccar-compatible calendar with RRULE.
	traccarCal := `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Traccar//Traccar//EN
BEGIN:VEVENT
DTSTART:20260101T080000Z
DTEND:20260101T200000Z
RRULE:FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR
SUMMARY:Business Hours
END:VEVENT
END:VCALENDAR`

	// Thursday Jan 15, 2026 at 10:00 UTC -- should be active.
	active, err := calendar.IsActiveAt(traccarCal, time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !active {
		t.Error("expected active during business hours on Thursday")
	}

	// Saturday Jan 17, 2026 at 10:00 UTC -- should not be active.
	active, err = calendar.IsActiveAt(traccarCal, time.Date(2026, 1, 17, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if active {
		t.Error("expected inactive on Saturday")
	}
}
