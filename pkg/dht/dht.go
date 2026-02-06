package dht

import (
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

const NodeIDLength = 20

// NodeID is a 160-bit identifier used to index peers in the routing table.
type NodeID [NodeIDLength]byte

// Peer represents a node known to the routing table.
type Peer struct {
	ID       NodeID
	Addr     string
	LastSeen time.Time
}

// NodeIDFromHex parses a 160-bit hex-encoded node ID.
func NodeIDFromHex(value string) (NodeID, error) {
	var id NodeID
	clean := strings.TrimPrefix(strings.TrimSpace(value), "0x")
	if len(clean) != NodeIDLength*2 {
		return id, errors.New("node id must be 40 hex characters")
	}

	decoded, err := hex.DecodeString(clean)
	if err != nil {
		return id, err
	}
	copy(id[:], decoded)
	return id, nil
}

func (id NodeID) String() string {
	return hex.EncodeToString(id[:])
}
