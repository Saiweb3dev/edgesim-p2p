package dht

import (
	"context"
	"sort"
	"testing"
)

func TestIterativeFindNodeTenNodes(t *testing.T) {
	ctx := context.Background()
	k := 3
	alpha := 3

	nodes := buildTestNetwork(t, 10)
	start := nodes[0]
	target := nodes[7].peer.ID

	byID := nodesByID(nodes)
	queryCount := 0
	requester := func(ctx context.Context, peer Peer, targetID NodeID) ([]Peer, error) {
		queryCount++
		rt := byID[peer.ID]
		return rt.GetClosestPeers(targetID, k), nil
	}

	result, err := IterativeFindNode(ctx, start.rt, requester, target, FindNodeOptions{K: k, Alpha: alpha, MaxRounds: 20})
	if err != nil {
		t.Fatalf("IterativeFindNode failed: %v", err)
	}

	if len(result) != k {
		t.Fatalf("expected %d peers, got %d", k, len(result))
	}

	expected := expectedClosest(nodes, start.peer.ID, target, k)
	for i := 0; i < k; i++ {
		if result[i].ID != expected[i].ID {
			t.Fatalf("closest mismatch at %d: got %s want %s", i, result[i].ID.String(), expected[i].ID.String())
		}
	}

	if queryCount > len(nodes) {
		t.Fatalf("expected <= %d queries, got %d", len(nodes), queryCount)
	}
}

type testNode struct {
	peer Peer
	rt   *RoutingTable
}

func buildTestNetwork(t *testing.T, count int) []testNode {
	ids := make([]NodeID, 0, count)
	for i := 0; i < count; i++ {
		value := make([]byte, NodeIDLength)
		value[NodeIDLength-1] = byte(i + 1)
		var id NodeID
		copy(id[:], value)
		ids = append(ids, id)
	}

	nodes := make([]testNode, 0, count)
	for i, id := range ids {
		peer := Peer{ID: id, Addr: "node-" + string(rune('a'+i))}
		rt := NewRoutingTable(id, 20)
		nodes = append(nodes, testNode{peer: peer, rt: rt})
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

func nodesByID(nodes []testNode) map[NodeID]*RoutingTable {
	lookup := make(map[NodeID]*RoutingTable, len(nodes))
	for _, node := range nodes {
		lookup[node.peer.ID] = node.rt
	}
	return lookup
}

func expectedClosest(nodes []testNode, exclude NodeID, target NodeID, k int) []Peer {
	peers := make([]Peer, 0, len(nodes))
	for _, node := range nodes {
		if node.peer.ID == exclude {
			continue
		}
		peers = append(peers, node.peer)
	}

	sort.Slice(peers, func(i, j int) bool {
		return xorDistance(target, peers[i].ID).Cmp(xorDistance(target, peers[j].ID)) < 0
	})

	if len(peers) > k {
		peers = peers[:k]
	}
	return peers
}
