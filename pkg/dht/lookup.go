package dht

import (
	"context"
	"errors"
	"sort"
)

var ErrNoClosePeers = errors.New("no known peers to start lookup")

// FindNodeRequester models the FIND_NODE RPC to a remote peer.
// It should return the peer's k closest nodes to the target ID.
//
// Example:
//
//	peers, err := requester(ctx, peer, targetID)
//	if err != nil { /* handle */ }
//	// merge peers into shortlist
//
// The returned peers can include the target if known.
// Callers should expect best-effort behavior and partial failures.
type FindNodeRequester func(ctx context.Context, peer Peer, target NodeID) ([]Peer, error)

// FindNodeOptions controls the iterative lookup behavior.
type FindNodeOptions struct {
	K         int
	Alpha     int
	MaxRounds int
}

// IterativeFindNode performs Kademlia-style iterative lookup for target.
func IterativeFindNode(ctx context.Context, rt *RoutingTable, requester FindNodeRequester, target NodeID, opts FindNodeOptions) ([]Peer, error) {
	if rt == nil {
		return nil, errors.New("routing table is nil")
	}
	if requester == nil {
		return nil, errors.New("find node requester is nil")
	}

	k, alpha, maxRounds := normalizeFindNodeOptions(opts)

	shortlist := rt.GetClosestPeers(target, k)
	if len(shortlist) == 0 {
		return nil, ErrNoClosePeers
	}

	shortlist = sortPeersByDistance(target, shortlist)
	queried := make(map[NodeID]bool, len(shortlist))

	for round := 0; round < maxRounds; round++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		batch := nextUnqueried(shortlist, queried, alpha)
		if len(batch) == 0 {
			break
		}

		for _, peer := range batch {
			queried[peer.ID] = true
			response, err := requester(ctx, peer, target)
			if err != nil {
				continue
			}
			shortlist = mergePeers(shortlist, response)
		}

		shortlist = sortPeersByDistance(target, shortlist)
		shortlist = trimPeers(shortlist, candidateLimit(k, alpha))
	}

	return trimPeers(shortlist, k), nil
}

func normalizeFindNodeOptions(opts FindNodeOptions) (int, int, int) {
	k := opts.K
	if k <= 0 {
		k = 20
	}
	alpha := opts.Alpha
	if alpha <= 0 {
		alpha = 3
	}
	maxRounds := opts.MaxRounds
	if maxRounds <= 0 {
		maxRounds = NodeIDLength * 8
	}
	return k, alpha, maxRounds
}

func nextUnqueried(shortlist []Peer, queried map[NodeID]bool, alpha int) []Peer {
	batch := make([]Peer, 0, alpha)
	for _, peer := range shortlist {
		if queried[peer.ID] {
			continue
		}
		batch = append(batch, peer)
		if len(batch) >= alpha {
			break
		}
	}
	return batch
}

func mergePeers(existing []Peer, incoming []Peer) []Peer {
	if len(incoming) == 0 {
		return existing
	}

	seen := make(map[NodeID]Peer, len(existing)+len(incoming))
	for _, peer := range existing {
		seen[peer.ID] = peer
	}
	for _, peer := range incoming {
		if peer.ID == (NodeID{}) {
			continue
		}
		seen[peer.ID] = peer
	}

	merged := make([]Peer, 0, len(seen))
	for _, peer := range seen {
		merged = append(merged, peer)
	}
	return merged
}

func trimPeers(peers []Peer, k int) []Peer {
	if k <= 0 || len(peers) <= k {
		return peers
	}
	return peers[:k]
}

func candidateLimit(k int, alpha int) int {
	limit := k * 4
	minLimit := alpha * 2
	if limit < minLimit {
		limit = minLimit
	}
	if limit < k {
		limit = k
	}
	return limit
}

func sortPeersByDistance(target NodeID, peers []Peer) []Peer {
	items := make([]peerDistance, 0, len(peers))
	for _, peer := range peers {
		items = append(items, peerDistance{peer: peer, distance: xorDistance(target, peer.ID)})
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].distance.Cmp(items[j].distance) < 0
	})

	result := make([]Peer, 0, len(items))
	for _, item := range items {
		result = append(result, item.peer)
	}
	return result
}
