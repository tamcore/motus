package mcp

import (
	"fmt"
	"strings"
	"time"

	ics "github.com/arran4/golang-ical"
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

// BuildICalendar generates an RFC 5545 iCalendar string from spec.
// CalendarService validates the result and the name on create.
func BuildICalendar(spec CalendarSpec) (string, error) {
	cal := ics.NewCalendar()
	cal.SetProductId("-//motus//AI//EN")

	switch {
	case spec.StartTime != nil && spec.EndTime != nil:
		if !spec.EndTime.After(*spec.StartTime) {
			return "", fmt.Errorf("end_time must be after start_time")
		}
		event := cal.AddEvent(fmt.Sprintf("motus-once-%d@motus", spec.StartTime.UnixNano()))
		event.SetSummary(spec.Name)
		event.SetStartAt(*spec.StartTime)
		event.SetEndAt(*spec.EndTime)

	case len(spec.Weekdays) > 0 && spec.DailyStartTime != nil && spec.DailyEndTime != nil:
		upperDays := make([]string, 0, len(spec.Weekdays))
		for _, wd := range spec.Weekdays {
			upperDays = append(upperDays, strings.ToUpper(strings.TrimSpace(wd)))
		}
		rrule := "FREQ=WEEKLY;BYDAY=" + strings.Join(upperDays, ",")
		if _, err := ics.ParseRecurrenceRule(rrule); err != nil {
			return "", fmt.Errorf("invalid weekdays %v (use MO TU WE TH FR SA SU): %w", spec.Weekdays, err)
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
		event := cal.AddEvent(fmt.Sprintf("motus-weekly-%d@motus", now.UnixNano()))
		event.SetSummary(spec.Name)
		event.SetStartAt(time.Date(now.Year(), now.Month(), now.Day(), start.Hour(), start.Minute(), 0, 0, time.UTC))
		event.SetEndAt(time.Date(now.Year(), now.Month(), now.Day(), end.Hour(), end.Minute(), 0, 0, time.UTC))
		event.AddRrule(rrule)

	default:
		return "", fmt.Errorf("provide either (start_time + end_time) or (weekdays + daily_start_time + daily_end_time)")
	}

	return cal.Serialize(ics.WithNewLineWindows), nil
}
