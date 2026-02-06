package dht

import (
	"math/big"
	"testing"
)

func TestBucketIndexExample(t *testing.T) {
	local := mustNodeID(t, "0000000000000000000000000000000000000000")
	peer := mustNodeID(t, "1400000000000000000000000000000000000000")

	index := bucketIndex(local, peer)
	if index != 4 {
		t.Fatalf("expected bucket index 4, got %d", index)
	}
}

func TestXORDistanceExample(t *testing.T) {
	a := mustNodeID(t, "000000000000000000000000000000000000000f")
	b := mustNodeID(t, "0000000000000000000000000000000000000033")

	distance := xorDistance(a, b)
	if distance.Cmp(big.NewInt(60)) != 0 {
		t.Fatalf("expected xor distance 60, got %s", distance.String())
	}
}

func TestRoutingTableClosestPeersFiveNodes(t *testing.T) {
	local := mustNodeID(t, "0000000000000000000000000000000000000000")
	rt := NewRoutingTable(local, 20)

	peerIDs := []string{
		"0000000000000000000000000000000000000001",
		"0000000000000000000000000000000000000002",
		"0000000000000000000000000000000000000004",
		"0000000000000000000000000000000000000008",
		"0000000000000000000000000000000000000010",
	}

	for i, id := range peerIDs {
		peer := Peer{ID: mustNodeID(t, id), Addr: "peer-" + string(rune('a'+i))}
		if err := rt.AddPeer(peer); err != nil {
			t.Fatalf("AddPeer failed: %v", err)
		}
	}

	closest := rt.GetClosestPeers(local, 3)
	if len(closest) != 3 {
		t.Fatalf("expected 3 closest peers, got %d", len(closest))
	}

	if closest[0].ID.String() != peerIDs[0] {
		t.Fatalf("expected closest %s, got %s", peerIDs[0], closest[0].ID.String())
	}
	if closest[1].ID.String() != peerIDs[1] {
		t.Fatalf("expected second %s, got %s", peerIDs[1], closest[1].ID.String())
	}
	if closest[2].ID.String() != peerIDs[2] {
		t.Fatalf("expected third %s, got %s", peerIDs[2], closest[2].ID.String())
	}
}

func mustNodeID(t *testing.T, value string) NodeID {
	t.Helper()

	id, err := NodeIDFromHex(value)
	if err != nil {
		t.Fatalf("invalid node id %q: %v", value, err)
	}
	return id
}
