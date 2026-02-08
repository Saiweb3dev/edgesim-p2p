package sensor

import (
	"crypto/sha256"
	"encoding/binary"
	"math"
	"math/rand"
	"time"
)

const (
	minTempC = -10.0
	maxTempC = 45.0
)

// Location represents a geographic point.
type Location struct {
	Latitude  float64
	Longitude float64
}

// Reading models a single sensor observation.
type Reading struct {
	NodeID       string
	TemperatureC float64
	Location     Location
	Timestamp    time.Time
}

// Generator produces synthetic temperature readings for a node.
type Generator struct {
	nodeID   string
	location Location
	rng      *rand.Rand
	baseTemp float64
	jitter   float64
}

// NewGenerator builds a generator with a stable random seed derived from nodeID.
func NewGenerator(nodeID string, location Location) *Generator {
	seed := seedFromString(nodeID)
	rng := rand.New(rand.NewSource(seed))
	base := 18 + rng.Float64()*10
	jitter := 0.8

	return &Generator{
		nodeID:   nodeID,
		location: location,
		rng:      rng,
		baseTemp: base,
		jitter:   jitter,
	}
}

// Next returns a new reading with timestamp set to now.
func (g *Generator) Next() Reading {
	noise := g.rng.NormFloat64() * g.jitter
	value := clamp(g.baseTemp+noise, minTempC, maxTempC)

	return Reading{
		NodeID:       g.nodeID,
		TemperatureC: value,
		Location:     g.location,
		Timestamp:    time.Now().UTC(),
	}
}

// LocationFromSeed derives a stable location from a seed string.
func LocationFromSeed(seed string) Location {
	hash := sha256.Sum256([]byte(seed))
	latBits := binary.BigEndian.Uint64(hash[:8])
	lonBits := binary.BigEndian.Uint64(hash[8:16])

	lat := scaleUint64(latBits, -90, 90)
	lon := scaleUint64(lonBits, -180, 180)

	return Location{Latitude: lat, Longitude: lon}
}

func seedFromString(value string) int64 {
	hash := sha256.Sum256([]byte(value))
	return int64(binary.BigEndian.Uint64(hash[:8]))
}

func scaleUint64(value uint64, min float64, max float64) float64 {
	if max <= min {
		return min
	}
	fraction := float64(value) / float64(math.MaxUint64)
	return min + fraction*(max-min)
}

func clamp(value float64, min float64, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}
