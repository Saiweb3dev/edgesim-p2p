package dht

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Saiweb3dev/edgesim-p2p/pkg/sensor"
)

func TestSensorPublishAndQueryAcrossTwentyNodes(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	network := buildSensorNetwork(t, 20)
	anchor := network.nodes[0]

	for _, node := range network.nodes {
		reading := sensor.NewGenerator(node.Peer().Addr, sensor.LocationFromSeed(node.Peer().Addr)).Next()
		payload, err := sensor.EncodeReading(reading)
		if err != nil {
			t.Fatalf("encode reading: %v", err)
		}

		key := sensorKey(node.Peer().Addr)
		_, err = IterativeStore(ctx, node.RoutingTable(), network.findNode, network.storeValue, key, payload, FindNodeOptions{K: 20, Alpha: 3, MaxRounds: 20})
		if err != nil {
			t.Fatalf("iterative store failed: %v", err)
		}
	}

	for _, node := range network.nodes {
		key := sensorKey(node.Peer().Addr)
		value, _, err := IterativeFindValue(ctx, anchor.RoutingTable(), network.findValue, key, FindNodeOptions{K: 20, Alpha: 3, MaxRounds: 20})
		if err != nil {
			t.Fatalf("find value failed: %v", err)
		}

		reading, err := sensor.DecodeReading(value)
		if err != nil {
			t.Fatalf("decode reading: %v", err)
		}
		if reading.NodeID != node.Peer().Addr {
			t.Fatalf("unexpected reading node id: %s", reading.NodeID)
		}
	}
}

type sensorNetwork struct {
	nodes []*Node
	byID  map[NodeID]*Node
}

func buildSensorNetwork(tb testing.TB, count int) *sensorNetwork {
	tb.Helper()

	nodes := make([]*Node, 0, count)
	byID := make(map[NodeID]*Node, count)

	for i := 0; i < count; i++ {
		name := fmt.Sprintf("node-%d", i+1)
		id := KeyToID(name)
		node := NewNode(id, name, 20)
		nodes = append(nodes, node)
		byID[id] = node
	}

	for _, node := range nodes {
		for _, peer := range nodes {
			if peer.Peer().ID == node.Peer().ID {
				continue
			}
			_ = node.RoutingTable().AddPeer(peer.Peer())
		}
	}

	return &sensorNetwork{nodes: nodes, byID: byID}
}

func (n *sensorNetwork) findNode(ctx context.Context, peer Peer, target NodeID) ([]Peer, error) {
	remote, ok := n.byID[peer.ID]
	if !ok {
		return nil, fmt.Errorf("unknown peer: %s", peer.ID.String())
	}
	return remote.HandleFindNode(ctx, target, 20)
}

func (n *sensorNetwork) storeValue(ctx context.Context, peer Peer, key string, value []byte) error {
	remote, ok := n.byID[peer.ID]
	if !ok {
		return fmt.Errorf("unknown peer: %s", peer.ID.String())
	}
	return remote.HandleStore(ctx, key, value)
}

func (n *sensorNetwork) findValue(ctx context.Context, peer Peer, key string) ([]byte, []Peer, bool, error) {
	remote, ok := n.byID[peer.ID]
	if !ok {
		return nil, nil, false, fmt.Errorf("unknown peer: %s", peer.ID.String())
	}
	return remote.HandleFindValue(ctx, key, 20)
}

func sensorKey(nodeID string) string {
	return fmt.Sprintf("sensor:%s:latest", nodeID)
}
