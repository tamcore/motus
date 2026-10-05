// Package calendar provides iCalendar (RFC 5545) parsing and schedule checking.
// It wraps the arran4/golang-ical library with a focused API for determining
// whether a given time falls within any VEVENT in an iCalendar document.
package calendar

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	ics "github.com/arran4/golang-ical"
)

// IsActiveAt checks whether the given iCalendar data has any VEVENT that
// is active (i.e., the time t falls between DTSTART and DTEND) at time t.
//
// If the iCalendar data is empty or cannot be parsed, it returns false
// with an error. If no events are defined, it returns false (no error).
//
// This function handles:
//   - Simple one-time events (DTSTART + DTEND)
//   - All-day events (DATE format without time component)
//   - Events with DURATION instead of DTEND
//   - Recurring events (RRULE) are evaluated by expanding occurrences
func IsActiveAt(icalData string, t time.Time) (bool, error) {
	if strings.TrimSpace(icalData) == "" {
		return false, fmt.Errorf("empty iCalendar data")
	}

	cal, err := ics.ParseCalendar(strings.NewReader(icalData))
	if err != nil {
		return false, fmt.Errorf("parse iCalendar: %w", err)
	}

	for _, event := range cal.Events() {
		active, err := eventActiveAt(event, t)
		if err != nil {
			continue // Skip malformed events.
		}
		if active {
			return true, nil
		}
	}

	return false, nil
}

// Validate checks whether the given iCalendar data is valid and contains
// at least one VEVENT component.
func Validate(icalData string) error {
	if strings.TrimSpace(icalData) == "" {
		return fmt.Errorf("empty iCalendar data")
	}

	cal, err := ics.ParseCalendar(strings.NewReader(icalData))
	if err != nil {
		return fmt.Errorf("parse iCalendar: %w", err)
	}

	if len(cal.Events()) == 0 {
		return fmt.Errorf("no VEVENT components found")
	}

	return nil
}

// eventActiveAt checks whether a single VEVENT is active at time t.
func eventActiveAt(event *ics.VEvent, t time.Time) (bool, error) {
	dtstart, err := getEventStart(event)
	if err != nil {
		return false, fmt.Errorf("parse DTSTART: %w", err)
	}

	dtend, err := getEventEnd(event, dtstart)
	if err != nil {
		return false, fmt.Errorf("determine event end: %w", err)
	}

	// Check if t falls within [dtstart, dtend).
	if !t.Before(dtstart) && t.Before(dtend) {
		return true, nil
	}

	// Check RRULE for recurring events.
	rruleProp := event.GetProperty(ics.ComponentPropertyRrule)
	if rruleProp != nil {
		return isActiveInRecurrence(rruleProp.Value, dtstart, dtend, t)
	}

	return false, nil
}

// getEventStart extracts the start time from a VEVENT.
func getEventStart(event *ics.VEvent) (time.Time, error) {
	prop := event.GetProperty(ics.ComponentPropertyDtStart)
	if prop == nil {
		return time.Time{}, fmt.Errorf("DTSTART not found")
	}
	return parseICalTime(prop)
}

// getEventEnd extracts the end time from a VEVENT. If DTEND is not present,
// it tries DURATION. If neither exists, it uses the RFC 5545 defaults.
func getEventEnd(event *ics.VEvent, dtstart time.Time) (time.Time, error) {
	// Try DTEND first.
	endProp := event.GetProperty(ics.ComponentPropertyDtEnd)
	if endProp != nil {
		return parseICalTime(endProp)
	}

	// Try DURATION.
	durProp := event.GetProperty(ics.ComponentPropertyDuration)
	if durProp != nil {
		dur, err := parseDuration(durProp.Value)
		if err != nil {
			return time.Time{}, fmt.Errorf("parse DURATION: %w", err)
		}
		return dtstart.Add(dur), nil
	}

	// Per RFC 5545: no DTEND and no DURATION means:
	// - DATE type: event lasts one day
	// - DATE-TIME type: event is instantaneous
	startProp := event.GetProperty(ics.ComponentPropertyDtStart)
	if isDateOnly(startProp) {
		return dtstart.Add(24 * time.Hour), nil
	}

	// Instantaneous event.
	return dtstart, nil
}

// parseICalTime parses a time from an iCalendar property, handling TZID,
// UTC suffix, and DATE-only formats.
func parseICalTime(prop *ics.IANAProperty) (time.Time, error) {
	if prop == nil {
		return time.Time{}, fmt.Errorf("nil property")
	}

	t, err := ParseValue(prop.Value)
	if err != nil {
		return t, err
	}
	if tzid := prop.ICalParameters["TZID"]; len(tzid) > 0 && tzid[0] != "" && !strings.HasSuffix(prop.Value, "Z") {
		if loc, err := time.LoadLocation(tzid[0]); err == nil {
			t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), 0, loc)
		}
	}
	return t, nil
}

