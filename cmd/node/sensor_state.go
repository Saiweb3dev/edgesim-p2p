package main

import (
	"sync"

	"github.com/Saiweb3dev/edgesim-p2p/pkg/sensor"
)

type sensorState struct {
	mu         sync.RWMutex
	reading    sensor.Reading
	hasReading bool
}

func newSensorState() *sensorState {
	return &sensorState{}
}

func (s *sensorState) Set(reading sensor.Reading) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.reading = reading
	s.hasReading = true
}

func (s *sensorState) Last() (sensor.Reading, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.reading, s.hasReading
}
