package notification

import (
	"fmt"
	"strings"

	"github.com/tamcore/motus/internal/model"
)

// TemplateContext holds the data available for variable substitution in templates.
type TemplateContext struct {
	Device   *model.Device
	Event    *model.Event
	Geofence *model.Geofence
	Position *model.Position
}

// RenderTemplate replaces {{variable}} placeholders in a template string
// with values from the provided context.
func RenderTemplate(template string, ctx *TemplateContext) string {
	var pairs []string

	if ctx.Device != nil {
		pairs = append(pairs,
			"{{device.id}}", fmt.Sprintf("%d", ctx.Device.ID),
			"{{device.name}}", ctx.Device.Name,
			"{{device.uniqueId}}", ctx.Device.UniqueID,
			"{{device.status}}", ctx.Device.Status,
		)
	}

	if ctx.Event != nil {
		pairs = append(pairs,
			"{{event.id}}", fmt.Sprintf("%d", ctx.Event.ID),
			"{{event.type}}", ctx.Event.Type,
			"{{event.timestamp}}", ctx.Event.Timestamp.Format("2006-01-02 15:04:05"),
		)
	}

	if ctx.Geofence != nil {
		pairs = append(pairs,
			"{{geofence.id}}", fmt.Sprintf("%d", ctx.Geofence.ID),
			"{{geofence.name}}", ctx.Geofence.Name,
		)
	}

	if ctx.Position != nil {
		pairs = append(pairs,
			"{{position.latitude}}", fmt.Sprintf("%.6f", ctx.Position.Latitude),
			"{{position.longitude}}", fmt.Sprintf("%.6f", ctx.Position.Longitude),
		)
		if ctx.Position.Speed != nil {
			pairs = append(pairs, "{{position.speed}}", fmt.Sprintf("%.1f", *ctx.Position.Speed))
		}
		if ctx.Position.Altitude != nil {
			pairs = append(pairs, "{{position.altitude}}", fmt.Sprintf("%.1f", *ctx.Position.Altitude))
		}
		if ctx.Position.Course != nil {
			pairs = append(pairs, "{{position.course}}", fmt.Sprintf("%.1f", *ctx.Position.Course))
		}
	}

	return strings.NewReplacer(pairs...).Replace(template)
}
