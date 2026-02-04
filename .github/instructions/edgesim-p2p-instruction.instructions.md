---
applyTo: '**'
---
# Copilot Agent Instructions - EdgeSim P2P

## Project Context

**Project:** EdgeSim - Distributed P2P Coordination System for Edge Sensors  
**Goal:** Build a production-grade distributed system simulating sensor network coordination  
**Timeline:** 6 weeks (120 hours total, 4 hours/day)  
**Tech Stack:** Go 1.21+, Docker, gRPC, Prometheus, Grafana  
**Skill Level:** Fresh grad, knows theory (Raft, DHT, P2P) but first implementation in Go

---

## Architecture Overview

### System Components

1. **Sensor Nodes (50+)**
   - Simulate temperature sensors with geographic distribution
   - Generate fake sensor data every 5 seconds
   - Participate in Kademlia DHT for peer discovery
   - Use gossip protocol to propagate readings
   - Run as Docker containers

2. **Coordinator Nodes (3)**
   - Form Raft consensus cluster
   - Elected leader aggregates cluster state
   - Provide query API (gRPC) for data retrieval
   - Handle coordinator failures with automatic re-election

3. **Observability Stack**
   - Prometheus for metrics collection
   - Grafana for visualization
   - Structured logging with contextual fields

### Key Technical Decisions

**Why Kademlia DHT?**
- O(log N) lookup complexity
- XOR distance metric ensures balanced routing
- Self-healing network topology
- Used by BitTorrent, Ethereum, IPFS

