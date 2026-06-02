// Package mcp provides an in-process MCP tool registry for the motus AI
// feature. Tools are registered at startup and invoked directly by the chat
// orchestrator — no HTTP transport is involved.
package mcp

import (
	"github.com/mark3labs/mcp-go/server"
	"github.com/tamcore/motus/internal/geocoding"
	"github.com/tamcore/motus/internal/services"
	"github.com/tamcore/motus/internal/storage/repository"
)

// Deps holds the dependencies required to register all MCP tools.
type Deps struct {
	Devices         repository.DeviceRepo
	Positions       repository.PositionRepo
	Events          repository.EventRepo
	Geofences       repository.GeofenceRepo
	GeofenceService *services.GeofenceService
	ForwardGeocoder geocoding.ForwardGeocoder
}

// NewServer creates and returns a configured in-process MCP server with all
// motus tools registered.
func NewServer(deps Deps) *server.MCPServer {
	s := server.NewMCPServer("motus", "1.0")
	registerTools(s, deps)
	return s
}
