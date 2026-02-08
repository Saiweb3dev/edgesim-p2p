package dht

import "testing"

func BenchmarkRoutingTableGetClosestPeers(b *testing.B) {
	local := mustNodeIDBench(b, "0000000000000000000000000000000000000000")
	rt := NewRoutingTable(local, 20)

	for i := 1; i <= 1000; i++ {
		value := make([]byte, NodeIDLength)
		value[NodeIDLength-1] = byte(i)
		var id NodeID
		copy(id[:], value)
		peer := Peer{ID: id, Addr: "peer"}
		if err := rt.AddPeer(peer); err != nil {
			b.Fatalf("AddPeer failed: %v", err)
		}
	}

	target := mustNodeIDBench(b, "00000000000000000000000000000000000000ff")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = rt.GetClosestPeers(target, 20)
	}
}

func mustNodeIDBench(tb testing.TB, value string) NodeID {
	tb.Helper()

	id, err := NodeIDFromHex(value)
	if err != nil {
		tb.Fatalf("invalid node id %q: %v", value, err)
	}
	return id
}
