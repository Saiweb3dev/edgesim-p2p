package sensor

import "errors"

// AverageTemperature returns the average temperature for provided readings.
func AverageTemperature(readings []Reading) (float64, error) {
	if len(readings) == 0 {
		return 0, errors.New("no readings provided")
	}

	sum := 0.0
	for _, reading := range readings {
		sum += reading.TemperatureC
	}
	return sum / float64(len(readings)), nil
}

// AverageTemperatureInRadius computes the average temperature within radiusKm.
func AverageTemperatureInRadius(readings []Reading, center Location, radiusKm float64) (float64, int, error) {
	matched := FilterByRadius(readings, center, radiusKm)
	if len(matched) == 0 {
		return 0, 0, errors.New("no readings in radius")
	}

	avg, err := AverageTemperature(matched)
	if err != nil {
		return 0, 0, err
	}
	return avg, len(matched), nil
}
