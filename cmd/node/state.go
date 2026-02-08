package main

import (
	"sort"
	"sync"
	"time"
)

type peerInfo struct {
	nodeID   string
	addr     string
	lastSeen time.Time
}

type nodeState struct {
	mu    sync.RWMutex
	self  peerInfo
	peers map[string]peerInfo
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
