package sensor

import "math"

const earthRadiusKm = 6371.0

// DistanceKm returns the great-circle distance between two locations in km.
func DistanceKm(a Location, b Location) float64 {
	lat1 := degreesToRadians(a.Latitude)
	lat2 := degreesToRadians(b.Latitude)
	deltaLat := degreesToRadians(b.Latitude - a.Latitude)
	deltaLon := degreesToRadians(b.Longitude - a.Longitude)

	sinLat := math.Sin(deltaLat / 2)
	sinLon := math.Sin(deltaLon / 2)

	h := sinLat*sinLat + math.Cos(lat1)*math.Cos(lat2)*sinLon*sinLon
	return 2 * earthRadiusKm * math.Asin(math.Sqrt(h))
}

// FilterByRadius returns readings within radiusKm of center.
func FilterByRadius(readings []Reading, center Location, radiusKm float64) []Reading {
	if radiusKm <= 0 {
		return nil
	}

	selected := make([]Reading, 0, len(readings))
	for _, reading := range readings {
		if DistanceKm(center, reading.Location) <= radiusKm {
			selected = append(selected, reading)
		}
	}
	return selected
}

func degreesToRadians(deg float64) float64 {
	return deg * math.Pi / 180
}
