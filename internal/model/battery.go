package model

// BatteryLevel returns the battery charge in percent (0–100) reported in the
// "batteryLevel" position attribute, or nil when the attribute is missing or
// not a valid percentage. Voltage attributes (e.g. "battery", "power") are
// deliberately not converted: there is no reliable voltage-to-percent mapping.
func BatteryLevel(attrs map[string]any) *float64 {
	var level float64
	switch v := attrs["batteryLevel"].(type) {
	case int:
		level = float64(v)
	case float64:
		level = v
	default:
		return nil
	}
	if level < 0 || level > 100 {
		return nil
	}
	return &level
}
