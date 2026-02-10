package raft

import (
	"context"
	"testing"
	"time"
)

func TestElectionChoosesLeader(t *testing.T) {
	ids := []string{"coord-1", "coord-2", "coord-3"}
	transport := NewMemoryTransport()

	nodes := make([]*Node, 0, len(ids))
	for _, id := range ids {
		peers := make([]string, 0, len(ids)-1)
		for _, peerID := range ids {
			if peerID != id {
				peers = append(peers, peerID)
			}
		}

		node, err := NewNode(Config{
			ID:                 id,
			Peers:              peers,
			Transport:          transport,
			ElectionTimeoutMin: 50 * time.Millisecond,
			ElectionTimeoutMax: 100 * time.Millisecond,
			HeartbeatInterval:  20 * time.Millisecond,
			RPCTimeout:         40 * time.Millisecond,
		})
		if err != nil {
			t.Fatalf("new node %s: %v", id, err)
		}

		if err := transport.Register(node); err != nil {
			t.Fatalf("register node %s: %v", id, err)
		}
		nodes = append(nodes, node)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, node := range nodes {
		go func(n *Node) {
			_ = n.Run(ctx)
		}(node)
	}

	leaderID, err := waitForLeader(nodes, 2*time.Second)
	if err != nil {
		t.Fatalf("leader not elected: %v", err)
	}

	for _, node := range nodes {
		if node.State() == StateLeader && node.id != leaderID {
			t.Fatalf("unexpected leader %s (expected %s)", node.id, leaderID)
		}
	}
}

func waitForLeader(nodes []*Node, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		leaderID := ""
		leaders := 0
		for _, node := range nodes {
			if node.State() == StateLeader {
				leaders++
				leaderID = node.id
			}
		}

		if leaders == 1 {
			return leaderID, nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return "", errTimeout
}

var errTimeout = &timeoutError{}

type timeoutError struct{}

func (t *timeoutError) Error() string {
	return "timeout waiting for leader"
}
