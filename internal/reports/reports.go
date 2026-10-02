// Package reports detects trips and stops from a time-ordered position stream
// in constant memory. Speeds are in km/h (internal storage unit).
package reports

import (
	"fmt"
	"math"
	"time"

	"github.com/tamcore/motus/internal/model"
)

const (
	tripSpeedThreshold = 5.0 // km/h
	tripMinStop        = 300 * time.Second
	tripMinDuration    = 60 * time.Second
	tripMaxGap         = time.Hour

	stopSpeedThreshold = 1.0 // km/h
	stopMinDuration    = 300 * time.Second

	earthRadiusKm = 6371.0
)

// Trip is a detected trip. Duration is in seconds, Distance in km, speeds in km/h.
type Trip struct {
	StartTime, EndTime time.Time
	Duration           float64
	Distance           float64
	AvgSpeed           float64
	MaxSpeed           float64
}

// Stop is a detected stop. Duration is in seconds.
type Stop struct {
	Latitude, Longitude        float64
	Address                    string
	ArrivalTime, DepartureTime time.Time
	Duration                   float64
}

func speedOf(p *model.Position) float64 {
	if p.Speed == nil {
		return 0
	}
	return *p.Speed
}

func haversineKm(lat1, lon1, lat2, lon2 float64) float64 {
	rad := math.Pi / 180
	dLat := (lat2 - lat1) * rad
	dLon := (lon2 - lon1) * rad
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return earthRadiusKm * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

// tripAcc accumulates the aggregates of the trip in progress.
type tripAcc struct {
	count              int
	start, end         time.Time
	lastLat, lastLon   float64
	distance, maxSpeed float64
	movingSum          float64
	movingCount        int
}

func (a *tripAcc) add(p *model.Position) {
	s := speedOf(p)
	if a.count == 0 {
		a.start, a.maxSpeed = p.Timestamp, s
	} else {
		a.distance += haversineKm(a.lastLat, a.lastLon, p.Latitude, p.Longitude)
		a.maxSpeed = max(a.maxSpeed, s)
	}
	if s > tripSpeedThreshold {
		a.movingSum += s
		a.movingCount++
	}
	a.count++
	a.end, a.lastLat, a.lastLon = p.Timestamp, p.Latitude, p.Longitude
}

// TripDetector is a port of detectTrips (web/src/lib/utils/trips.ts). Feed
// positions in ascending timestamp order with Add, then call Trips.
type TripDetector struct {
	cur       tripAcc
	stopStart *time.Time
	last      *time.Time
	trips     []Trip
}

// Add feeds the next position.
func (d *TripDetector) Add(p *model.Position) {
	t := p.Timestamp
	if d.last != nil && d.cur.count > 0 && t.Sub(*d.last) >= tripMaxGap {
		d.finalize()
	}
	d.last = &t

	if speedOf(p) > tripSpeedThreshold {
		d.stopStart = nil
		d.cur.add(p)
		return
	}
	if d.cur.count == 0 {
		return
	}
	if d.stopStart == nil {
		d.stopStart = &t
		d.cur.add(p)
		return
	}
	if t.Sub(*d.stopStart) >= tripMinStop {
		d.finalize()
		return
	}
	d.cur.add(p)
}

func (d *TripDetector) finalize() {
	a := d.cur
	d.cur, d.stopStart = tripAcc{}, nil
	if a.count < 2 || a.end.Sub(a.start) < tripMinDuration {
		return
	}
	var avg float64
	if a.movingCount > 0 {
		avg = a.movingSum / float64(a.movingCount)
	}
	d.trips = append(d.trips, Trip{
		StartTime: a.start,
		EndTime:   a.end,
		Duration:  a.end.Sub(a.start).Seconds(),
		Distance:  a.distance,
		AvgSpeed:  avg,
		MaxSpeed:  a.maxSpeed,
	})
}

// Trips finalizes the trip in progress and returns all detected trips.
func (d *TripDetector) Trips() []Trip {
	d.finalize()
	return d.trips
}

// StopDetector is a port of detectStops (web/src/lib/utils/stops.ts). Feed
// positions in ascending timestamp order with Add, then call Stops.
type StopDetector struct {
	count          int
	start, last    time.Time
	sumLat, sumLon float64
	address        string
	stops          []Stop
}

// Add feeds the next position.
func (d *StopDetector) Add(p *model.Position) {
	if speedOf(p) < stopSpeedThreshold {
		if d.count == 0 {
			d.start = p.Timestamp
		}
		if d.address == "" && p.Address != nil {
			d.address = *p.Address
		}
		d.count++
		d.sumLat += p.Latitude
		d.sumLon += p.Longitude
		d.last = p.Timestamp
		return
	}
	if d.count > 0 {
		// Matches the TS: duration runs until the first moving position.
		d.emit(p.Timestamp.Sub(d.start))
	}
}

func (d *StopDetector) emit(duration time.Duration) {
	if duration >= stopMinDuration {
		lat, lon := d.sumLat/float64(d.count), d.sumLon/float64(d.count)
		addr := d.address
		if addr == "" {
			addr = fmt.Sprintf("%.5f, %.5f", lat, lon)
		}
		d.stops = append(d.stops, Stop{
			Latitude:      lat,
			Longitude:     lon,
			Address:       addr,
			ArrivalTime:   d.start,
			DepartureTime: d.last,
			Duration:      duration.Seconds(),
		})
	}
	*d = StopDetector{stops: d.stops}
}

// Stops finalizes the stop in progress and returns all detected stops.
func (d *StopDetector) Stops() []Stop {
	if d.count > 0 {
		d.emit(d.last.Sub(d.start))
	}
	return d.stops
}
