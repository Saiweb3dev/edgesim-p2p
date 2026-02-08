package sensor

import "testing"

func TestAverageTemperature(t *testing.T) {
	readings := []Reading{
		{TemperatureC: 10},
		{TemperatureC: 20},
		{TemperatureC: 30},
	}

	avg, err := AverageTemperature(readings)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if avg != 20 {
		t.Fatalf("expected avg 20, got %f", avg)
	}
}

func TestAverageTemperatureInRadius(t *testing.T) {
	center := Location{Latitude: 0, Longitude: 0}
	readings := []Reading{
		{NodeID: "near", TemperatureC: 10, Location: Location{Latitude: 0.01, Longitude: 0.01}},
		{NodeID: "far", TemperatureC: 30, Location: Location{Latitude: 10, Longitude: 10}},
	}

	avg, count, err := AverageTemperatureInRadius(readings, center, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 reading, got %d", count)
	}
	if avg != 10 {
		t.Fatalf("expected avg 10, got %f", avg)
	}
}
