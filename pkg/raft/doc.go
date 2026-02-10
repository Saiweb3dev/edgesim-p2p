// Package raft implements coordinator leader election using a Raft-style state machine.
//
// This package focuses on the leader election phase (timeouts, voting, and
// heartbeats). Log replication is intentionally omitted for the current
// simulation milestone.
//
// Basic usage:
//
//	transport := raft.NewMemoryTransport()
//	node, err := raft.NewNode(raft.Config{
//		ID:                "coord-1",
//		Peers:             []string{"coord-2", "coord-3"},
//		Transport:         transport,
//		ElectionTimeoutMin: 150 * time.Millisecond,
//		ElectionTimeoutMax: 300 * time.Millisecond,
//		HeartbeatInterval:  50 * time.Millisecond,
//	})
//	if err != nil {
//		// handle error
//	}
//	go func() { _ = node.Run(context.Background()) }()
package raft
