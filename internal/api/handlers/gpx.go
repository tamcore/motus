package handlers

import (
	"context"
	"encoding/xml"
	"io"
	"math"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/tamcore/motus/internal/api"
	oas "github.com/tamcore/motus/internal/api/oas"
	"github.com/tamcore/motus/internal/audit"
	"github.com/tamcore/motus/internal/demo"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
)

// GPXImportHandler handles importing GPX track files into device position history.
type GPXImportHandler struct {
	devices   repository.DeviceRepo
	positions repository.PositionRepo
	audit     *audit.Logger
}

// NewGPXImportHandler creates a new GPX import handler.
func NewGPXImportHandler(devices repository.DeviceRepo, positions repository.PositionRepo, auditLogger *audit.Logger) *GPXImportHandler {
	return &GPXImportHandler{
		devices:   devices,
		positions: positions,
		audit:     auditLogger,
	}
}

// Import handles POST /api/devices/{id}/gpx.
// Accepts a multipart/form-data request with a "file" field containing GPX data.
// Returns {"imported": N, "skipped": M} where M is the count of points without <time>.
func (h *GPXImportHandler) Import(w http.ResponseWriter, r *http.Request) {
	user := api.UserFromContext(r.Context())

	deviceID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid device id")
		return
	}

	if !h.devices.UserHasAccess(r.Context(), user, deviceID) {
		api.RespondError(w, http.StatusForbidden, "access denied")
		return
	}

	// Limit raw body to 32 MB to prevent memory exhaustion before multipart parsing.
	r.Body = http.MaxBytesReader(w, r.Body, 32<<20)
	if err := r.ParseMultipartForm(16 << 20); err != nil {
		api.RespondError(w, http.StatusBadRequest, "failed to parse form")
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		api.RespondError(w, http.StatusBadRequest, "missing file field")
		return
	}
	defer func() { _ = file.Close() }()

	data, err := io.ReadAll(file)
	if err != nil {
		api.RespondError(w, http.StatusBadRequest, "failed to read file")
		return
	}

	var gpxFile demo.GPXFile
	if err := xml.Unmarshal(data, &gpxFile); err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid GPX file")
		return
	}

	imported, skipped, lastPos := h.processPoints(r, deviceID, &gpxFile)
	if imported == 0 {
		api.RespondError(w, http.StatusBadRequest, "no timed positions found in GPX file")
		return
	}

	// Update device LastUpdate and PositionID if the imported track is newer.
	if lastPos != nil {
		if device, err := h.devices.GetByID(r.Context(), deviceID); err == nil {
			if device.LastUpdate == nil || lastPos.Timestamp.After(*device.LastUpdate) {
				device.LastUpdate = &lastPos.Timestamp
				device.PositionID = &lastPos.ID
				_ = h.devices.Update(r.Context(), device)
			}
		}
	}

	if h.audit != nil {
		h.audit.LogFromRequest(r, &user.ID, audit.ActionGPXImport, audit.ResourceDevice, &deviceID,
			map[string]interface{}{"deviceId": deviceID, "positions": imported})
	}

	api.RespondJSON(w, http.StatusOK, map[string]int{"imported": imported, "skipped": skipped})
}

// processPoints iterates all GPX trackpoints, inserts timed ones as positions,
// and returns the imported count, skipped count, and the last inserted position.
// Speed is stored in km/h (internal unit) and calculated from haversine distance
// divided by elapsed time between consecutive timed points.
func (h *GPXImportHandler) processPoints(r *http.Request, deviceID int64, gpxFile *demo.GPXFile) (imported, skipped int, lastPos *model.Position) {
	var prevLat, prevLon float64
	var prevUnix int64

	for _, track := range gpxFile.Tracks {
		for _, seg := range track.Segments {
			for _, pt := range seg.Points {
				if pt.Time.IsZero() {
					skipped++
					continue
				}

				var spd, crs float64
				if prevUnix > 0 {
					dt := pt.Time.Unix() - prevUnix
					if dt > 0 {
						distM := gpxHaversine(prevLat, prevLon, pt.Lat, pt.Lon)
						spd = (distM / float64(dt)) * 3.6 // m/s → km/h
					}
					crs = gpxBearing(prevLat, prevLon, pt.Lat, pt.Lon)
				}

				alt := pt.Ele
				pos := &model.Position{
					DeviceID:   deviceID,
					Protocol:   "gpx",
					Timestamp:  pt.Time,
					Valid:      true,
					Latitude:   pt.Lat,
					Longitude:  pt.Lon,
					Altitude:   &alt,
					Speed:      &spd,
					Course:     &crs,
					Attributes: map[string]interface{}{"source": "gpx"},
				}

				if err := h.positions.Create(r.Context(), pos); err != nil {
					skipped++
					continue
				}

				imported++
				lastPos = pos
				prevLat = pt.Lat
				prevLon = pt.Lon
				prevUnix = pt.Time.Unix()
			}
		}
	}
	return
}

