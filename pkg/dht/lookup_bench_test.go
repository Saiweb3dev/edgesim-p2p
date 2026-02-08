package dht

import (
	"context"
	"testing"
)

func BenchmarkIterativeFindNode(b *testing.B) {
	ctx := context.Background()
	k := 20
	alpha := 3

	nodes := buildBenchmarkNetwork(b, 200)
	start := nodes[0]
	target := nodes[150].peer.ID

	byID := nodesByIDBenchmark(nodes)
	requester := func(ctx context.Context, peer Peer, targetID NodeID) ([]Peer, error) {
		rt := byID[peer.ID]
		return rt.GetClosestPeers(targetID, k), nil
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = IterativeFindNode(ctx, start.rt, requester, target, FindNodeOptions{K: k, Alpha: alpha, MaxRounds: 20})
	}
}

type benchNode struct {
	peer Peer
	rt   *RoutingTable
}

func buildBenchmarkNetwork(tb testing.TB, count int) []benchNode {
	tb.Helper()

	ids := make([]NodeID, 0, count)
	for i := 0; i < count; i++ {
		value := make([]byte, NodeIDLength)
		value[NodeIDLength-1] = byte(i + 1)
		var id NodeID
		copy(id[:], value)
		ids = append(ids, id)
	}

	nodes := make([]benchNode, 0, count)
	for i, id := range ids {
		peer := Peer{ID: id, Addr: "node-" + string(rune('a'+(i%26)))}
		rt := NewRoutingTable(id, 20)
		nodes = append(nodes, benchNode{peer: peer, rt: rt})
	}

	for i := range nodes {
		for j := 1; j <= 8; j++ {
			index := (i + j) % len(nodes)
			peer := nodes[index].peer
			if err := nodes[i].rt.AddPeer(peer); err != nil {
				tb.Fatalf("AddPeer failed: %v", err)
			}
		}
	}

	return nodes
}

func nodesByIDBenchmark(nodes []benchNode) map[NodeID]*RoutingTable {
	lookup := make(map[NodeID]*RoutingTable, len(nodes))
	for _, node := range nodes {
		lookup[node.peer.ID] = node.rt
	}
	return lookup
}
