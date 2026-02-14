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
	result := template

	if ctx.Device != nil {
		result = strings.ReplaceAll(result, "{{device.id}}", fmt.Sprintf("%d", ctx.Device.ID))
		result = strings.ReplaceAll(result, "{{device.name}}", ctx.Device.Name)
		result = strings.ReplaceAll(result, "{{device.uniqueId}}", ctx.Device.UniqueID)
		result = strings.ReplaceAll(result, "{{device.status}}", ctx.Device.Status)
	}

	if ctx.Event != nil {
		result = strings.ReplaceAll(result, "{{event.id}}", fmt.Sprintf("%d", ctx.Event.ID))
		result = strings.ReplaceAll(result, "{{event.type}}", ctx.Event.Type)
		result = strings.ReplaceAll(result, "{{event.timestamp}}", ctx.Event.Timestamp.Format("2006-01-02 15:04:05"))
	}

	if ctx.Geofence != nil {
		result = strings.ReplaceAll(result, "{{geofence.id}}", fmt.Sprintf("%d", ctx.Geofence.ID))
		result = strings.ReplaceAll(result, "{{geofence.name}}", ctx.Geofence.Name)
	}

	if ctx.Position != nil {
		result = strings.ReplaceAll(result, "{{position.latitude}}", fmt.Sprintf("%.6f", ctx.Position.Latitude))
		result = strings.ReplaceAll(result, "{{position.longitude}}", fmt.Sprintf("%.6f", ctx.Position.Longitude))
		if ctx.Position.Speed != nil {
			result = strings.ReplaceAll(result, "{{position.speed}}", fmt.Sprintf("%.1f", *ctx.Position.Speed))
		}
		if ctx.Position.Altitude != nil {
			result = strings.ReplaceAll(result, "{{position.altitude}}", fmt.Sprintf("%.1f", *ctx.Position.Altitude))
		}
		if ctx.Position.Course != nil {
			result = strings.ReplaceAll(result, "{{position.course}}", fmt.Sprintf("%.1f", *ctx.Position.Course))
		}
	}

	return result
}
