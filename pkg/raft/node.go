package raft

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// Config controls a node's election behavior.
type Config struct {
	ID                 string
	Peers              []string
	Transport          Transport
	Logger             *logrus.Entry
	Rand               *rand.Rand
	ElectionTimeoutMin time.Duration
	ElectionTimeoutMax time.Duration
	HeartbeatInterval  time.Duration
	RPCTimeout         time.Duration
	ApplyFunc          func(LogEntry)
}

// Node implements the Raft leader election state machine.
type Node struct {
	mu        sync.Mutex
	timerMu   sync.Mutex
	id        string
	peers     []string
	transport Transport
	logger    *logrus.Entry
	rng       *rand.Rand

	currentTerm   uint64
	votedFor      string
	leaderID      string
	state         State
	log           []LogEntry
	commitIndex   uint64
	lastApplied   uint64
	lastHeartbeat time.Time
	applyFn       func(LogEntry)

	electionMin time.Duration
	electionMax time.Duration
	heartbeat   time.Duration
	rpcTimeout  time.Duration

	electionTimer *time.Timer
}

// NewNode validates config and creates a new node.
func NewNode(cfg Config) (*Node, error) {
	if cfg.ID == "" {
		return nil, fmt.Errorf("node id: %w", ErrInvalidConfig)
	}
	if cfg.Transport == nil {
		return nil, ErrNilTransport
	}
	if cfg.ElectionTimeoutMin <= 0 || cfg.ElectionTimeoutMax <= 0 || cfg.HeartbeatInterval <= 0 {
		return nil, ErrInvalidTimeout
	}
	if cfg.ElectionTimeoutMax < cfg.ElectionTimeoutMin {
		return nil, fmt.Errorf("election timeout max < min: %w", ErrInvalidTimeout)
	}

	logger := cfg.Logger
	if logger == nil {
		base := logrus.New()
		base.SetFormatter(&logrus.TextFormatter{FullTimestamp: true})
		logger = base.WithField("component", "raft")
	}

	rng := cfg.Rand
	if rng == nil {
		rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	}

	rpcTimeout := cfg.RPCTimeout
	if rpcTimeout <= 0 {
		rpcTimeout = 250 * time.Millisecond
	}

	node := &Node{
		id:          cfg.ID,
		peers:       append([]string{}, cfg.Peers...),
		transport:   cfg.Transport,
		logger:      logger.WithField("node_id", cfg.ID),
		rng:         rng,
		state:       StateFollower,
		electionMin: cfg.ElectionTimeoutMin,
		electionMax: cfg.ElectionTimeoutMax,
		heartbeat:   cfg.HeartbeatInterval,
		rpcTimeout:  rpcTimeout,
		applyFn:     cfg.ApplyFunc,
	}

	return node, nil
}

// Run starts the election loop until the context is canceled.
func (n *Node) Run(ctx context.Context) error {
	n.resetElectionTimer()
	heartbeatTicker := time.NewTicker(n.heartbeat)
	defer heartbeatTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			n.stopElectionTimer()
			return ctx.Err()
		case <-n.electionTimer.C:
			n.onElectionTimeout(ctx)
		case <-heartbeatTicker.C:
			n.sendHeartbeats(ctx)
		}
	}
}

// HandleRequestVote handles incoming RequestVote RPCs.
func (n *Node) HandleRequestVote(ctx context.Context, req RequestVoteRequest) RequestVoteResponse {
	n.mu.Lock()
	defer n.mu.Unlock()

	if req.Term < n.currentTerm {
		return RequestVoteResponse{Term: n.currentTerm, VoteGranted: false}
	}

	if req.Term > n.currentTerm {
		n.stepDownLocked(req.Term, "")
	}

	if !n.isLogUpToDate(req.LastLogIndex, req.LastLogTerm) {
		return RequestVoteResponse{Term: n.currentTerm, VoteGranted: false}
	}

	grant := n.votedFor == "" || n.votedFor == req.CandidateID
	if grant {
		n.votedFor = req.CandidateID
	}

	if grant {
		n.resetElectionTimer()
	}

	return RequestVoteResponse{Term: n.currentTerm, VoteGranted: grant}
}

