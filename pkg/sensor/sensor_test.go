package sensor

import "testing"

func TestLocationFromSeedRange(t *testing.T) {
	location := LocationFromSeed("node-1")
	if location.Latitude < -90 || location.Latitude > 90 {
		t.Fatalf("latitude out of range: %f", location.Latitude)
	}
	if location.Longitude < -180 || location.Longitude > 180 {
		t.Fatalf("longitude out of range: %f", location.Longitude)
	}
}

func TestGeneratorTemperatureRange(t *testing.T) {
	gen := NewGenerator("node-1", LocationFromSeed("node-1"))
	for i := 0; i < 50; i++ {
		reading := gen.Next()
		if reading.TemperatureC < minTempC || reading.TemperatureC > maxTempC {
			t.Fatalf("temperature out of range: %f", reading.TemperatureC)
		}
	}
}
