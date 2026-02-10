package raft

import (
	"context"
	"math/rand"
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

func TestHeartbeatsKeepSingleLeader(t *testing.T) {
	ids := []string{"coord-1", "coord-2", "coord-3"}
	transport := NewMemoryTransport()

	nodes, cancelAll, _ := startNodes(t, ids, transport)
	defer cancelAll()

	leaderID, err := waitForLeader(nodes, 2*time.Second)
	if err != nil {
		t.Fatalf("leader not elected: %v", err)
	}

	deadline := time.Now().Add(300 * time.Millisecond)
	for time.Now().Before(deadline) {
		currentLeader, err := waitForLeader(nodes, 200*time.Millisecond)
		if err != nil {
			t.Fatalf("leader unstable: %v", err)
		}
		if currentLeader != leaderID {
			t.Fatalf("leader changed while heartbeats active: %s -> %s", leaderID, currentLeader)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestAppendEntriesReplicatesLog(t *testing.T) {
	follower := &Node{currentTerm: 2}
	follower.log = []LogEntry{{Term: 2, Command: "set=a"}}

	resp := follower.HandleAppendEntries(context.Background(), AppendEntriesRequest{
		Term:         3,
		LeaderID:     "coord-1",
		PrevLogIndex: 1,
		PrevLogTerm:  2,
		Entries:      []LogEntry{{Term: 3, Command: "set=b"}},
		LeaderCommit: 2,
	})
	if !resp.Success {
		t.Fatalf("append entries rejected")
	}
	if len(follower.log) != 2 {
		t.Fatalf("expected follower to have 2 entries, got %d", len(follower.log))
	}
	if follower.log[1].Term != 3 {
		t.Fatalf("expected follower term 3, got %d", follower.log[1].Term)
	}
	if follower.commitIndex != 2 {
		t.Fatalf("expected commit index 2, got %d", follower.commitIndex)
	}
}

func TestFollowersDetectLeaderFailureUnder1s(t *testing.T) {
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

	newLeaderID, err := waitForLeader(nodes, 900*time.Millisecond)
	if err != nil {
		t.Fatalf("new leader not elected in time: %v", err)
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