// ParseValue parses a bare iCalendar DATE-TIME or DATE value. Values without
// a zone are returned as UTC.
func ParseValue(val string) (time.Time, error) {
	for _, layout := range []string{"20060102T150405Z", "20060102T150405", "20060102"} {
		if t, err := time.Parse(layout, val); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("cannot parse time value %q", val)
}

// isDateOnly checks if a property uses DATE (not DATE-TIME) format.
func isDateOnly(prop *ics.IANAProperty) bool {
	if prop == nil {
		return false
	}
	// Check VALUE parameter.
	valueParam := prop.ICalParameters["VALUE"]
	if len(valueParam) > 0 && valueParam[0] == "DATE" {
		return true
	}
	// Also detect by format: 8 digits without T.
	return len(prop.Value) == 8 && !strings.Contains(prop.Value, "T")
}

// parseDuration parses an iCalendar DURATION value (e.g., "PT1H", "P1D", "PT30M").
func parseDuration(val string) (time.Duration, error) {
	if val == "" {
		return 0, fmt.Errorf("empty duration")
	}

	negative := false
	s := val
	if strings.HasPrefix(s, "-") {
		negative = true
		s = s[1:]
	}
	if !strings.HasPrefix(s, "P") {
		return 0, fmt.Errorf("duration must start with P: %q", val)
	}
	s = s[1:] // Remove "P".

	var d time.Duration

	datePart, timePart, _ := strings.Cut(s, "T")

	// Parse date part (W, D).
	if datePart != "" {
		d += parseDurationSegments(datePart, map[byte]time.Duration{
			'W': 7 * 24 * time.Hour,
			'D': 24 * time.Hour,
		})
	}

	// Parse time part (H, M, S).
	if timePart != "" {
		d += parseDurationSegments(timePart, map[byte]time.Duration{
			'H': time.Hour,
			'M': time.Minute,
			'S': time.Second,
		})
	}

	if negative {
		d = -d
	}

	return d, nil
}

// parseDurationSegments parses segments like "1H30M" using the given suffix->duration map.
func parseDurationSegments(s string, suffixes map[byte]time.Duration) time.Duration {
	var d time.Duration
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] >= '0' && s[i] <= '9' {
			continue
		}
		if n, err := strconv.Atoi(s[start:i]); err == nil {
			d += time.Duration(n) * suffixes[s[i]]
		}
		start = i + 1
	}
	return d
}

// isActiveInRecurrence evaluates a basic RRULE to determine if time t falls
// within any recurrence of the event. Supports FREQ=DAILY, WEEKLY, MONTHLY, YEARLY
// with optional INTERVAL, UNTIL/COUNT, and BYDAY.
func isActiveInRecurrence(rrule string, dtstart, dtend time.Time, t time.Time) (bool, error) {
	params := parseRRULE(rrule)

	freq := params["FREQ"]
	if freq == "" {
		return false, fmt.Errorf("RRULE missing FREQ")
	}

	interval := 1
	if n, err := strconv.Atoi(params["INTERVAL"]); err == nil && n > 0 {
		interval = n
	}

	// Determine the event duration.
	eventDuration := dtend.Sub(dtstart)

	// Parse UNTIL or COUNT for termination.
	var until *time.Time
	maxCount := 0
	if v, ok := params["UNTIL"]; ok {
		parsed, err := ParseValue(v)
		if err != nil {
			return false, nil
		}
		until = &parsed
	}
	if v, ok := params["COUNT"]; ok {
		maxCount, _ = strconv.Atoi(v)
	}

	// Parse BYDAY for WEEKLY frequency.
	var byDay []time.Weekday
	if v, ok := params["BYDAY"]; ok {
		byDay = parseBYDAY(v)
	}

	// Without COUNT, occurrence indexes do not matter, so start near t:
	// the expansion cap would otherwise end long-running rules.
	if maxCount == 0 {
		dtstart = skipPeriods(dtstart, eventDuration, freq, interval, t)
	}

	// Expand occurrences up to time t.
	// Limit expansion to prevent unbounded iteration.
	maxExpansions := 1000
	if maxCount > 0 && maxCount < maxExpansions {
		maxExpansions = maxCount
	}

	// For WEEKLY with BYDAY, we expand each week's specified days individually.
	if freq == "WEEKLY" && len(byDay) > 0 {
		return isActiveInWeeklyByDay(dtstart, eventDuration, byDay, interval, until, maxExpansions, t)
	}

	occurrence := dtstart
	for i := 0; i < maxExpansions; i++ {
		if until != nil && occurrence.After(*until) {
			break
		}

		occEnd := occurrence.Add(eventDuration)

		// All future occurrences are after t; no match.
		if t.Before(occurrence) {
			break
		}

		if !t.Before(occurrence) && t.Before(occEnd) {
			return true, nil
		}

		// Advance to next occurrence.
		occurrence = advanceOccurrence(occurrence, freq, interval)
	}

	return false, nil
}

