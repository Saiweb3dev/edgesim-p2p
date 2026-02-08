package gossip

import "testing"

func TestSimulatePropagationReachesTarget(t *testing.T) {
	nodes := make([]Node, 0, 20)
	ids := make([]string, 0, 20)
	for i := 0; i < 20; i++ {
		id := "node-" + string(rune('a'+i))
		ids = append(ids, id)
	}
	for _, id := range ids {
		peers := make([]string, 0, len(ids)-1)
		for _, peerID := range ids {
			if peerID == id {
				continue
			}
			peers = append(peers, peerID)
		}
		nodes = append(nodes, Node{ID: id, Peers: peers})
	}

	sim := NewSimulator(42)
	result, err := sim.SimulatePropagation(nodes, ids[0], 3, 0.9, 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ReachedRatio < 0.9 {
		t.Fatalf("expected >= 0.9 reached, got %f", result.ReachedRatio)
	}
}
