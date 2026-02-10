package raft

import (
	"context"
	"fmt"
	"sync"
)

// Transport sends Raft RPCs to peer nodes.
type Transport interface {
	RequestVote(ctx context.Context, targetID string, req RequestVoteRequest) (RequestVoteResponse, error)
	AppendEntries(ctx context.Context, targetID string, req AppendEntriesRequest) (AppendEntriesResponse, error)
}

// MemoryTransport is an in-process transport for simulations and tests.
type MemoryTransport struct {
	mu    sync.RWMutex
	nodes map[string]*Node
}

// NewMemoryTransport returns a transport that routes RPCs to in-memory nodes.
func NewMemoryTransport() *MemoryTransport {
	return &MemoryTransport{nodes: make(map[string]*Node)}
}

// Register makes the node discoverable for RPCs.
func (t *MemoryTransport) Register(node *Node) error {
	if node == nil {
		return fmt.Errorf("register node: %w", ErrInvalidConfig)
	}
	if node.id == "" {
		return fmt.Errorf("register node: %w", ErrInvalidConfig)
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	t.nodes[node.id] = node
	return nil
}

// RequestVote routes the vote request to the target node.
func (t *MemoryTransport) RequestVote(ctx context.Context, targetID string, req RequestVoteRequest) (RequestVoteResponse, error) {
	node, err := t.lookup(targetID)
	if err != nil {
		return RequestVoteResponse{}, err
	}
	return node.HandleRequestVote(ctx, req), nil
}

// AppendEntries routes the heartbeat to the target node.
func (t *MemoryTransport) AppendEntries(ctx context.Context, targetID string, req AppendEntriesRequest) (AppendEntriesResponse, error) {
	node, err := t.lookup(targetID)
	if err != nil {
		return AppendEntriesResponse{}, err
	}
	return node.HandleAppendEntries(ctx, req), nil
}

func (t *MemoryTransport) lookup(targetID string) (*Node, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	node, ok := t.nodes[targetID]
	if !ok {
		return nil, fmt.Errorf("unknown target %s", targetID)
	}
	return node, nil
}