// HandleAppendEntries handles leader heartbeats.
func (n *Node) HandleAppendEntries(ctx context.Context, req AppendEntriesRequest) AppendEntriesResponse {
	n.mu.Lock()

	if req.Term < n.currentTerm {
		resp := AppendEntriesResponse{Term: n.currentTerm, Success: false}
		n.mu.Unlock()
		return resp
	}

	if req.Term > n.currentTerm {
		n.stepDownLocked(req.Term, req.LeaderID)
	}

	if len(req.Entries) > 0 && !n.matchPrevLog(req.PrevLogIndex, req.PrevLogTerm) {
		resp := AppendEntriesResponse{Term: n.currentTerm, Success: false}
		n.mu.Unlock()
		return resp
	}

	if len(req.Entries) > 0 {
		n.appendEntries(req.PrevLogIndex, req.Entries)
	}

	if req.LeaderCommit > n.commitIndex {
		lastIndex := n.lastLogIndex()
		if req.LeaderCommit < lastIndex {
			n.commitIndex = req.LeaderCommit
		} else {
			n.commitIndex = lastIndex
		}
	}

	n.lastHeartbeat = time.Now()
	entriesToApply := n.collectCommittedLocked()

	n.state = StateFollower
	n.leaderID = req.LeaderID
	n.resetElectionTimer()

	resp := AppendEntriesResponse{Term: n.currentTerm, Success: true}
	n.mu.Unlock()

	n.applyEntries(entriesToApply)
	return resp
}

// State returns the current role of the node.
func (n *Node) State() State {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.state
}

// CurrentTerm returns the node's current term.
func (n *Node) CurrentTerm() uint64 {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.currentTerm
}

// LeaderID returns the current leader identifier, if known.
func (n *Node) LeaderID() string {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.leaderID
}

// CommitIndex returns the highest committed log index.
func (n *Node) CommitIndex() uint64 {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.commitIndex
}

// LastHeartbeatTime returns the last time a heartbeat was observed.
func (n *Node) LastHeartbeatTime() time.Time {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.lastHeartbeat
}

// LogSnapshot returns a copy of the log for tests or diagnostics.
func (n *Node) LogSnapshot() []LogEntry {
	n.mu.Lock()
	defer n.mu.Unlock()
	copyLog := make([]LogEntry, len(n.log))
	copy(copyLog, n.log)
	return copyLog
}

func (n *Node) onElectionTimeout(ctx context.Context) {
	n.mu.Lock()
	if n.state == StateLeader {
		n.mu.Unlock()
		n.resetElectionTimer()
		return
	}
	n.mu.Unlock()

	n.startElection(ctx)
}

func (n *Node) startElection(ctx context.Context) {
	n.mu.Lock()
	n.state = StateCandidate
	n.currentTerm++
	term := n.currentTerm
	n.votedFor = n.id
	n.leaderID = ""
	n.mu.Unlock()

	n.logger.WithFields(logrus.Fields{
		"term":  term,
		"state": n.state,
	}).Info("starting election")

	n.resetElectionTimer()

	votes := 1
	needed := n.quorum()
	responses := 0
	respCh := make(chan RequestVoteResponse, len(n.peers))

	for _, peerID := range n.peers {
		peerID := peerID
		go func() {
			rpcCtx, cancel := context.WithTimeout(ctx, n.rpcTimeout)
			defer cancel()

			resp, err := n.transport.RequestVote(rpcCtx, peerID, RequestVoteRequest{
				Term:         term,
				CandidateID:  n.id,
				LastLogIndex: n.lastLogIndex(),
				LastLogTerm:  n.lastLogTerm(),
			})
			if err != nil {
				n.logger.WithError(err).WithField("peer_id", peerID).Warn("request vote failed")
				respCh <- RequestVoteResponse{Term: term, VoteGranted: false}
				return
			}
			respCh <- resp
		}()
	}

	for responses < len(n.peers) {
		select {
		case <-ctx.Done():
			return
		case resp := <-respCh:
			responses++

			n.mu.Lock()
			if resp.Term > n.currentTerm {
				n.stepDownLocked(resp.Term, "")
				n.mu.Unlock()
				n.resetElectionTimer()
				return
			}
			if n.state != StateCandidate || n.currentTerm != term {
				n.mu.Unlock()
				return
			}
			n.mu.Unlock()

			if resp.VoteGranted {
				votes++
				if votes >= needed {
					n.becomeLeader(ctx)
					return
				}
			}
		}
	}
}

