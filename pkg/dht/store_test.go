package dht

import (
	"context"
	"testing"
)

func TestStoreAndFindValueAcrossTenNodes(t *testing.T) {
	ctx := context.Background()
	nodes := buildKVNetwork(t, 10)
	byID := kvNodesByID(nodes)

	key := "sensor:room-101"
	value := []byte("21.5")
	k := 3

	storeRequester := func(ctx context.Context, peer Peer, key string, value []byte) error {
		return byID[peer.ID].HandleStore(ctx, key, value)
	}

	findNodeRequester := func(ctx context.Context, peer Peer, target NodeID) ([]Peer, error) {
		return byID[peer.ID].HandleFindNode(ctx, target, k)
	}

	findValueRequester := func(ctx context.Context, peer Peer, key string) ([]byte, []Peer, bool, error) {
		return byID[peer.ID].HandleFindValue(ctx, key, k)
	}

	// Store using node 0, then retrieve using node 5.
	_, err := IterativeStore(ctx, nodes[0].rt, findNodeRequester, storeRequester, key, value, FindNodeOptions{K: k, Alpha: 3, MaxRounds: 20})
	if err != nil {
		t.Fatalf("IterativeStore failed: %v", err)
	}

	got, _, err := IterativeFindValue(ctx, nodes[5].rt, findValueRequester, key, FindNodeOptions{K: k, Alpha: 3, MaxRounds: 20})
	if err != nil {
		t.Fatalf("IterativeFindValue failed: %v", err)
	}
	if string(got) != string(value) {
		t.Fatalf("expected value %q, got %q", string(value), string(got))
	}
}

type kvNode struct {
	peer Peer
	rt   *RoutingTable
	node *Node
}

func buildKVNetwork(t *testing.T, count int) []kvNode {
	nodes := make([]kvNode, 0, count)
	for i := 0; i < count; i++ {
		value := make([]byte, NodeIDLength)
		value[NodeIDLength-1] = byte(i + 1)
		var id NodeID
		copy(id[:], value)

		node := NewNode(id, "node-"+string(rune('a'+i)), 20)
		nodes = append(nodes, kvNode{peer: node.Peer(), rt: node.RoutingTable(), node: node})
	}

	for i := range nodes {
		for j := 1; j <= 3; j++ {
			index := (i + j) % len(nodes)
			peer := nodes[index].peer
			if err := nodes[i].rt.AddPeer(peer); err != nil {
				t.Fatalf("AddPeer failed: %v", err)
			}
		}
	}

	return nodes
}

func kvNodesByID(nodes []kvNode) map[NodeID]*Node {
	lookup := make(map[NodeID]*Node, len(nodes))
	for _, node := range nodes {
		lookup[node.peer.ID] = node.node
	}
	return lookup
}
