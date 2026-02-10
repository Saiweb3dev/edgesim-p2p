package main

import (
	"context"
	"fmt"

	"github.com/Saiweb3dev/edgesim-p2p/pkg/raft"
)

// tcpTransport sends Raft RPCs to peer coordinators over TCP.
type tcpTransport struct {
	peers map[string]string
}

func newTCPTransport(peers map[string]string) *tcpTransport {
	copyPeers := make(map[string]string, len(peers))
	for id, addr := range peers {
		copyPeers[id] = addr
	}
	return &tcpTransport{peers: copyPeers}
}

func (t *tcpTransport) RequestVote(ctx context.Context, targetID string, req raft.RequestVoteRequest) (raft.RequestVoteResponse, error) {
	addr, ok := t.peers[targetID]
	if !ok {
		return raft.RequestVoteResponse{}, fmt.Errorf("unknown peer %s", targetID)
	}

	reqEnvelope := raftEnvelope{
		Type:       rpcRequestVote,
		RequestVote: &req,
	}
	var resp raft.RequestVoteResponse
	if err := sendRaftRPC(ctx, addr, reqEnvelope, &resp); err != nil {
		return raft.RequestVoteResponse{}, err
	}
	return resp, nil
}

func (t *tcpTransport) AppendEntries(ctx context.Context, targetID string, req raft.AppendEntriesRequest) (raft.AppendEntriesResponse, error) {
	addr, ok := t.peers[targetID]
	if !ok {
		return raft.AppendEntriesResponse{}, fmt.Errorf("unknown peer %s", targetID)
	}

	reqEnvelope := raftEnvelope{
		Type:          rpcAppendEntries,
		AppendEntries: &req,
	}
	var resp raft.AppendEntriesResponse
	if err := sendRaftRPC(ctx, addr, reqEnvelope, &resp); err != nil {
		return raft.AppendEntriesResponse{}, err
	}
	return resp, nil
}
