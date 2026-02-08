package dht

import (
	"context"
	"errors"
	"sync"
)

var ErrValueNotFound = errors.New("value not found")

// Node models a DHT node with routing table and local key-value storage.
type Node struct {
	peer  Peer
	rt    *RoutingTable
	mu    sync.RWMutex
	store map[string][]byte
}

func NewNode(id NodeID, addr string, bucketSize int) *Node {
	peer := Peer{ID: id, Addr: addr}
	return &Node{
		peer:  peer,
		rt:    NewRoutingTable(id, bucketSize),
		store: make(map[string][]byte),
	}
}

func (n *Node) Peer() Peer {
	return n.peer
}

func (n *Node) RoutingTable() *RoutingTable {
	return n.rt
}

// StoreLocal writes a value into local storage.
func (n *Node) StoreLocal(key string, value []byte) {
	n.mu.Lock()
	defer n.mu.Unlock()

	copyValue := make([]byte, len(value))
	copy(copyValue, value)
	n.store[key] = copyValue
}

// FindValueLocal returns a value if present in local storage.
func (n *Node) FindValueLocal(key string) ([]byte, bool) {
	n.mu.RLock()
	defer n.mu.RUnlock()

	value, ok := n.store[key]
	if !ok {
		return nil, false
	}
	copyValue := make([]byte, len(value))
	copy(copyValue, value)
	return copyValue, true
}

// HandleFindNode implements a FIND_NODE RPC for this node.
func (n *Node) HandleFindNode(ctx context.Context, target NodeID, k int) ([]Peer, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	return n.rt.GetClosestPeers(target, k), nil
}

// HandleStore implements a STORE RPC for this node.
func (n *Node) HandleStore(ctx context.Context, key string, value []byte) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	n.StoreLocal(key, value)
	return nil
}

// HandleFindValue implements a FIND_VALUE RPC for this node.
// It returns the value if known, otherwise the k closest peers to the key.
func (n *Node) HandleFindValue(ctx context.Context, key string, k int) ([]byte, []Peer, bool, error) {
	select {
	case <-ctx.Done():
		return nil, nil, false, ctx.Err()
	default:
	}

	if value, ok := n.FindValueLocal(key); ok {
		return value, nil, true, nil
	}

	keyID := KeyToID(key)
	peers := n.rt.GetClosestPeers(keyID, k)
	return nil, peers, false, nil
}

// FindValueRequester models the FIND_VALUE RPC to a remote peer.
// If found is true, value must be non-nil and peers should be nil.
// If found is false, peers should contain the closest nodes to the key.
type FindValueRequester func(ctx context.Context, peer Peer, key string) (value []byte, peers []Peer, found bool, err error)

// StoreRequester models the STORE RPC to a remote peer.
type StoreRequester func(ctx context.Context, peer Peer, key string, value []byte) error

// IterativeFindValue performs Kademlia-style lookup for a value by key.
func IterativeFindValue(ctx context.Context, rt *RoutingTable, requester FindValueRequester, key string, opts FindNodeOptions) ([]byte, []Peer, error) {
	if rt == nil {
		return nil, nil, errors.New("routing table is nil")
	}
	if requester == nil {
		return nil, nil, errors.New("find value requester is nil")
	}

	k, alpha, maxRounds := normalizeFindNodeOptions(opts)

	target := KeyToID(key)
	shortlist := rt.GetClosestPeers(target, k)
	if len(shortlist) == 0 {
		return nil, nil, ErrNoClosePeers
	}

	shortlist = sortPeersByDistance(target, shortlist)
	queried := make(map[NodeID]bool, len(shortlist))

	for round := 0; round < maxRounds; round++ {
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		default:
		}

		batch := nextUnqueried(shortlist, queried, alpha)
		if len(batch) == 0 {
			break
		}

		for _, peer := range batch {
			queried[peer.ID] = true
			value, peers, found, err := requester(ctx, peer, key)
			if err != nil {
				continue
			}
			if found {
				return value, nil, nil
			}
			shortlist = mergePeers(shortlist, peers)
		}

		shortlist = sortPeersByDistance(target, shortlist)
		shortlist = trimPeers(shortlist, candidateLimit(k, alpha))
	}

	return nil, trimPeers(shortlist, k), ErrValueNotFound
}

// IterativeStore finds the closest peers for a key and stores the value.
func IterativeStore(ctx context.Context, rt *RoutingTable, find FindNodeRequester, store StoreRequester, key string, value []byte, opts FindNodeOptions) ([]Peer, error) {
	if rt == nil {
		return nil, errors.New("routing table is nil")
	}
	if find == nil {
		return nil, errors.New("find node requester is nil")
	}
	if store == nil {
		return nil, errors.New("store requester is nil")
	}

	target := KeyToID(key)
	closest, err := IterativeFindNode(ctx, rt, find, target, opts)
	if err != nil {
		return nil, err
	}

	for _, peer := range closest {
		if err := store(ctx, peer, key, value); err != nil {
			return closest, err
		}
	}

	return closest, nil
}
