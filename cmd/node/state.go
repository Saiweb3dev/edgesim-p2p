package main

import (
	"sort"
	"sync"
	"time"

	"github.com/Saiweb3dev/edgesim-p2p/pkg/sensor"
)

type peerInfo struct {
	nodeID   string
	addr     string
	lastSeen time.Time
}

type nodeState struct {
	mu         sync.RWMutex
	self       peerInfo
	peers      map[string]peerInfo
	readings   map[string]sensor.Reading
	seenGossip map[string]time.Time
}

func (s *nodeState) upsertPeer(peer peerInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if peer.addr == "" {
		return
	}
	peer.lastSeen = time.Now()
	s.peers[peer.addr] = peer
}

func (s *nodeState) peerAddrs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	addrs := make([]string, 0, len(s.peers)+1)
	addrs = append(addrs, s.self.addr)
	for addr := range s.peers {
		addrs = append(addrs, addr)
	}
	sort.Strings(addrs)
	return addrs
}

func (s *nodeState) peerCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return len(s.peers)
}

func (s *nodeState) setReading(reading sensor.Reading) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.readings == nil {
		s.readings = make(map[string]sensor.Reading)
	}
	s.readings[reading.NodeID] = reading
}

func (s *nodeState) shouldProcessGossip(messageID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.seenGossip == nil {
		s.seenGossip = make(map[string]time.Time)
	}
	if _, ok := s.seenGossip[messageID]; ok {
		return false
	}
	s.seenGossip[messageID] = time.Now()
	return true
}