// gpxHaversine returns the great-circle distance between two coordinates in metres.
func gpxHaversine(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371000.0
	dLat := (lat2 - lat1) * math.Pi / 180.0
	dLon := (lon2 - lon1) * math.Pi / 180.0
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*math.Pi/180.0)*math.Cos(lat2*math.Pi/180.0)*
			math.Sin(dLon/2)*math.Sin(dLon/2)
	return R * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

// gpxBearing returns the initial bearing from (lat1,lon1) to (lat2,lon2) in degrees [0,360).
func gpxBearing(lat1, lon1, lat2, lon2 float64) float64 {
	dLon := (lon2 - lon1) * math.Pi / 180.0
	lat1R := lat1 * math.Pi / 180.0
	lat2R := lat2 * math.Pi / 180.0
	y := math.Sin(dLon) * math.Cos(lat2R)
	x := math.Cos(lat1R)*math.Sin(lat2R) - math.Sin(lat1R)*math.Cos(lat2R)*math.Cos(dLon)
	return math.Mod(math.Atan2(y, x)*180.0/math.Pi+360, 360)
}

// --- ogen Handler methods ---

// ImportGPX handles the ogen ImportGPX endpoint.
// Supports both multipart/form-data and application/gpx+xml content types.
func (h *Handler) ImportGPX(ctx context.Context, req oas.ImportGPXReq, params oas.ImportGPXParams) (oas.ImportGPXRes, error) {
	user := api.UserFromContext(ctx)
	if user == nil {
		return &oas.ImportGPXUnauthorized{Error: "unauthorized"}, nil
	}

	if !h.cfg.Devices.UserHasAccess(ctx, user, params.ID) {
		return &oas.ImportGPXForbidden{Error: "access denied"}, nil
	}

	var gpxData []byte
	switch v := req.(type) {
	case *oas.ImportGPXReqMultipartFormData:
		if !v.File.Set {
			return &oas.ImportGPXBadRequest{Error: "missing file field"}, nil
		}
		var err error
		gpxData, err = io.ReadAll(v.File.Value.File)
		if err != nil {
			return &oas.ImportGPXBadRequest{Error: "failed to read file"}, nil
		}
	case *oas.ImportGPXReqApplicationGpxXML:
		var err error
		gpxData, err = io.ReadAll(v.Data)
		if err != nil {
			return &oas.ImportGPXBadRequest{Error: "failed to read body"}, nil
		}
	default:
		return &oas.ImportGPXBadRequest{Error: "unsupported content type"}, nil
	}

	var gpxFile demo.GPXFile
	if err := xml.Unmarshal(gpxData, &gpxFile); err != nil {
		return &oas.ImportGPXBadRequest{Error: "invalid GPX file"}, nil
	}

	gpxHandler := &GPXImportHandler{
		devices:   h.cfg.Devices,
		positions: h.cfg.Positions,
		audit:     h.cfg.AuditLogger,
	}
	// processPoints requires an *http.Request for audit logging; pass a minimal one.
	r, _ := http.NewRequestWithContext(ctx, http.MethodPost, "/", nil)
	imported, _, lastPos := gpxHandler.processPoints(r, params.ID, &gpxFile)
	if imported == 0 {
		return &oas.ImportGPXBadRequest{Error: "no timed positions found in GPX file"}, nil
	}

	if lastPos != nil {
		if device, err := h.cfg.Devices.GetByID(ctx, params.ID); err == nil {
			if device.LastUpdate == nil || lastPos.Timestamp.After(*device.LastUpdate) {
				device.LastUpdate = &lastPos.Timestamp
				device.PositionID = &lastPos.ID
				_ = h.cfg.Devices.Update(ctx, device)
			}
		}
	}

	if h.cfg.AuditLogger != nil {
		id := params.ID
		h.cfg.AuditLogger.Log(ctx, &user.ID,
			audit.ActionGPXImport, audit.ResourceDevice, &id,
			map[string]interface{}{"deviceId": params.ID, "positions": imported}, "", "")
	}

	return &oas.ImportGPXOK{
		Imported: oas.OptInt{Value: imported, Set: true},
	}, nil
}
