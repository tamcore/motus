package notification

import (
	"testing"
	"time"

	"github.com/tamcore/motus/internal/model"
)

func TestRenderTemplate(t *testing.T) {
	speed := 65.5
	altitude := 120.3
	course := 180.0

	ctx := &TemplateContext{
		Device: &model.Device{
			ID:       42,
			Name:     "GT3 RS",
			UniqueID: "123456789012345",
			Status:   "online",
		},
		Event: &model.Event{
			ID:        100,
			Type:      "geofenceEnter",
			Timestamp: time.Date(2026, 2, 14, 10, 30, 0, 0, time.UTC),
		},
		Geofence: &model.Geofence{
			ID:   5,
			Name: "Home",
		},
		Position: &model.Position{
			Latitude:  52.520008,
			Longitude: 13.404954,
			Speed:     &speed,
			Altitude:  &altitude,
			Course:    &course,
		},
	}

	tests := []struct {
		name     string
		template string
		want     string
	}{
		{
			name:     "device variables",
			template: "Device {{device.name}} ({{device.uniqueId}}) is {{device.status}}",
			want:     "Device GT3 RS (123456789012345) is online",
		},
		{
			name:     "event variables",
			template: "Event #{{event.id}}: {{event.type}} at {{event.timestamp}}",
			want:     "Event #100: geofenceEnter at 2026-02-14 10:30:00",
		},
		{
			name:     "geofence variables",
			template: "{{device.name}} entered {{geofence.name}} (id={{geofence.id}})",
			want:     "GT3 RS entered Home (id=5)",
		},
		{
			name:     "position variables",
			template: "Location: {{position.latitude}}, {{position.longitude}} at {{position.speed}} km/h",
			want:     "Location: 52.520008, 13.404954 at 65.5 km/h",
		},
		{
			name:     "altitude and course",
			template: "Alt: {{position.altitude}}m, Course: {{position.course}}",
			want:     "Alt: 120.3m, Course: 180.0",
		},
		{
			name:     "mixed variables",
			template: `{"device":"{{device.name}}","event":"{{event.type}}","geofence":"{{geofence.name}}"}`,
			want:     `{"device":"GT3 RS","event":"geofenceEnter","geofence":"Home"}`,
		},
		{
			name:     "no variables",
			template: "Plain text with no variables",
			want:     "Plain text with no variables",
		},
		{
			name:     "unknown variables left as-is",
			template: "{{unknown.var}} stays",
			want:     "{{unknown.var}} stays",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RenderTemplate(tt.template, ctx)
			if got != tt.want {
				t.Errorf("RenderTemplate() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRenderTemplate_NilContext(t *testing.T) {
	ctx := &TemplateContext{}
	template := "{{device.name}} entered {{geofence.name}}"
	got := RenderTemplate(template, ctx)
	// Variables with nil context objects are left as-is.
	if got != template {
		t.Errorf("RenderTemplate() with nil context = %q, want %q", got, template)
	}
}

func TestRenderTemplate_NilSpeed(t *testing.T) {
	ctx := &TemplateContext{
		Position: &model.Position{
			Latitude:  52.0,
			Longitude: 13.0,
		},
	}
	template := "Speed: {{position.speed}}"
	got := RenderTemplate(template, ctx)
	// Speed is nil, so the variable is not replaced.
	if got != "Speed: {{position.speed}}" {
		t.Errorf("RenderTemplate() with nil speed = %q, want unreplaced variable", got)
	}
}
