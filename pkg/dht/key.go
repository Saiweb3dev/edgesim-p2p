package dht

import "crypto/sha256"

// KeyToID hashes an arbitrary key into a 160-bit NodeID.
func KeyToID(key string) NodeID {
	sum := sha256.Sum256([]byte(key))
	var id NodeID
	copy(id[:], sum[:NodeIDLength])
	return id
}
