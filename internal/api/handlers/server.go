package handlers

import (
	"net/http"

	"github.com/tamcore/motus/internal/api"
)

// ServerHandler handles server information endpoints.
type ServerHandler struct{}

// NewServerHandler creates a new server handler.
func NewServerHandler() *ServerHandler {
	return &ServerHandler{}
}

// GetServer returns Traccar-compatible server information.
// Required by pytraccar and Traccar Manager for initialization.
// GET /api/server
func (h *ServerHandler) GetServer(w http.ResponseWriter, r *http.Request) {
	server := map[string]interface{}{
		"id":               1,
		"registration":     true,
		"readonly":         false,
		"deviceReadonly":   false,
		"limitCommands":    false,
		"map":              "osm",
		"bingKey":          "",
		"mapUrl":           "",
		"poiLayer":         "",
		"latitude":         49.79,
		"longitude":        9.95,
		"zoom":             13,
		"version":          "3.0.0",
		"forceSettings":    false,
		"coordinateFormat": "dd",
		"openIdEnabled":    false,
		"openIdForce":      false,
		"attributes":       map[string]interface{}{},
	}

	api.RespondJSON(w, http.StatusOK, server)
}
