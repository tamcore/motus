package geo

import "math"

const earthRadiusKm = 6371.0

func toRadians(deg float64) float64 {
	return deg * math.Pi / 180.0
}

// HaversineDistance returns the great-circle distance between two points in km.
func HaversineDistance(lat1, lon1, lat2, lon2 float64) float64 {
	dLat := toRadians(lat2 - lat1)
	dLon := toRadians(lon2 - lon1)

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(toRadians(lat1))*math.Cos(toRadians(lat2))*
			math.Sin(dLon/2)*math.Sin(dLon/2)

	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return earthRadiusKm * c
}

// Bearing returns the initial bearing from (lat1,lon1) to (lat2,lon2) in degrees [0,360).
func Bearing(lat1, lon1, lat2, lon2 float64) float64 {
	dLon := toRadians(lon2 - lon1)
	lat1R := toRadians(lat1)
	lat2R := toRadians(lat2)
	y := math.Sin(dLon) * math.Cos(lat2R)
	x := math.Cos(lat1R)*math.Sin(lat2R) - math.Sin(lat1R)*math.Cos(lat2R)*math.Cos(dLon)
	return math.Mod(math.Atan2(y, x)*180.0/math.Pi+360, 360)
}
