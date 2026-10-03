package repository

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestGeometryError(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		wantInvalid bool
	}{
		{"wkt parse error", &pgconn.PgError{Code: "XX000", Message: "parse error - invalid geometry"}, true},
		{"unknown geojson type", &pgconn.PgError{Code: "XX000", Message: "unknown GeoJSON type"}, true},
		{"too few points", &pgconn.PgError{Code: "XX000", Message: "geometry requires more points"}, true},
		{"internal fault stays generic", &pgconn.PgError{Code: "XX000", Message: "cache lookup failed for type 12345"}, false},
		{"other sqlstate stays generic", &pgconn.PgError{Code: "23505", Message: "parse error - duplicate"}, false},
		{"non-pg error stays generic", errors.New("connection reset"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := geometryError("create geofence", tt.err)
			if errors.Is(got, ErrInvalidGeometry) != tt.wantInvalid {
				t.Fatalf("geometryError(%v) = %v; ErrInvalidGeometry = %v, want %v", tt.err, got, errors.Is(got, ErrInvalidGeometry), tt.wantInvalid)
			}
		})
	}
}
