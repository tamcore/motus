package model

import "testing"

func TestBatteryLevel(t *testing.T) {
	tests := []struct {
		name  string
		attrs map[string]any
		want  *float64
	}{
		{"nil attributes", nil, nil},
		{"missing", map[string]any{"motion": true}, nil},
		{"int (watch, osmand json)", map[string]any{"batteryLevel": 53}, new(53.0)},
		{"float (osmand query)", map[string]any{"batteryLevel": 87.5}, new(87.5)},
		{"zero is a reading", map[string]any{"batteryLevel": 0}, new(0.0)},
		{"full", map[string]any{"batteryLevel": 100}, new(100.0)},
		{"above 100 rejected", map[string]any{"batteryLevel": 101}, nil},
		{"negative rejected", map[string]any{"batteryLevel": -1.0}, nil},
		{"string ignored", map[string]any{"batteryLevel": "80"}, nil},
		{"voltage only is not a percentage", map[string]any{"battery": 3.9, "power": 12.1}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BatteryLevel(tt.attrs)
			switch {
			case tt.want == nil && got != nil:
				t.Errorf("BatteryLevel = %v, want nil", *got)
			case tt.want != nil && got == nil:
				t.Errorf("BatteryLevel = nil, want %v", *tt.want)
			case tt.want != nil && *got != *tt.want:
				t.Errorf("BatteryLevel = %v, want %v", *got, *tt.want)
			}
		})
	}
}