func (n *Node) becomeLeader(ctx context.Context) {
	n.mu.Lock()
	if n.state != StateCandidate {
		n.mu.Unlock()
		return
	}
	if n.leaderID == n.id {
		n.mu.Unlock()
		return
	}

	n.state = StateLeader
	n.leaderID = n.id
	term := n.currentTerm
	n.mu.Unlock()

	n.logger.WithFields(logrus.Fields{
		"term":  term,
		"state": n.state,
	}).Info("leader elected")

	n.sendHeartbeats(ctx)
}

// Propose appends a command on the leader and replicates it to followers.
func (n *Node) Propose(ctx context.Context, command string) error {
	n.mu.Lock()
	if n.state != StateLeader {
		n.mu.Unlock()
		return ErrNotLeader
	}
	prevIndex := n.lastLogIndex()
	prevTerm := n.lastLogTerm()
	entry := LogEntry{Term: n.currentTerm, Command: command}
	n.log = append(n.log, entry)
	newIndex := n.lastLogIndex()
	n.mu.Unlock()

	acks := 1
	needed := n.quorum()
	respCh := make(chan AppendEntriesResponse, len(n.peers))

	for _, peerID := range n.peers {
		peerID := peerID
		go func() {
			rpcCtx, cancel := context.WithTimeout(ctx, n.rpcTimeout)
			defer cancel()

			resp, err := n.transport.AppendEntries(rpcCtx, peerID, AppendEntriesRequest{
				Term:         entry.Term,
				LeaderID:     n.id,
				PrevLogIndex: prevIndex,
				PrevLogTerm:  prevTerm,
				Entries:      []LogEntry{entry},
				LeaderCommit: n.commitIndex,
			})
			if err != nil {
				n.logger.WithError(err).WithField("peer_id", peerID).Warn("append entries failed")
				respCh <- AppendEntriesResponse{Term: entry.Term, Success: false}
				return
			}
			respCh <- resp
		}()
	}

	responses := 0
	for responses < len(n.peers) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case resp := <-respCh:
			responses++
			if resp.Term > entry.Term {
				n.mu.Lock()
				if resp.Term > n.currentTerm {
					n.stepDownLocked(resp.Term, "")
				}
				n.mu.Unlock()
				return ErrNotLeader
			}
			if resp.Success {
				acks++
				if acks >= needed {
					n.mu.Lock()
					var entriesToApply []LogEntry
					if n.commitIndex < newIndex {
						n.commitIndex = newIndex
						entriesToApply = n.collectCommittedLocked()
					}
					n.mu.Unlock()
					n.applyEntries(entriesToApply)
					return nil
				}
			}
		}
	}

	return fmt.Errorf("replication failed")
}

