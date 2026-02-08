package dht

import "testing"

func TestBucketAddMovesPeerToEnd(t *testing.T) {
	bucket := NewBucket(3)
	peerA := Peer{ID: mustNodeID(t, "0000000000000000000000000000000000000001"), Addr: "a"}
	peerB := Peer{ID: mustNodeID(t, "0000000000000000000000000000000000000002"), Addr: "b"}
	peerC := Peer{ID: mustNodeID(t, "0000000000000000000000000000000000000003"), Addr: "c"}

	bucket.Add(peerA)
	bucket.Add(peerB)
	bucket.Add(peerC)

	peerBUpdated := Peer{ID: peerB.ID, Addr: "b-new"}
	bucket.Add(peerBUpdated)

	peers := bucket.List()
	if len(peers) != 3 {
		t.Fatalf("expected 3 peers, got %d", len(peers))
	}
	if peers[2].ID != peerB.ID {
		t.Fatalf("expected most recent peer to be B, got %s", peers[2].ID.String())
	}
}

func TestBucketEvictsOldest(t *testing.T) {
	bucket := NewBucket(3)
	peerA := Peer{ID: mustNodeID(t, "0000000000000000000000000000000000000001"), Addr: "a"}
	peerB := Peer{ID: mustNodeID(t, "0000000000000000000000000000000000000002"), Addr: "b"}
	peerC := Peer{ID: mustNodeID(t, "0000000000000000000000000000000000000003"), Addr: "c"}
	peerD := Peer{ID: mustNodeID(t, "0000000000000000000000000000000000000004"), Addr: "d"}

	bucket.Add(peerA)
	bucket.Add(peerB)
	bucket.Add(peerC)

	evicted := bucket.Add(peerD)
	if evicted == nil {
		t.Fatalf("expected eviction, got nil")
	}
	if evicted.ID != peerA.ID {
		t.Fatalf("expected evicted peer A, got %s", evicted.ID.String())
	}
	if bucket.Len() != 3 {
		t.Fatalf("expected bucket size 3, got %d", bucket.Len())
	}
}

func TestBucketRemove(t *testing.T) {
	bucket := NewBucket(3)
	peerA := Peer{ID: mustNodeID(t, "0000000000000000000000000000000000000001"), Addr: "a"}
	peerB := Peer{ID: mustNodeID(t, "0000000000000000000000000000000000000002"), Addr: "b"}

	bucket.Add(peerA)
	bucket.Add(peerB)

	removed := bucket.Remove(peerA.ID)
	if !removed {
		t.Fatalf("expected remove to return true")
	}
	if bucket.Len() != 1 {
		t.Fatalf("expected bucket size 1, got %d", bucket.Len())
	}
	if bucket.Remove(peerA.ID) {
		t.Fatalf("expected second remove to return false")
	}
}
