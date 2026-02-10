package raft

import (
	"context"
	"testing"
	"time"
)

func TestElectionChoosesLeader(t *testing.T) {
	ids := []string{"coord-1", "coord-2", "coord-3"}
	transport := NewMemoryTransport()

	nodes, cancelAll, _ := startNodes(t, ids, transport)
	defer cancelAll()

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

func TestElectionReelectsAfterLeaderFailure(t *testing.T) {
	ids := []string{"coord-1", "coord-2", "coord-3"}
	transport := NewMemoryTransport()

	nodes, cancelAll, stopNode := startNodes(t, ids, transport)
	defer cancelAll()

	leaderID, err := waitForLeader(nodes, 2*time.Second)
	if err != nil {
		t.Fatalf("leader not elected: %v", err)
	}

	if !stopNode(leaderID) {
		t.Fatalf("leader node not found: %s", leaderID)
	}
	transport.Unregister(leaderID)

	newLeaderID, err := waitForLeader(nodes, 2*time.Second)
	if err != nil {
		t.Fatalf("new leader not elected: %v", err)
	}
	if newLeaderID == leaderID {
		t.Fatalf("leader did not change after failure: %s", leaderID)
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

func startNodes(t *testing.T, ids []string, transport *MemoryTransport) ([]*Node, func(), func(string) bool) {
	t.Helper()

	nodes := make([]*Node, 0, len(ids))
	cancels := make([]context.CancelFunc, 0, len(ids))
 	index := make(map[string]int, len(ids))

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

		ctx, cancel := context.WithCancel(context.Background())
		cancels = append(cancels, cancel)
		index[id] = len(nodes)
		nodes = append(nodes, node)
	}

	for i, node := range nodes {
		ctx := ctxs[i]
		go func(n *Node, runCtx context.Context) {
			_ = n.Run(runCtx)
		}(node, ctx)
	}

	return nodes, func() {
		for _, cancel := range cancels {
			cancel()
		}
	}, func(nodeID string) bool {
		idx, ok := index[nodeID]
		if !ok {
			return false
		}
		cancels[idx]()
		node := nodes[idx]
		node.mu.Lock()
		node.state = StateFollower
		node.leaderID = ""
		node.mu.Unlock()
		return true
	}
}
