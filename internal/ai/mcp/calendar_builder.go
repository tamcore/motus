package mcp

import (
	"fmt"
	"strings"
	"time"
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

// BuildICalendar generates an RFC 5545 iCalendar string from spec.
// CalendarService validates the result and the name on create.
func BuildICalendar(spec CalendarSpec) (string, error) {
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
			"SUMMARY:"+icalEscaper.Replace(spec.Name),
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
		start, err := time.Parse("15:04", *spec.DailyStartTime)
		if err != nil {
			return "", fmt.Errorf("invalid daily_start_time: %w", err)
		}
		end, err := time.Parse("15:04", *spec.DailyEndTime)
		if err != nil {
			return "", fmt.Errorf("invalid daily_end_time: %w", err)
		}
		if !end.After(start) {
			return "", fmt.Errorf("daily_end_time must be after daily_start_time")
		}

		now := time.Now().UTC()
		dtstart := time.Date(now.Year(), now.Month(), now.Day(), start.Hour(), start.Minute(), 0, 0, time.UTC)
		dtend := time.Date(now.Year(), now.Month(), now.Day(), end.Hour(), end.Minute(), 0, 0, time.UTC)
		uid := fmt.Sprintf("motus-weekly-%d@motus", now.UnixNano())
		lines = append(lines,
			"BEGIN:VEVENT",
			"UID:"+uid,
			"SUMMARY:"+icalEscaper.Replace(spec.Name),
			"DTSTART:"+dtstart.Format("20060102T150405Z"),
			"DTEND:"+dtend.Format("20060102T150405Z"),
			"RRULE:FREQ=WEEKLY;BYDAY="+strings.Join(upperDays, ","),
			"END:VEVENT",
		)

	default:
		return "", fmt.Errorf("provide either (start_time + end_time) or (weekdays + daily_start_time + daily_end_time)")
	}

	lines = append(lines, "END:VCALENDAR")
	return strings.Join(lines, "\r\n"), nil
}

var icalEscaper = strings.NewReplacer(`\`, `\\`, ";", `\;`, ",", `\,`, "\n", `\n`)
