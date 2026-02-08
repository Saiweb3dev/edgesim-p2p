package main

import (
	"fmt"

	"github.com/Saiweb3dev/edgesim-p2p/pkg/sensor"
)

// QueryAPI provides read-only aggregation helpers for sensor readings.
type QueryAPI struct{}

// AverageTemperatureInRadius returns the average temperature and sample count.
func (q *QueryAPI) AverageTemperatureInRadius(readings []sensor.Reading, center sensor.Location, radiusKm float64) (float64, int, error) {
	avg, count, err := sensor.AverageTemperatureInRadius(readings, center, radiusKm)
	if err != nil {
		return 0, 0, fmt.Errorf("average temperature in radius: %w", err)
	}
	return avg, count, nil
}
