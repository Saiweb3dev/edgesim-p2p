package dht

import "sync"

// Bucket stores a bounded list of peers ordered by recency (LRU).
//
// Dry run example (k=3):
//
//	Add A,B,C -> [A B C]
//	Add B     -> [A C B] (B moved to most-recent)
//	Add D     -> evict A, bucket = [C B D]
type Bucket struct {
	mu      sync.RWMutex
	peers   []Peer
	maxSize int
}

func NewBucket(maxSize int) *Bucket {
	if maxSize <= 0 {
		maxSize = 20
	}
	return &Bucket{maxSize: maxSize}
}

// Add inserts or updates a peer, evicting the oldest if the bucket is full.
func (b *Bucket) Add(peer Peer) (evicted *Peer) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if index := b.indexOf(peer.ID); index >= 0 {
		b.peers[index] = peer
		b.moveToEnd(index)
		return nil
	}

	if len(b.peers) >= b.maxSize {
		oldest := b.peers[0]
		b.peers = b.peers[1:]
		evicted = &oldest
	}

	b.peers = append(b.peers, peer)
	return evicted
}

func (b *Bucket) Remove(peerID NodeID) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	index := b.indexOf(peerID)
	if index < 0 {
		return false
	}

	copy(b.peers[index:], b.peers[index+1:])
	b.peers = b.peers[:len(b.peers)-1]
	return true
}

func (b *Bucket) List() []Peer {
	b.mu.RLock()
	defer b.mu.RUnlock()

	result := make([]Peer, len(b.peers))
	copy(result, b.peers)
	return result
}

func (b *Bucket) Len() int {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return len(b.peers)
}

func (b *Bucket) indexOf(peerID NodeID) int {
	for i, peer := range b.peers {
		if peer.ID == peerID {
			return i
		}
	}
	return -1
}

func (b *Bucket) moveToEnd(index int) {
	if index < 0 || index >= len(b.peers) {
		return
	}

	peer := b.peers[index]
	copy(b.peers[index:], b.peers[index+1:])
	b.peers[len(b.peers)-1] = peer
}
