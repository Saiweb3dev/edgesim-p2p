package raft

import (
	"context"
	"math/rand"
	"testing"
	"time"
)

func TestProposeReplicatesAndSurvivesLeaderFailure(t *testing.T) {
	ids := []string{"coord-1", "coord-2", "coord-3"}
	transport := NewMemoryTransport()

	nodes, cancelAll, stopNode := startNodesWithSeed(t, ids, transport)
	defer cancelAll()

	leaderID, err := waitForLeader(nodes, 2*time.Second)
	if err != nil {
		t.Fatalf("leader not elected: %v", err)
	}

	leader := nodeByID(nodes, leaderID)
	if leader == nil {
		t.Fatalf("leader node not found: %s", leaderID)
	}

	if err := leader.Propose(context.Background(), "reading|node-1|22.5"); err != nil {
		t.Fatalf("propose failed: %v", err)
	}

	if err := waitForCommit(nodes, 1, 2*time.Second); err != nil {
		t.Fatalf("commit not observed: %v", err)
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

	newLeader := nodeByID(nodes, newLeaderID)
	if newLeader == nil {
		t.Fatalf("new leader node not found: %s", newLeaderID)
	}

	if len(newLeader.LogSnapshot()) == 0 {
		t.Fatalf("expected log entries to survive leader failure")
	}
}

func startNodesWithSeed(t *testing.T, ids []string, transport *MemoryTransport) ([]*Node, func(), func(string) bool) {
	t.Helper()

	nodes := make([]*Node, 0, len(ids))
	ctxs := make([]context.Context, 0, len(ids))
	cancels := make([]context.CancelFunc, 0, len(ids))
	index := make(map[string]int, len(ids))

	for i, id := range ids {
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
			Rand:               rand.New(rand.NewSource(int64(i + 1))),
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
		ctxs = append(ctxs, ctx)
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

func nodeByID(nodes []*Node, id string) *Node {
	for _, node := range nodes {
		if node.id == id {
			return node
		}
	}
	return nil
}

func waitForCommit(nodes []*Node, commitIndex uint64, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		count := 0
		for _, node := range nodes {
			if node.CommitIndex() >= commitIndex {
				count++
			}
		}
		if count == len(nodes) {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return errTimeout
}
