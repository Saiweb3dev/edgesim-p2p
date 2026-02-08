package sensor

import "testing"

func TestDistanceKmZero(t *testing.T) {
	loc := Location{Latitude: 0, Longitude: 0}
	if DistanceKm(loc, loc) != 0 {
		t.Fatalf("expected zero distance")
	}
}

func TestFilterByRadius(t *testing.T) {
	center := Location{Latitude: 0, Longitude: 0}
	near := Reading{NodeID: "near", Location: Location{Latitude: 0.01, Longitude: 0.01}}
	far := Reading{NodeID: "far", Location: Location{Latitude: 10, Longitude: 10}}

	readings := []Reading{near, far}
	within := FilterByRadius(readings, center, 5)
	if len(within) != 1 {
		t.Fatalf("expected 1 reading, got %d", len(within))
	}
	if within[0].NodeID != "near" {
		t.Fatalf("unexpected reading: %s", within[0].NodeID)
	}
}