func (n *Node) sendHeartbeats(ctx context.Context) {
	n.mu.Lock()
	if n.state != StateLeader {
		n.mu.Unlock()
		return
	}
	term := n.currentTerm
	n.lastHeartbeat = time.Now()
	n.mu.Unlock()

	for _, peerID := range n.peers {
		peerID := peerID
		go func() {
			rpcCtx, cancel := context.WithTimeout(ctx, n.rpcTimeout)
			defer cancel()

			resp, err := n.transport.AppendEntries(rpcCtx, peerID, AppendEntriesRequest{
				Term:         term,
				LeaderID:     n.id,
				PrevLogIndex: n.lastLogIndex(),
				PrevLogTerm:  n.lastLogTerm(),
				LeaderCommit: n.commitIndex,
			})
			if err != nil {
				n.logger.WithError(err).WithField("peer_id", peerID).Warn("heartbeat failed")
				return
			}
			if resp.Term > term {
				n.mu.Lock()
				if resp.Term > n.currentTerm {
					n.stepDownLocked(resp.Term, "")
				}
				n.mu.Unlock()
				n.resetElectionTimer()
			}
		}()
	}
}

func (n *Node) stepDownLocked(term uint64, leaderID string) {
	if term > n.currentTerm {
		n.currentTerm = term
	}
	n.votedFor = ""
	n.state = StateFollower
	n.leaderID = leaderID
}

func (n *Node) lastLogIndex() uint64 {
	return uint64(len(n.log))
}

func (n *Node) lastLogTerm() uint64 {
	if len(n.log) == 0 {
		return 0
	}
	return n.log[len(n.log)-1].Term
}

func (n *Node) isLogUpToDate(candidateIndex uint64, candidateTerm uint64) bool {
	localTerm := n.lastLogTerm()
	if candidateTerm != localTerm {
		return candidateTerm > localTerm
	}
	return candidateIndex >= n.lastLogIndex()
}

func (n *Node) matchPrevLog(prevIndex uint64, prevTerm uint64) bool {
	if prevIndex == 0 {
		return true
	}
	if prevIndex > uint64(len(n.log)) {
		return false
	}
	return n.log[prevIndex-1].Term == prevTerm
}

func (n *Node) appendEntries(prevIndex uint64, entries []LogEntry) {
	for i, entry := range entries {
		index := int(prevIndex) + i
		if index < len(n.log) {
			if n.log[index].Term != entry.Term {
				n.log = n.log[:index]
				n.log = append(n.log, entry)
			}
			continue
		}
		n.log = append(n.log, entry)
	}
}

func (n *Node) collectCommittedLocked() []LogEntry {
	if n.lastApplied >= n.commitIndex {
		return nil
	}
	count := n.commitIndex - n.lastApplied
	entries := make([]LogEntry, 0, count)
	for n.lastApplied < n.commitIndex {
		entries = append(entries, n.log[n.lastApplied])
		n.lastApplied++
	}
	return entries
}

func (n *Node) applyEntries(entries []LogEntry) {
	if n.applyFn == nil || len(entries) == 0 {
		return
	}
	for _, entry := range entries {
		n.applyFn(entry)
	}
}

func (n *Node) quorum() int {
	return (len(n.peers)+1)/2 + 1
}

func (n *Node) resetElectionTimer() {
	n.timerMu.Lock()
	defer n.timerMu.Unlock()

	next := n.nextElectionTimeout()
	if n.electionTimer == nil {
		n.electionTimer = time.NewTimer(next)
		return
	}

	resetTimer(n.electionTimer, next)
}

func (n *Node) stopElectionTimer() {
	n.timerMu.Lock()
	defer n.timerMu.Unlock()

	if n.electionTimer == nil {
		return
	}
	if !n.electionTimer.Stop() {
		select {
		case <-n.electionTimer.C:
		default:
		}
	}
}

func (n *Node) nextElectionTimeout() time.Duration {
	if n.electionMin == n.electionMax {
		return n.electionMin
	}

	delta := n.electionMax - n.electionMin
	offset := time.Duration(n.rng.Int63n(int64(delta)))
	return n.electionMin + offset
}

func resetTimer(timer *time.Timer, duration time.Duration) {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	timer.Reset(duration)
}