// parseRRULE splits an RRULE string into key=value pairs.
func parseRRULE(rrule string) map[string]string {
	result := make(map[string]string)
	for part := range strings.SplitSeq(rrule, ";") {
		if k, v, ok := strings.Cut(part, "="); ok {
			result[k] = v
		}
	}
	return result
}

// parseBYDAY parses BYDAY values like "MO,WE,FR" into Go Weekday values.
func parseBYDAY(val string) []time.Weekday {
	dayMap := map[string]time.Weekday{
		"SU": time.Sunday,
		"MO": time.Monday,
		"TU": time.Tuesday,
		"WE": time.Wednesday,
		"TH": time.Thursday,
		"FR": time.Friday,
		"SA": time.Saturday,
	}

	var days []time.Weekday
	for d := range strings.SplitSeq(val, ",") {
		d = strings.TrimSpace(d)
		// Strip numeric prefix (e.g., "1MO" -> "MO").
		for len(d) > 2 && d[0] >= '0' && d[0] <= '9' {
			d = d[1:]
		}
		if len(d) > 2 {
			d = d[len(d)-2:]
		}
		if wd, ok := dayMap[strings.ToUpper(d)]; ok {
			days = append(days, wd)
		}
	}
	return days
}

// isActiveInWeeklyByDay expands weekly recurrence with BYDAY by checking each
// specified day of the week for each recurring week. For example, BYDAY=MO,WE,FR
// means the event occurs on Monday, Wednesday, and Friday of every Nth week.
func isActiveInWeeklyByDay(dtstart time.Time, eventDuration time.Duration, byDay []time.Weekday, interval int, until *time.Time, maxExpansions int, t time.Time) (bool, error) {
	// Calculate time-of-day offset from the original DTSTART.
	startHour, startMin, startSec := dtstart.Clock()
	startOffset := time.Duration(startHour)*time.Hour + time.Duration(startMin)*time.Minute + time.Duration(startSec)*time.Second

	// Find the start of the week containing dtstart (Monday-based for iCal default).
	weekStart := dtstart.AddDate(0, 0, -int(dtstart.Weekday()))

	for range maxExpansions {
		if until != nil && weekStart.After(*until) {
			break
		}

		// Check each BYDAY within this week.
		for _, wd := range byDay {
			dayOffset := int(wd) // Sunday=0, Monday=1, etc.
			occurrence := weekStart.AddDate(0, 0, dayOffset)
			// Apply the same time-of-day as DTSTART.
			occurrence = time.Date(occurrence.Year(), occurrence.Month(), occurrence.Day(),
				0, 0, 0, 0, dtstart.Location()).Add(startOffset)

			occEnd := occurrence.Add(eventDuration)

			if until != nil && occurrence.After(*until) {
				continue
			}

			if !t.Before(occurrence) && t.Before(occEnd) {
				return true, nil
			}
		}

		// Move past t? All future weeks are after t.
		nextWeekStart := weekStart.AddDate(0, 0, 7*interval)
		if t.Before(nextWeekStart) {
			break
		}
		weekStart = nextWeekStart
	}

	return false, nil
}

// skipPeriods moves dtstart forward by whole DAILY or WEEKLY periods, keeping
// one period of margin before the last occurrence that could still cover t.
func skipPeriods(dtstart time.Time, eventDuration time.Duration, freq string, interval int, t time.Time) time.Time {
	periodDays := map[string]int{"DAILY": 1, "WEEKLY": 7}[freq] * interval
	if periodDays == 0 {
		return dtstart
	}
	n := int(t.Sub(dtstart.Add(eventDuration)).Hours()/24)/periodDays - 1
	if n <= 0 {
		return dtstart
	}
	return dtstart.AddDate(0, 0, n*periodDays)
}

// advanceOccurrence moves an occurrence forward by the given frequency and interval.
func advanceOccurrence(t time.Time, freq string, interval int) time.Time {
	switch freq {
	case "DAILY":
		return t.AddDate(0, 0, interval)
	case "WEEKLY":
		return t.AddDate(0, 0, 7*interval)
	case "MONTHLY":
		return t.AddDate(0, interval, 0)
	case "YEARLY":
		return t.AddDate(interval, 0, 0)
	default:
		// Unsupported frequency; advance by 1 day as fallback.
		return t.AddDate(0, 0, 1)
	}
}
