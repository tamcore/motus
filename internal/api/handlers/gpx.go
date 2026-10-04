package handlers

import (
	"context"
	"encoding/xml"
	"io"

	"github.com/tamcore/motus/internal/api"
	oas "github.com/tamcore/motus/internal/api/oas"
	"github.com/tamcore/motus/internal/audit"
	"github.com/tamcore/motus/internal/demo"
	"github.com/tamcore/motus/internal/geo"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
)

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

	imported, _, lastPos := processGPXPoints(ctx, h.cfg.Positions, params.ID, &gpxFile)
	if imported == 0 {
		return &oas.ImportGPXBadRequest{Error: "no timed positions found in GPX file"}, nil
	}

	// Update device LastUpdate and PositionID if the imported track is newer.
	if lastPos != nil {
		if device, err := h.cfg.Devices.GetByID(ctx, params.ID); err == nil {
			if device.LastUpdate == nil || lastPos.Timestamp.After(*device.LastUpdate) {
				device.LastUpdate = &lastPos.Timestamp
				device.PositionID = &lastPos.ID
				_ = h.cfg.Devices.Update(ctx, device)
			}
		}
	}

	h.cfg.AuditLogger.Log(ctx, &user.ID,
		audit.ActionGPXImport, audit.ResourceDevice, &params.ID,
		map[string]any{"deviceId": params.ID, "positions": imported}, "", "")

	return &oas.ImportGPXOK{
		Imported: oas.OptInt{Value: imported, Set: true},
	}, nil
}

// processGPXPoints iterates all GPX trackpoints, inserts timed ones as
// positions, and returns the imported count, skipped count, and the last
// inserted position. Speed is stored in km/h (internal unit) and calculated
// from haversine distance divided by elapsed time between consecutive timed
// points.
func processGPXPoints(ctx context.Context, positions repository.PositionRepo, deviceID int64, gpxFile *demo.GPXFile) (imported, skipped int, lastPos *model.Position) {
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
						distM := geo.HaversineDistance(prevLat, prevLon, pt.Lat, pt.Lon) * 1000
						spd = (distM / float64(dt)) * 3.6 // m/s → km/h
					}
					crs = geo.Bearing(prevLat, prevLon, pt.Lat, pt.Lon)
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
					Attributes: map[string]any{"source": "gpx"},
				}

				if err := positions.Create(ctx, pos); err != nil {
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
