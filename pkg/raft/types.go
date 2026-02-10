package raft

import "errors"

// State represents the Raft role of a node.
type State string

const (
	StateFollower  State = "follower"
	StateCandidate State = "candidate"
	StateLeader    State = "leader"
)

var (
	ErrInvalidConfig  = errors.New("invalid raft config")
	ErrNilTransport   = errors.New("transport is required")
	ErrInvalidTimeout = errors.New("timeouts must be positive")
	ErrNotLeader      = errors.New("not leader")
)

// RequestVoteRequest is sent by candidates to gather votes.
type RequestVoteRequest struct {
	Term         uint64
	CandidateID  string
	LastLogIndex uint64
	LastLogTerm  uint64
}

// RequestVoteResponse carries the vote decision.
type RequestVoteResponse struct {
	Term        uint64
	VoteGranted bool
}

// AppendEntriesRequest is used by leaders as a heartbeat.
type AppendEntriesRequest struct {
	Term         uint64
	LeaderID     string
	PrevLogIndex uint64
	PrevLogTerm  uint64
	Entries      []LogEntry
	LeaderCommit uint64
}

// AppendEntriesResponse acknowledges a heartbeat.
type AppendEntriesResponse struct {
	Term    uint64
	Success bool
}

// LogEntry represents a single replicated command.
type LogEntry struct {
	Term    uint64
	Command string
}
