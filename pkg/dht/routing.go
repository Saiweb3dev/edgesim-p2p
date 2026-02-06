package dht

import (
	"errors"
	"math/big"
	"math/bits"
	"sort"
	"sync"
	"time"
)

var (
	ErrInvalidPeer = errors.New("peer must have id and address")
	ErrSelfPeer    = errors.New("cannot add local node as peer")
)

// RoutingTable organizes peers into k-buckets based on XOR distance.
//
// Dry run example (8-bit IDs, bucket index = highest differing bit):
//
//	local = 00000000, peer = 00010100
//	xor   = 00010100, highest 1-bit at position 4 -> bucket index 4
type RoutingTable struct {
	mu         sync.RWMutex
	localID    NodeID
	bucketSize int
	buckets    []*Bucket
}

func NewRoutingTable(localID NodeID, bucketSize int) *RoutingTable {
	if bucketSize <= 0 {
		bucketSize = 20
	}

	bucketCount := NodeIDLength * 8
	buckets := make([]*Bucket, bucketCount)
	for i := range buckets {
		buckets[i] = NewBucket(bucketSize)
	}

	return &RoutingTable{
		localID:    localID,
		bucketSize: bucketSize,
		buckets:    buckets,
	}
}

// AddPeer inserts or updates a peer based on its XOR distance.
func (rt *RoutingTable) AddPeer(peer Peer) error {
	if peer.Addr == "" {
		return ErrInvalidPeer
	}
	if peer.ID == (NodeID{}) {
		return ErrInvalidPeer
	}
	if peer.ID == rt.localID {
		return ErrSelfPeer
	}

	peer.LastSeen = time.Now()
	index := bucketIndex(rt.localID, peer.ID)

	rt.mu.RLock()
	bucket := rt.buckets[index]
	rt.mu.RUnlock()

	bucket.Add(peer)
	return nil
}

// RemovePeer removes a peer from the routing table.
func (rt *RoutingTable) RemovePeer(peerID NodeID) bool {
	index := bucketIndex(rt.localID, peerID)

	rt.mu.RLock()
	bucket := rt.buckets[index]
	rt.mu.RUnlock()

	return bucket.Remove(peerID)
}

// GetClosestPeers returns up to k peers sorted by XOR distance to target.
func (rt *RoutingTable) GetClosestPeers(target NodeID, k int) []Peer {
	if k <= 0 {
		return nil
	}

	var allPeers []Peer
	rt.mu.RLock()
	for _, bucket := range rt.buckets {
		allPeers = append(allPeers, bucket.List()...)
	}
	rt.mu.RUnlock()

	if len(allPeers) == 0 {
		return nil
	}

	sorted := make([]peerDistance, 0, len(allPeers))
	for _, peer := range allPeers {
		distance := xorDistance(target, peer.ID)
		sorted = append(sorted, peerDistance{peer: peer, distance: distance})
	}

	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].distance.Cmp(sorted[j].distance) < 0
	})

	limit := k
	if limit > len(sorted) {
		limit = len(sorted)
	}

	result := make([]Peer, 0, limit)
	for i := 0; i < limit; i++ {
		result = append(result, sorted[i].peer)
	}
	return result
}

type peerDistance struct {
	peer     Peer
	distance *big.Int
}

// bucketIndex returns the bucket for a peer based on the highest differing bit.
func bucketIndex(localID NodeID, peerID NodeID) int {
	for i := 0; i < NodeIDLength; i++ {
		xorByte := localID[i] ^ peerID[i]
		if xorByte == 0 {
			continue
		}
		bitPos := 7 - bits.LeadingZeros8(xorByte)
		return i*8 + bitPos
	}

	return NodeIDLength*8 - 1
}

// xorDistance returns the XOR metric as a big.Int.
//
// Dry run example (8-bit IDs):
//
//	a = 00001111 (15), b = 00110011 (51)
//	xor = 00111100 (60)
func xorDistance(a NodeID, b NodeID) *big.Int {
	buf := make([]byte, NodeIDLength)
	for i := 0; i < NodeIDLength; i++ {
		buf[i] = a[i] ^ b[i]
	}
	return new(big.Int).SetBytes(buf)
}