**Why Raft Consensus?**
- Easier to understand than Paxos
- Strong consistency guarantees
- Well-documented (Raft paper + students' guide)
- Industry proven (etcd, CockroachDB)

**Why Gossip Protocol?**
- Fault-tolerant broadcast
- No single point of failure
- Trade bandwidth for reliability
- Used by Cassandra, Consul

**Why Go?**
- Excellent concurrency (goroutines)
- Strong networking stdlib
- Fast compilation
- Industry standard for distributed systems

---

## Code Quality Standards

### Go Style Guidelines

Follow **Effective Go** and **Go Code Review Comments**:
- Use `gofmt` for formatting (enforced in CI)
- Run `golangci-lint` before commits
- Package names: lowercase, no underscores (e.g., `dht` not `DHT` or `dht_routing`)
- Exported names: use MixedCaps (e.g., `NodeID` not `Node_ID`)

### Naming Conventions

**Variables:**
```go
// Good
nodeID := generateNodeID()
peerList := dht.GetPeers()
ctx := context.Background()

// Bad
node_id := generateNodeID()  // No underscores
n := generateNodeID()        // Too short for important vars
peers_list := dht.GetPeers() // Redundant suffix
```

**Functions:**
```go
// Good - clear, verb-based
func (n *Node) SendMessage(peer *Peer, msg Message) error
func (dht *DHT) FindClosestPeers(targetID NodeID, k int) []*Peer

// Bad
func (n *Node) Msg(p *Peer, m Message) error  // Abbreviations
func (dht *DHT) GetPeers(id NodeID) []*Peer   // Ambiguous
```

**Interfaces:**
```go
// Good - single method interfaces use "-er" suffix
type Sender interface {
    Send(msg Message) error
}

type MessageHandler interface {
    HandleMessage(msg Message) error
}

// Bad
type SendInterface interface {  // Don't use "Interface" suffix
    Send(msg Message) error
}
```

### Error Handling

**ALWAYS handle errors explicitly:**
```go
// Good
data, err := readSensorData()
if err != nil {
    return fmt.Errorf("failed to read sensor data: %w", err)
}

// Bad
data, _ := readSensorData()  // NEVER ignore errors with _

// Good - wrapping errors with context
if err := node.Join(bootstrapPeers); err != nil {
    return fmt.Errorf("node %s failed to join network: %w", node.ID, err)
}
```

**Use custom errors for domain logic:**
```go
var (
    ErrNodeNotFound     = errors.New("node not found in routing table")
    ErrLeaderNotElected = errors.New("no leader elected in cluster")
    ErrInvalidNodeID    = errors.New("node ID must be 160 bits")
)
```

### Concurrency Patterns

**Always protect shared state:**
```go
// Good
type RoutingTable struct {
    mu      sync.RWMutex
    buckets []*Bucket
}

func (rt *RoutingTable) AddPeer(peer *Peer) {
    rt.mu.Lock()
    defer rt.mu.Unlock()
    // ... modify buckets
}

func (rt *RoutingTable) GetPeers() []*Peer {
    rt.mu.RLock()  // Read lock for concurrent reads
    defer rt.mu.RUnlock()
    // ... read buckets
}
```

**Use context for cancellation:**
```go
// Good
func (n *Node) FindNode(ctx context.Context, targetID NodeID) (*Peer, error) {
    select {
    case result := <-n.search(targetID):
        return result, nil
    case <-ctx.Done():
        return nil, ctx.Err()
    }
}
```

**Avoid goroutine leaks:**
```go
// Good - always have exit condition
func (n *Node) startHeartbeat(ctx context.Context) {
    ticker := time.NewTicker(50 * time.Millisecond)
    defer ticker.Stop()
    
    for {
        select {
        case <-ticker.C:
            n.sendHeartbeat()
        case <-ctx.Done():
            return  // Exit when context cancelled
        }
    }
}
```

---

## Testing Requirements

### Coverage Expectations
- **Minimum:** 70% overall coverage
- **Critical paths:** 90%+ (DHT lookup, Raft election, Gossip propagation)
- **Run tests before every commit:** `make test`

### Test Structure

**Use table-driven tests:**
```go
func TestXORDistance(t *testing.T) {
    tests := []struct {
        name     string
        id1      NodeID
        id2      NodeID
        expected *big.Int
    }{
        {
            name:     "identical IDs",
            id1:      NodeID{0x00, 0x01},
            id2:      NodeID{0x00, 0x01},
            expected: big.NewInt(0),
        },
        {
            name:     "different IDs",
            id1:      NodeID{0xFF, 0x00},
            id2:      NodeID{0x00, 0xFF},
            expected: big.NewInt(65535),
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := XORDistance(tt.id1, tt.id2)
            if result.Cmp(tt.expected) != 0 {
                t.Errorf("got %v, want %v", result, tt.expected)
            }
        })
    }
}
```

**Use testify for assertions (optional but preferred):**
```go
func TestRoutingTable(t *testing.T) {
    rt := NewRoutingTable(localID)
    peer := &Peer{ID: peerID}
    
    err := rt.AddPeer(peer)
    require.NoError(t, err)
    
    peers := rt.GetClosestPeers(targetID, 5)
    assert.Len(t, peers, 1)
    assert.Equal(t, peerID, peers[0].ID)
}
```

### Integration Tests

**Use Docker Compose for multi-node tests:**
```go
// integration_test.go
func TestDHTLookup(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping integration test")
    }
    
    // Start 10 nodes via docker-compose
    compose := setupDockerCompose(t, 10)
    defer compose.Teardown()
    
    // Test DHT lookup across network
    // ...
}
```

---

## Documentation Standards

### Package Documentation

**Every package must have doc.go:**
```go
// Package dht implements a Kademlia Distributed Hash Table for peer discovery.
//
// The DHT provides O(log N) lookup complexity and self-healing topology.
// Each node maintains a routing table with k-buckets organized by XOR distance.
//
// Basic usage:
//
//     dht := dht.New(localID, bootstrapPeers)
//     peer, err := dht.FindNode(ctx, targetID)
//
// References:
// - Kademlia Paper: https://pdos.csail.mit.edu/~petar/papers/maymounkov-kademlia-lncs.pdf
package dht
```

### Function Documentation

**Document exported functions with examples:**
```go
// FindClosestPeers returns the k peers closest to targetID by XOR distance.
// It performs an iterative lookup, querying α nodes in parallel at each step.
//
// The returned peers are sorted by distance (closest first).
// If fewer than k peers are found, all known peers are returned.
//
// Example:
//
//     peers := dht.FindClosestPeers(targetID, 20)
//     for _, peer := range peers {
//         fmt.Printf("Peer %s at distance %v\n", peer.ID, XORDistance(targetID, peer.ID))
//     }
func (dht *DHT) FindClosestPeers(targetID NodeID, k int) []*Peer {
    // Implementation
}
```

### Code Comments

**Explain WHY, not WHAT:**
```go
// Good - explains reasoning
// Use XOR metric instead of numerical distance because it ensures:
// 1. Symmetric distance: d(A,B) = d(B,A)
// 2. Triangle inequality holds
// 3. Balanced tree structure
distance := XORDistance(nodeA, nodeB)

// Bad - just repeats code
// Calculate XOR distance
distance := XORDistance(nodeA, nodeB)
```

---

## Project Structure Conventions

```
edgesim-p2p/
├── cmd/                    # Application entrypoints
│   ├── node/              # Sensor node binary
│   │   └── main.go
│   └── coordinator/       # Raft coordinator binary
│       └── main.go
├── pkg/                   # Public libraries (reusable)
│   ├── dht/              # Kademlia DHT implementation
│   │   ├── dht.go
│   │   ├── bucket.go
│   │   ├── routing.go
│   │   └── dht_test.go
│   ├── gossip/           # Gossip protocol
│   ├── raft/             # Raft consensus
│   └── sensor/           # Sensor simulation
├── internal/             # Private code (not importable)
│   ├── metrics/         # Prometheus metrics helpers
│   └── config/          # Configuration loading
├── deployments/
│   └── compose/         # Docker Compose files
├── docs/                # Documentation
│   ├── design.md       # Design document
│   └── architecture/   # Architecture diagrams
└── scripts/            # Build/deploy scripts
```

**File naming:**
- Implementation: `dht.go`, `routing.go`
- Tests: `dht_test.go`, `routing_test.go`
- No `_impl.go` or `implementation.go` suffixes

---

## Metrics & Observability

### Prometheus Metrics Naming

```go
// Good - descriptive with unit suffix
var (
    dhtLookupLatency = prometheus.NewHistogram(prometheus.HistogramOpts{
        Name: "edgesim_dht_lookup_latency_milliseconds",
        Help: "DHT node lookup latency in milliseconds",
        Buckets: prometheus.ExponentialBuckets(1, 2, 10), // 1ms to 512ms
    })
    
    raftIsLeader = prometheus.NewGauge(prometheus.GaugeOpts{
        Name: "edgesim_raft_is_leader",
        Help: "1 if this node is the Raft leader, 0 otherwise",
    })
    
    gossipMessagesSent = prometheus.NewCounterVec(prometheus.CounterOpts{
        Name: "edgesim_gossip_messages_sent_total",
        Help: "Total number of gossip messages sent by type",
    }, []string{"message_type"})
)
```

### Structured Logging

**Use logrus or zap with consistent fields:**
```go
log.WithFields(log.Fields{
    "node_id":   n.ID.String(),
    "peer_id":   peer.ID.String(),
    "operation": "FIND_NODE",
    "latency_ms": latency.Milliseconds(),
}).Info("DHT lookup completed")

// NOT this:
log.Printf("Node %s looked up peer %s in %v", n.ID, peer.ID, latency)
```

---

## Common Patterns to Follow

### 1. Factory Functions

```go
// Good - clear constructor
func NewDHT(localID NodeID, k int, alpha int) *DHT {
    return &DHT{
        localID:      localID,
        routingTable: NewRoutingTable(localID, k),
        alpha:        alpha,
        peers:        make(map[NodeID]*Peer),
    }
}
```

### 2. Options Pattern for Complex Constructors

```go
type NodeOption func(*Node)

func WithBootstrapPeers(peers []*Peer) NodeOption {
    return func(n *Node) {
        n.bootstrapPeers = peers
    }
}

func WithMetrics(m *Metrics) NodeOption {
    return func(n *Node) {
        n.metrics = m
    }
}

// Usage
node := NewNode(nodeID, 
    WithBootstrapPeers(peers),
    WithMetrics(metrics),
)
```

### 3. Context Propagation

```go
// ALWAYS pass context as first parameter
func (n *Node) Join(ctx context.Context, peers []*Peer) error
func (dht *DHT) Lookup(ctx context.Context, key string) ([]byte, error)
```

### 4. Graceful Shutdown

```go
func (n *Node) Start(ctx context.Context) error {
    errCh := make(chan error, 1)
    
    go func() {
        errCh <- n.serve()
    }()
    
    select {
    case err := <-errCh:
        return err
    case <-ctx.Done():
        return n.shutdown()
    }
}

func (n *Node) shutdown() error {
    n.mu.Lock()
    defer n.mu.Unlock()
    
    if n.conn != nil {
        return n.conn.Close()
    }
    return nil
}
```

---

## Common Anti-Patterns to Avoid

### ❌ Global State

```go
// BAD
var globalDHT *DHT  // Never use package-level mutable state

// GOOD
type Node struct {
    dht *DHT  // Dependency injection
}
```

### ❌ Panic in Library Code

```go
// BAD
func (dht *DHT) AddPeer(peer *Peer) {
    if peer == nil {
        panic("peer cannot be nil")  // Don't panic in libraries
    }
}

// GOOD
func (dht *DHT) AddPeer(peer *Peer) error {
    if peer == nil {
        return ErrNilPeer
    }
    // ...
}
```

### ❌ Naked Returns

```go
// BAD
func parseNodeID(s string) (id NodeID, err error) {
    // ... complex logic
    return  // What are we returning? Unclear!
}

// GOOD
func parseNodeID(s string) (NodeID, error) {
    // ... complex logic
    return nodeID, nil  // Explicit
}
```

### ❌ Interface Pollution

```go
// BAD - don't create interfaces for mocking
type SensorReader interface {  // Only used once
    Read() (float64, error)
}

// GOOD - accept concrete types, use interfaces where needed
func ReadSensor(s *Sensor) (float64, error) {
    return s.Read()
}
```

---

## gRPC Service Definitions

### Proto Style Guide

```protobuf
// sensor.proto
syntax = "proto3";

package edgesim.v1;

option go_package = "github.com/yourusername/edgesim-p2p/pkg/proto/v1";

// SensorService handles sensor data queries.
service SensorService {
  // GetReading returns the latest sensor reading.
  rpc GetReading(GetReadingRequest) returns (GetReadingResponse);
  
  // StreamReadings streams sensor readings in real-time.
  rpc StreamReadings(StreamReadingsRequest) returns (stream SensorReading);
}

message SensorReading {
  string node_id = 1;
  double temperature = 2;
  Location location = 3;
  int64 timestamp_unix = 4;
}

message Location {
  double latitude = 1;
  double longitude = 2;
}
```

---

## Performance Considerations

### Memory Optimization

```go
// Use sync.Pool for frequently allocated objects
var messagePool = sync.Pool{
    New: func() interface{} {
        return &Message{}
    },
}

func getMessage() *Message {
    return messagePool.Get().(*Message)
}

func putMessage(m *Message) {
    m.Reset()  // Clear data
    messagePool.Put(m)
}
```

### Network Optimization

```go
// Reuse connections
type ConnectionPool struct {
    conns map[NodeID]*grpc.ClientConn
    mu    sync.RWMutex
}

func (p *ConnectionPool) GetConn(nodeID NodeID, addr string) (*grpc.ClientConn, error) {
    p.mu.RLock()
    if conn, ok := p.conns[nodeID]; ok {
        p.mu.RUnlock()
        return conn, nil
    }
    p.mu.RUnlock()
    
    // Create new connection if not exists
    // ... (with write lock)
}
```

---

## CI/CD Requirements

### GitHub Actions Checks

Every PR must pass:
1. `make test` - All tests passing
2. `make lint` - golangci-lint with no errors
3. `make vet` - go vet with no warnings
4. Coverage report - Must not decrease

### Makefile Targets

```makefile
.PHONY: test lint vet build

test:
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

lint:
	golangci-lint run ./...

vet:
	go vet ./...

build:
	go build -o bin/node ./cmd/node
	go build -o bin/coordinator ./cmd/coordinator

docker-build:
	docker build -t edgesim-node:latest -f deployments/docker/Dockerfile.node .
	docker build -t edgesim-coordinator:latest -f deployments/docker/Dockerfile.coordinator .

run-dev:
	docker-compose -f deployments/compose/docker-compose.dev.yml up

run-prod:
	docker-compose -f deployments/compose/docker-compose.prod.yml up
```

---

## Git Commit Message Standards

Follow **Conventional Commits:**

```
<type>(<scope>): <subject>

<body>

<footer>
```

**Types:**
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation only
- `test`: Adding tests
- `refactor`: Code change that neither fixes a bug nor adds a feature
- `perf`: Performance improvement
- `chore`: Build process or auxiliary tool changes

**Examples:**

```
feat(dht): implement k-bucket routing table

Add k-bucket data structure with LRU eviction policy.
Each bucket holds up to k peers, organized by XOR distance.

Refs: #12
```

```
fix(raft): prevent split-brain during network partition

Always check term number before voting to ensure only one
leader per term.

Fixes: #45
```

---

## When to Ask for Help

**DO ask when:**
- Stuck on same bug for >30 minutes
- Unsure about architectural decision
- Performance issue you can't profile
- Race condition you can't reproduce

**DON'T ask before:**
- Reading the relevant paper/docs
- Checking existing code for patterns
- Running the debugger/profiler
- Searching GitHub issues

**How to ask:**
1. What you tried
2. What you expected
3. What actually happened
4. Minimal reproduction code

---

## Project-Specific Context for Copilot

When generating code, remember:

1. **This is a learning project** - Prefer clarity over cleverness
2. **Simulate production constraints** - Add observability, handle errors properly
3. **Docker is our deployment** - All code must work in containers
4. **4-hour daily limit** - Prioritize shipping over perfection
5. **Public accountability** - Code will be reviewed weekly on LinkedIn

### When Suggesting Code

**Always include:**
- Error handling
- Context propagation
- Logging at key points
- Metrics where applicable
- Unit test outline

**Example prompt response:**

User: "Implement FIND_NODE RPC"

Copilot should provide:
```go
// pkg/dht/rpc.go

// FindNode implements the FIND_NODE RPC.
// It returns the k peers closest to the target ID from the local routing table.
func (d *DHT) FindNode(ctx context.Context, targetID NodeID) ([]*Peer, error) {
    start := time.Now()
    defer func() {
        d.metrics.dhtLookupLatency.Observe(float64(time.Since(start).Milliseconds()))
    }()
    
    d.logger.WithFields(log.Fields{
        "target_id": targetID.String(),
        "operation": "FIND_NODE",
    }).Debug("starting node lookup")
    
    peers := d.routingTable.GetClosestPeers(targetID, d.k)
    
    if len(peers) == 0 {
        return nil, ErrNoClosePeers
    }
    
    return peers, nil
}

// Test outline:
// func TestDHT_FindNode(t *testing.T) {
//     tests := []struct {
//         name     string
//         targetID NodeID
//         want     int  // expected number of peers
//     }{
//         {"target in routing table", knownID, 1},
//         {"target not in routing table", unknownID, 20},
//     }
//     // ... implement
// }
```

---

## Quick Reference Commands

```bash
# Development
make test              # Run all tests
make lint              # Run linter
make run-dev           # Start 5 nodes locally
make run-prod          # Start 50 nodes

# Docker
docker-compose logs -f node-1  # View node logs
docker-compose down            # Stop all containers

# Metrics
open http://localhost:9090     # Prometheus
open http://localhost:3000     # Grafana

# Profiling
go tool pprof http://localhost:6060/debug/pprof/profile
go tool pprof http://localhost:6060/debug/pprof/heap
```

---

## Summary: Core Principles

1. **Clarity > Cleverness** - Code is read more than written
2. **Test Everything** - If it's not tested, it's broken
3. **Handle Errors** - Never ignore errors
4. **Add Context** - Logs, metrics, comments explain WHY
5. **Ship Incrementally** - Small PRs, frequent commits
6. **Document Decisions** - Update design doc when architecture changes
7. **Profile Before Optimizing** - Measure, don't guess
8. **Fail Fast** - Errors should be loud and clear

---

**Last Updated:** [DATE]  
**Questions?** Update this file via PR and tag @yourusername
