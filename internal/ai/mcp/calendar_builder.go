package mcp

import (
	"fmt"
	"strings"
	"time"

	"github.com/tamcore/motus/internal/calendar"
)

// CalendarSpec describes what kind of iCalendar event to generate.
// Exactly one of the two mode pairs must be set.
type CalendarSpec struct {
	Name string
	// One-shot mode: a single event from StartTime to EndTime.
	StartTime *time.Time
	EndTime   *time.Time
	// Weekly recurring mode: repeats on the given weekdays at the given daily window.
	Weekdays       []string // MO, TU, WE, TH, FR, SA, SU
	DailyStartTime *string  // "HH:MM" UTC
	DailyEndTime   *string  // "HH:MM" UTC
}

var validWeekdaySet = map[string]bool{
	"MO": true, "TU": true, "WE": true,
	"TH": true, "FR": true, "SA": true, "SU": true,
}

// BuildICalendar generates a valid RFC 5545 iCalendar string from spec and
// validates it using the existing calendar.Validate function.
func BuildICalendar(spec CalendarSpec) (string, error) {
	if spec.Name == "" {
		return "", fmt.Errorf("name is required")
	}

	var lines []string
	lines = append(lines, "BEGIN:VCALENDAR", "VERSION:2.0", "PRODID:-//motus//AI//EN")

	switch {
	case spec.StartTime != nil && spec.EndTime != nil:
		if !spec.EndTime.After(*spec.StartTime) {
			return "", fmt.Errorf("end_time must be after start_time")
		}
		uid := fmt.Sprintf("motus-once-%d@motus", spec.StartTime.UnixNano())
		lines = append(lines,
			"BEGIN:VEVENT",
			"UID:"+uid,
			"SUMMARY:"+icalEscape(spec.Name),
			"DTSTART:"+spec.StartTime.UTC().Format("20060102T150405Z"),
			"DTEND:"+spec.EndTime.UTC().Format("20060102T150405Z"),
			"END:VEVENT",
		)

	case len(spec.Weekdays) > 0 && spec.DailyStartTime != nil && spec.DailyEndTime != nil:
		upperDays := make([]string, 0, len(spec.Weekdays))
		for _, wd := range spec.Weekdays {
			u := strings.ToUpper(strings.TrimSpace(wd))
			if !validWeekdaySet[u] {
				return "", fmt.Errorf("invalid weekday %q (use MO TU WE TH FR SA SU)", wd)
			}
			upperDays = append(upperDays, u)
		}
		startH, startM, err := parseHHMM(*spec.DailyStartTime)
		if err != nil {
			return "", fmt.Errorf("invalid daily_start_time: %w", err)
		}
		endH, endM, err := parseHHMM(*spec.DailyEndTime)
		if err != nil {
			return "", fmt.Errorf("invalid daily_end_time: %w", err)
		}
		if endH*60+endM <= startH*60+startM {
			return "", fmt.Errorf("daily_end_time must be after daily_start_time")
		}

		now := time.Now().UTC()
		dtstart := time.Date(now.Year(), now.Month(), now.Day(), startH, startM, 0, 0, time.UTC)
		dtend := time.Date(now.Year(), now.Month(), now.Day(), endH, endM, 0, 0, time.UTC)
		uid := fmt.Sprintf("motus-weekly-%d@motus", now.UnixNano())
		lines = append(lines,
			"BEGIN:VEVENT",
			"UID:"+uid,
			"SUMMARY:"+icalEscape(spec.Name),
			"DTSTART:"+dtstart.Format("20060102T150405Z"),
			"DTEND:"+dtend.Format("20060102T150405Z"),
			"RRULE:FREQ=WEEKLY;BYDAY="+strings.Join(upperDays, ","),
			"END:VEVENT",
		)

	default:
		return "", fmt.Errorf("provide either (start_time + end_time) or (weekdays + daily_start_time + daily_end_time)")
	}

	lines = append(lines, "END:VCALENDAR")
	ical := strings.Join(lines, "\r\n")

	if err := calendar.Validate(ical); err != nil {
		return "", fmt.Errorf("generated invalid iCalendar: %w", err)
	}
	return ical, nil
}

func parseHHMM(s string) (h, m int, err error) {
	if _, err = fmt.Sscanf(s, "%d:%d", &h, &m); err != nil {
		return 0, 0, fmt.Errorf("expected HH:MM, got %q", s)
	}
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, 0, fmt.Errorf("time out of range: %q", s)
	}
	return h, m, nil
}

func icalEscape(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, ";", "\\;")
	s = strings.ReplaceAll(s, ",", "\\,")
	s = strings.ReplaceAll(s, "\n", "\\n")
	return s
}
