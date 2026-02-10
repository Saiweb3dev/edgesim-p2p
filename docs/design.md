# EdgeSim: P2P Coordination Layer for Distributed Sensors

## Design Document

**Author:** [Sai Kumar])  
**Status:** 🟡 In Review → 🟢 Approved → 🔵 Implemented  
**Created:** 2026-02-04  
**Last Updated:** 2026-02-10  
**Related Docs:** [Architecture Doc], [API Spec]

---

## Table of Contents

1. [Context and Scope](#1-context-and-scope)
2. [Goals and Non-Goals](#2-goals-and-non-goals)
3. [System Overview](#3-system-overview)
4. [Detailed Design](#4-detailed-design)
5. [Alternatives Considered](#5-alternatives-considered)
6. [Cross-Cutting Concerns](#6-cross-cutting-concerns)
7. [Timeline and Milestones](#7-timeline-and-milestones)
8. [Open Questions](#8-open-questions)
9. [References](#9-references)

---

## 1. Context and Scope

### 1.1 Background

Modern edge computing requires coordination of distributed physical devices (sensors, IoT nodes, edge compute) without relying on centralized infrastructure. Current solutions either:

1. **Centralized coordination** (AWS IoT, Azure IoT Hub): Single point of failure, vendor lock-in, latency to cloud
2. **Manual configuration**: Doesn't scale, brittle, no fault tolerance
3. **Ad-hoc P2P**: Inconsistent, no proven reliability

**Problem Statement:** How do we coordinate 50+ distributed sensors for real-time data aggregation without centralized authority?

### 1.2 Scope

**In Scope:**

- Peer discovery mechanism (Kademlia DHT)
- Fault-tolerant coordinator election (Raft consensus)
- Data propagation protocol (Gossip)
- Real-time observability (Prometheus + Grafana)
- Simulation framework (Docker-based)

**Out of Scope (V1):**

- Security/encryption (future: TLS, mutual auth)
- Persistent storage (future: integrate with TimescaleDB)
- Production deployment (simulation only)
- Hardware integration (simulated sensors only)
- Geographic routing optimizations (future: latency-aware routing)

### 1.3 Assumptions

1. Network is **partially reliable** (TCP available, but connections can drop)
2. Nodes can **crash and recover** (no Byzantine faults in V1)
3. Clock skew is **bounded** (NTP available, max 1s skew)
4. **50-100 nodes** is target scale for V1 (1000+ in future)
5. Sensor data is **non-critical** (eventual consistency acceptable)

### 1.4 Success Criteria

**Functional:**

- ✅ 50 nodes can discover each other in <5 seconds
- ✅ Coordinator failover completes in <2 seconds
- ✅ Data propagates to 90% of network in <3 seconds
- ✅ System tolerates 30% node churn

**Non-Functional:**

- ✅ DHT lookup: O(log N) hops, <50ms p99 latency
- ✅ Query throughput: 1000 qps
- ✅ Memory usage: <10MB per node
- ✅ CPU usage: <5% per node on modern laptop

---

## 2. Goals and Non-Goals

### 2.1 Goals

**Primary Goals:**

1. **Demonstrate distributed systems fundamentals** through working implementation
2. **Build production-grade system** with observability, testing, documentation
3. **Learn P2P, consensus, gossip protocols** via hands-on coding
4. **Create portfolio project** suitable for Big Tech interviews

**Secondary Goals:**

1. Understand trade-offs in CAP theorem (choosing AP over C)
2. Practice chaos engineering (Toxiproxy-based failure injection)
3. Build interview narrative around system design decisions

### 2.2 Non-Goals

**Explicitly NOT doing in V1:**

- ❌ Byzantine fault tolerance (assumes honest nodes)
- ❌ Security hardening (no auth, encryption, rate limiting)
- ❌ Multi-datacenter deployment (single simulated network)
- ❌ Optimized for WAN latencies (LAN simulation only)
- ❌ Production-ready code (educational quality, not Google SRE quality)
- ❌ Smart contract integration (not blockchain-focused)

### 2.3 Success Metrics

| Metric                | Target                  | Measurement Method   |
| --------------------- | ----------------------- | -------------------- |
| DHT Lookup Latency    | p99 < 50ms              | Prometheus histogram |
| Gossip Propagation    | 90% in <3s              | Timestamp analysis   |
| Raft Failover Time    | <2s                     | Grafana dashboard    |
| Test Coverage         | >70%                    | go test -cover       |
| Node Resource Usage   | <10MB RAM               | Docker stats         |
| Documentation Quality | All packages documented | godoc check          |

---

## 3. System Overview

### 3.0 V0.1 TCP Communication Demo

This iteration introduces a minimal, testable TCP path so two sensor nodes can exchange a single message over Docker networking before the full DHT/gossip stack lands.

```
┌────────────────────────── Docker Compose (dev) ──────────────────────────┐
│                                                                          │
│   ┌──────────────┐             TCP             ┌──────────────┐          │
│   │  node-1      │  ───────────────────────▶   │   node-2     │          │
│   │ listen :8001 │  ◀───────────────────────   │ listen :8002 │          │
│   │ peer :8002   │            ACK              │ peer :8001   │          │
│   └──────────────┘                              └──────────────┘          │
│                                                                          │
│  Images built from deployments/docker/Dockerfile.node                    │
│  Compose file: deployments/compose/docker-compose.dev.yml                │
└──────────────────────────────────────────────────────────────────────────┘
```

Node behavior (cmd/node/main.go):

- Starts a TCP server on LISTEN_ADDR.
- If PEER_ADDR is set, connects and sends one newline-delimited message.
- Logs received messages and replies with a simple ACK.

### 3.1 High-Level Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    EdgeSim Network                          │
│                                                             │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐                 │
│  │ Sensor 1 │  │ Sensor 2 │  │ Sensor N │  (50 nodes)     │
│  │          │  │          │  │          │                  │
│  │ - DHT    │  │ - DHT    │  │ - DHT    │                 │
│  │ - Gossip │  │ - Gossip │  │ - Gossip │                 │
│  │ - Data   │  │ - Data   │  │ - Data   │                 │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘                 │
│       │             │             │                         │
│       └─────────────┴─────────────┘                        │
│                     │                                       │
│              P2P Network Layer                             │
│         (Kademlia DHT + Gossip)                            │
│                     │                                       │
│       ┌─────────────┴─────────────┐                        │
│       │                           │                         │
│  ┌────▼─────┐  ┌──────────┐  ┌──────────┐                 │
│  │Coord 1   │  │Coord 2   │  │Coord 3   │  (Raft Cluster) │
│  │(Leader)  │◄─┤(Follower)│◄─┤(Follower)│                 │
│  └────┬─────┘  └──────────┘  └──────────┘                 │
│       │                                                     │
│       │ gRPC                                                │
│       │                                                     │
│  ┌────▼─────┐                                              │
│  │  Query   │                                              │
│  │   API    │                                              │
│  └──────────┘                                              │
│                                                             │
└─────────────────────────────────────────────────────────────┘
         │                           │
         │                           │
    ┌────▼────┐              ┌──────▼──────┐
    │Prometheus│              │   Grafana   │
    │ Metrics  │              │  Dashboard  │
    └──────────┘              └─────────────┘
```

### 3.2 Component Descriptions

**Sensor Nodes (50+):**

- **Responsibilities:**
  - Generate simulated temperature data every 5 seconds
  - Participate in Kademlia DHT for peer discovery
  - Propagate sensor readings via gossip protocol
  - Respond to DHT queries (FIND_NODE, FIND_VALUE)
- **Technology:** Go processes in Docker containers
- **Communication:** TCP for DHT, UDP for gossip (future)
- **State:** In-memory routing table + recent sensor readings

**Coordinator Cluster (3 nodes):**

- **Responsibilities:**
  - Maintain cluster-wide state via Raft consensus
  - Aggregate sensor data for queries
  - Provide gRPC API for external queries
  - Handle leader election on coordinator failures
- **Technology:** Go with hashicorp/raft library
- **Communication:** gRPC between coordinators
- **State:** Raft log (replicated) + aggregated sensor data

**Observability Stack:**

- **Prometheus:** Scrapes metrics from all nodes (DHT latency, gossip stats, Raft state)
- **Grafana:** Visualizes network topology, real-time metrics, alerts

### 3.3 Data Flow

**Scenario 1: Sensor Data Publication**

```
1. Sensor Node generates reading (temp=23.5°C)
2. Gossip to 3 random peers (fanout=3)
3. Peers propagate to their peers (epidemic broadcast)
4. Coordinator receives via gossip, aggregates
5. Query API exposes aggregated data
```

**Scenario 2: Peer Discovery**

```
1. New node joins with bootstrap peers
2. FIND_NODE RPC to bootstrap (find nodes close to self)
3. Iterative lookup: query α closest nodes in parallel
4. Repeat until k closest peers found
5. Add peers to routing table, start sending heartbeats
```

**Scenario 3: Coordinator Failover**

```
1. Leader coordinator crashes
2. Followers detect missing heartbeats (timeout=150ms)
3. Follower becomes candidate, requests votes
4. Majority votes received → new leader elected
5. New leader resumes heartbeats, cluster continues
```

---

## 4. Detailed Design

### 4.1 Kademlia DHT

**Overview:**  
Distributed hash table for peer discovery with O(log N) lookup complexity. Uses XOR distance metric for routing.

**Data Structures:**

```go
// NodeID is a 160-bit identifier (SHA-256 hash truncated to 160 bits)
type NodeID [20]byte

// Peer represents a network node
type Peer struct {
    ID      NodeID
    Address string  // "ip:port"
    LastSeen time.Time
}

// RoutingTable stores peers in k-buckets
type RoutingTable struct {
    localID  NodeID
    buckets  [160]*Bucket  // One bucket per bit
    k        int           // Bucket size (typically 20)
}

// Bucket stores up to k peers at given distance
type Bucket struct {
    peers []*Peer
    mu    sync.RWMutex
}
```

**Key Operations:**

1. **XOR Distance:**

   ```
   distance(A, B) = A ⊕ B

   Properties:
   - d(A, B) = d(B, A)  (symmetric)
   - d(A, A) = 0
   - d(A, B) > 0 if A ≠ B
   ```

2. **Node Lookup (FIND_NODE):**

   ```
   Input: targetID, k (number of closest peers desired)
   Output: k peers closest to targetID

   Algorithm:
   1. Initialize shortlist with α closest known peers
   2. Query α peers in parallel for their k closest
   3. Add responses to shortlist, sort by distance
   4. Repeat step 2 with unqueried close peers
   5. Terminate when:
      - k closest peers all queried, OR
      - No closer peers found in round, OR
      - Max iterations (log N) reached
   ```

3. **Bucket Maintenance:**
   ```
   On receiving message from peer P:
   1. Find appropriate bucket for P (by XOR distance)
   2. If P already in bucket → move to tail (LRU)
   3. If bucket not full → add P to tail
      4. If bucket full → evict head (oldest peer) for now
         - Future: ping head before eviction to preserve live peers
   ```

**Configuration:**

- `k = 20` (peers per bucket)
- `α = 3` (parallelism during lookup)
- Bucket refresh: Every 1 hour
- Stale peer timeout: 5 minutes

**Trade-offs:**

- **Pro:** O(log N) lookup, self-healing, proven (BitTorrent, Ethereum)
- **Con:** Vulnerable to eclipse attacks (mitigated by peer diversity)

---

### 4.2 Raft Consensus

**Overview:**  
Leader-based consensus for coordinator cluster. Ensures all coordinators agree on cluster state despite failures.

**Roles:**

1. **Leader:**
   - Handles all client requests
   - Sends heartbeats to followers
   - Replicates log to followers
2. **Follower:**
   - Responds to RPCs from leader/candidates
   - Redirects client requests to leader
   - Becomes candidate if no heartbeat
3. **Candidate:**
   - Requests votes from other nodes
   - Becomes leader if majority votes
   - Reverts to follower if election fails

**Coordinator Architecture (V1):**

- **Coordinator Node:** Runs the Raft election state machine and exposes leader status to the query API.
- **Transport Layer:** Abstracted RPC interface for `RequestVote` and `AppendEntries` heartbeats.
- **Timing Loop:** Randomized election timeout per node, plus periodic leader heartbeats.
- **State Store:** In-memory (term, vote, role, leader ID). Log replication is deferred to the next milestone.

**Election State Machine (V1):**

```go
type RaftState struct {
   // Persistent state (survive crashes)
   currentTerm uint64
   votedFor    string

   // Volatile state (all nodes)
   role     Role   // follower | candidate | leader
   leaderID string
}
```

**Key Operations:**

1. **Leader Election (V1 focus):**

   ```
   Follower timeout (no heartbeat) →
   Convert to candidate →
   Increment term, vote for self →
   Request votes from others →

   If majority votes received:
       → Become leader
       → Send heartbeats
   Else if higher term seen:
       → Revert to follower
   Else if timeout:
       → Start new election
   ```

      Timeout reset rules:
      - Reset on valid heartbeat (`AppendEntries`) from a leader in current or higher term.
      - Reset on granting a vote to a candidate.
      - Randomize each timeout to reduce split votes.

2. **Log Replication:**

   ```
   Leader receives client request →
   Append to local log →
   Send AppendEntries RPC to followers →

   If majority ack:
       → Commit entry
       → Apply to state machine
       → Return success to client
   Else:
       → Retry indefinitely
   ```

3. **Safety Guarantee:**
   ```
   Election Restriction:
   - Candidate must have all committed entries
   - Voters check: candidate's log ≥ own log
   - Ensures new leader has complete history
   ```

**Configuration:**

- Election timeout: 150-300ms (randomized per node)
- Heartbeat interval: 50ms
- Max entries per AppendEntries: 100
- Snapshot threshold: 10,000 entries (future)

**Trade-offs:**

- **Pro:** Understandable, strong consistency, proven
- **Con:** Leader bottleneck, requires majority (not partition-tolerant)

---

### 4.3 Gossip Protocol

**Overview:**  
Epidemic-style broadcast for sensor data propagation. Trades bandwidth for reliability.

**Algorithm:**

```
On receiving sensor reading R:
1. If R already seen (check message ID) → discard
2. Store R locally
3. Select fanout (f=3) random peers
4. Send R to each peer
5. Decrement TTL
6. If TTL > 0 → peers repeat steps 3-5

Parameters:
- Fanout (f): 3 peers per round
- TTL: 5 hops
- Gossip period: 1 second
```

**Message Format:**

```go
type GossipMessage struct {
    ID        string       // UUID for deduplication
    Type      string       // "SENSOR_READING"
    Payload   []byte       // Serialized data
    TTL       int          // Remaining hops
    Origin    NodeID       // Original sender
    Timestamp time.Time
}
```

**Propagation Analysis:**

```
Network size: N = 50
Fanout: f = 3
Expected coverage per round:
- Round 1: 3 nodes (f)
- Round 2: 9 nodes (f²)
- Round 3: 27 nodes (f³)
- Round 4: 81 nodes (> N)

Expected rounds to 90% coverage: 4 rounds ≈ 4 seconds
```

**Optimization (Future):**

- Hybrid push-pull (pull missing data)
- Topology-aware gossip (prefer low-latency peers)
- Bloom filter for efficient deduplication

**Trade-offs:**

- **Pro:** Fault-tolerant, no single point of failure, simple
- **Con:** Redundant messages (15-30% duplicates), bandwidth usage

---

### 4.4 API Design

**gRPC Service Definition:**

```protobuf
syntax = "proto3";

package edgesim.v1;

service SensorService {
  // GetReading returns latest reading from a specific sensor
  rpc GetReading(GetReadingRequest) returns (SensorReading);

  // GetRegionalData returns aggregated data for a geographic region
  rpc GetRegionalData(RegionalQuery) returns (AggregatedData);

  // StreamReadings streams real-time sensor updates
  rpc StreamReadings(StreamRequest) returns (stream SensorReading);
}

message SensorReading {
  string node_id = 1;
  double temperature = 2;
  Location location = 3;
  int64 timestamp_unix = 4;
}

message RegionalQuery {
  Location center = 1;
  double radius_km = 2;
}

message AggregatedData {
  double avg_temperature = 1;
  double min_temperature = 2;
  double max_temperature = 3;
  int32 sensor_count = 4;
}
```

**REST Fallback (Optional):**

```
GET  /api/v1/sensors/:id              - Get specific sensor
GET  /api/v1/sensors?region=...       - Query by region
POST /api/v1/query                    - Complex queries
WS   /api/v1/stream                   - WebSocket stream
```

---

### 4.5 Observability

**Prometheus Metrics:**

```go
// DHT Metrics
dht_lookup_latency_milliseconds      // Histogram
dht_routing_table_size               // Gauge
dht_rpc_requests_total               // Counter (by type)
dht_rpc_errors_total                 // Counter (by type)

// Raft Metrics
raft_leader_election_duration_ms     // Histogram
raft_is_leader                       // Gauge (0 or 1)
raft_commit_index                    // Gauge
raft_log_entries_total               // Counter
raft_heartbeat_latency_ms            // Histogram

// Gossip Metrics
gossip_messages_sent_total           // Counter
gossip_messages_received_total       // Counter
gossip_duplicate_rate                // Gauge (0-1)
gossip_propagation_latency_ms        // Histogram

// System Metrics
node_uptime_seconds                  // Gauge
node_cpu_usage_percent               // Gauge
node_memory_usage_bytes              // Gauge
```

**Grafana Dashboards:**

1. **Network Overview:**
   - Active nodes count
   - Network topology graph (D3.js)
   - Message flow visualization

2. **DHT Performance:**
   - Lookup latency (p50, p99)
   - Routing table sizes
   - RPC success rate

3. **Raft Health:**
   - Current leader indicator
   - Election frequency
   - Log replication lag

4. **Gossip Analysis:**
   - Propagation time distribution
   - Duplicate rate trend
   - Coverage percentage

---

## 5. Alternatives Considered

### 5.1 DHT: Chord vs Kademlia

**Option A: Chord**

- **Pro:** Simpler algorithm, deterministic routing
- **Pro:** Better load balancing (consistent hashing)
- **Con:** Requires finger table maintenance (O(log N) state)
- **Con:** Less proven in modern systems

**Option B: Kademlia (CHOSEN)**

- **Pro:** XOR metric ensures symmetry, efficient routing
- **Pro:** Widely used (BitTorrent, Ethereum, IPFS)
- **Pro:** Self-healing (bucket refresh mechanism)
- **Con:** Slightly more complex than Chord

**Decision:** Kademlia chosen for proven track record and industry relevance.

---

### 5.2 Consensus: Paxos vs Raft vs PBFT

**Option A: Paxos**

- **Pro:** Theoretically elegant, proven correct
- **Con:** Notoriously difficult to understand/implement
- **Con:** Many implementation variants (Multi-Paxos, Fast Paxos)

**Option B: Raft (CHOSEN)**

- **Pro:** Designed for understandability
- **Pro:** Strong open-source implementations (etcd, hashicorp/raft)
- **Pro:** Extensive documentation (paper, students' guide, visualizations)
- **Con:** Leader bottleneck under high load

**Option C: PBFT (Byzantine Fault Tolerant)**

- **Pro:** Handles malicious nodes
- **Con:** 3f+1 nodes required for f faults (expensive)
- **Con:** Overkill for trusted sensor network

**Decision:** Raft chosen for educational value and sufficient for non-Byzantine faults.

---

### 5.3 Data Propagation: Gossip vs Pub/Sub vs Flooding

**Option A: Flooding**

- **Pro:** Simplest implementation
- **Con:** Network storms, doesn't scale

**Option B: Pub/Sub (Redis, Kafka)**

- **Pro:** Reliable delivery, ordering guarantees
- **Con:** Requires centralized broker (defeats P2P goal)

**Option C: Gossip (CHOSEN)**

- **Pro:** Decentralized, fault-tolerant
- **Pro:** Scales well (probabilistic guarantees)
- **Con:** Redundant messages (trade bandwidth for reliability)

**Decision:** Gossip aligns with P2P architecture and provides good balance.

---

### 5.4 Language: Go vs Rust vs Python

**Option A: Python**

- **Pro:** Rapid prototyping, easy debugging
- **Con:** GIL limits concurrency, slower performance
- **Con:** Not industry standard for distributed systems

**Option B: Rust**

- **Pro:** Memory safety, zero-cost abstractions
- **Pro:** Growing adoption in blockchain space
- **Con:** Steeper learning curve, slower development

**Option C: Go (CHOSEN)**

- **Pro:** Excellent concurrency (goroutines, channels)
- **Pro:** Industry standard (Docker, Kubernetes, etcd)
- **Pro:** Fast compilation, good tooling
- **Con:** Less type safety than Rust

**Decision:** Go chosen for productivity and industry relevance to Big Tech.

---

## 6. Cross-Cutting Concerns

### 6.1 Security (Future)

**Authentication:**

- mTLS between nodes (future V2)
- API key for query API (future V2)

**Authorization:**

- Role-based access (sensor vs coordinator)

**Known Vulnerabilities (Accepted in V1):**

- ❌ No encryption (data sent in plaintext)
- ❌ No DDoS protection (resource exhaustion possible)
- ❌ Eclipse attacks possible (malicious DHT routing)

**Mitigation Plan (V2):**

- Add TLS for all communications
- Rate limiting on RPCs
- Peer reputation system

---

### 6.2 Scalability

**Current Limits (V1):**

- 50 nodes: Works smoothly on laptop
- 100 nodes: Approaches memory limits
- 1000+ nodes: Requires optimization

**Bottlenecks:**

- Coordinator: Single leader handles all queries
- Gossip: Message duplication increases with network size
- DHT: Routing table size grows O(log N)

**Scaling Plan (V2+):**

- Sharded coordinators (consistent hashing)
- Hybrid push-pull gossip
- Geographic clustering

---

### 6.3 Reliability

**Failure Modes Handled:**

- ✅ Individual sensor crashes (gossip continues)
- ✅ Coordinator crash (Raft failover)
- ✅ Network partition (eventual consistency)
- ✅ Slow nodes (timeout mechanisms)

**Failure Modes NOT Handled (V1):**

- ❌ Cascading failures (no circuit breakers)
- ❌ Permanent data loss (no persistence)
- ❌ Malicious nodes (Byzantine faults)

**Testing Strategy:**

- Chaos engineering with Toxiproxy
- Random node kills during integration tests
- Network partition scenarios

---

### 6.4 Monitoring & Alerting

**Key Alerts (Future):**

- `DHT lookup p99 > 100ms` → Performance degradation
- `Raft leader elections > 5/hour` → Cluster instability
- `Gossip duplicate rate > 40%` → Network congestion
- `Active nodes < 40` → Mass failure event

**Logging Strategy:**

- Structured logs (JSON) with correlation IDs
- Log levels: DEBUG, INFO, WARN, ERROR
- Centralized logging (future: ELK stack)

---

### 6.5 Testing Strategy

**Unit Tests:**

- All packages >70% coverage
- Table-driven tests for edge cases
- Mock DHT/Raft for isolated testing

**Integration Tests:**

- Docker Compose with 10 nodes
- End-to-end scenarios (join, leave, query)
- Performance benchmarks

**Chaos Tests:**

- Toxiproxy: latency, packet loss, partitions
- Random node kills (30% churn)
- Split-brain scenarios

**Load Tests (Future):**

- 1000 qps query load
- Measure p99 latency under load
- Memory leak detection

---

## 7. Timeline and Milestones

### 7.1 Development Phases

**Week 1: Foundation** (Feb 4-10)

- ✅ Project setup (repo, Docker, CI)
- ✅ Basic TCP communication (5 nodes)
- ✅ Design doc v1.0 complete
- **Deliverable:** 5 nodes can exchange messages

**Week 2: Kademlia DHT** (Feb 11-17)

- ✅ XOR distance metric
- ✅ k-bucket implementation
- ✅ FIND_NODE RPC
- ✅ Scale to 20 nodes
- **Deliverable:** DHT with verified O(log N) lookups

**Week 3: Sensor Simulation** (Feb 18-24)

- ✅ Sensor data model
- ✅ Gossip protocol implementation
- ✅ Data propagation metrics
- **Deliverable:** 20 sensors gossiping data

**Week 4: Raft Consensus** (Feb 25-Mar 3)

- ✅ Leader election
- ✅ Log replication
- ✅ Coordinator integration
- **Deliverable:** Fault-tolerant coordinator cluster

**Week 5: Production Polish** (Mar 4-10)

- ✅ Scale to 50 nodes
- ✅ Chaos testing suite
- ✅ Performance optimization
- **Deliverable:** Production-ready system

**Week 6: Documentation & Demo** (Mar 11-17)

- ✅ Comprehensive README
- ✅ Architecture diagrams
- ✅ Demo video
- ✅ Blog post
- **Deliverable:** v1.0.0 release

### 7.2 Key Milestones

| Date   | Milestone           | Acceptance Criteria           |
| ------ | ------------------- | ----------------------------- |
| Feb 10 | Week 1 Complete     | 5 nodes communicating via TCP |
| Feb 17 | DHT Working         | 20 nodes, O(log N) lookups    |
| Feb 24 | Gossip Working      | Data reaches 90% in <3s       |
| Mar 3  | Raft Working        | Coordinator failover <2s      |
| Mar 10 | V1 Feature Complete | 50 nodes, all metrics green   |
| Mar 17 | v1.0.0 Shipped      | Public release + blog post    |

### 7.3 Risk Mitigation

| Risk                   | Impact | Probability | Mitigation                           |
| ---------------------- | ------ | ----------- | ------------------------------------ |
| Raft too complex       | High   | Medium      | Use hashicorp/raft library if needed |
| Docker resource limits | Medium | Low         | Optimize container sizes             |
| Gossip message storms  | Medium | Medium      | Add rate limiting early              |
| Scope creep            | High   | High        | Strict adherence to non-goals        |

---

## 8. Open Questions

### 8.1 Technical Decisions

- [ ] **Q1:** Should we implement Raft from scratch or use etcd/raft library?
  - **Current thinking:** Implement from scratch for learning, but timebox to 1 week
  - **Decision by:** Feb 25
  - **Decision maker:** Self (learning project)

- [ ] **Q2:** UDP vs TCP for gossip protocol?
  - **Current thinking:** TCP in V1 (easier), UDP in V2 (performance)
  - **Decision by:** Feb 18
  - **Blocked on:** Nothing, proceed with TCP

- [ ] **Q3:** Geographic distribution strategy for simulated sensors?
  - **Current thinking:** Random lat/long in rectangular region
  - **Alternative:** Cluster in cities (more realistic)
  - **Decision by:** Feb 18

### 8.2 Product Questions

- [ ] **Q4:** Should query API support SQL-like queries or just simple filters?
  - **Current thinking:** Simple filters (region, time range) in V1
  - **Decision by:** Mar 4

- [ ] **Q5:** Real-time streaming vs polling for sensor data?
  - **Current thinking:** Support both (gRPC stream + REST polling)
  - **Decision by:** Mar 11

### 8.3 Infrastructure Questions

- [ ] **Q6:** Deploy to cloud for demo or keep local?
  - **Current thinking:** Local Docker is sufficient for V1
  - **Alternative:** Deploy to AWS Free Tier for realistic networking
  - **Decision by:** Mar 10

---

## 9. References

### 9.1 Academic Papers

1. **Kademlia: A Peer-to-Peer Information System Based on the XOR Metric**  
   Maymounkov & Mazières, 2002  
   https://pdos.csail.mit.edu/~petar/papers/maymounkov-kademlia-lncs.pdf

2. **In Search of an Understandable Consensus Algorithm (Raft)**  
   Ongaro & Ousterhout, 2014  
   https://raft.github.io/raft.pdf

3. **Epidemic Algorithms for Replicated Database Maintenance**  
   Demers et al., 1987  
   https://dl.acm.org/doi/10.1145/41840.41841

### 9.2 Industry Resources

4. **Google SRE Book - Chapter 23: Managing Critical State**  
   https://sre.google/sre-book/managing-critical-state/

5. **etcd Architecture Documentation**  
   https://etcd.io/docs/latest/learning/

6. **Students' Guide to Raft**  
   https://thesecretlivesofdata.com/raft/

### 9.3 Open Source Projects

7. **libp2p (Protocol Labs)**  
   https://github.com/libp2p/go-libp2p

8. **hashicorp/raft (Raft implementation)**  
   https://github.com/hashicorp/raft

9. **Ethereum's devp2p (DHT implementation)**  
   https://github.com/ethereum/devp2p

### 9.4 Internal Docs

10. **EdgeSim Architecture Diagrams**  
    [Link to draw.io diagrams]

11. **API Specification**  
    [Link to OpenAPI/Protobuf specs]

---

## Appendix A: Glossary

| Term           | Definition                                                   |
| -------------- | ------------------------------------------------------------ |
| **DHT**        | Distributed Hash Table - decentralized key-value store       |
| **k-bucket**   | Routing table bucket holding up to k peers at given distance |
| **XOR metric** | Distance function: d(A,B) = A ⊕ B (bitwise XOR)              |
| **Gossip**     | Epidemic-style broadcast protocol                            |
| **Raft**       | Consensus algorithm ensuring replicated state machines       |
| **Quorum**     | Majority of nodes (n/2 + 1) required for decisions           |
| **Fanout**     | Number of peers to forward message to in gossip              |
| **TTL**        | Time-to-live - maximum hops for gossip message               |

---

## Appendix B: Change Log

| Date       | Version | Changes                             | Author      |
| ---------- | ------- | ----------------------------------- | ----------- |
| 2026-02-04 | 0.1     | Initial draft                       | [Your Name] |
| 2026-02-10 | 0.2     | Updated after Week 1 implementation | [Your Name] |
| 2026-02-17 | 0.3     | DHT design finalized                | [Your Name] |
| 2026-03-17 | 1.0     | Final version for v1.0.0 release    | [Your Name] |

---

## Appendix C: Review Comments

**[Tech Lead] - 2026-02-05:**

> Looks good overall. Consider adding metrics for gossip duplicate rate - important for tuning fanout parameter.

**Response:** Added gossip_duplicate_rate gauge metric. Will track in Week 3.

**[Senior Engineer] - 2026-02-06:**

> Question: Why not use Consul for service discovery instead of implementing Kademlia?

**Response:** This is a learning project - implementing fundamentals is the goal. Consul would be production choice, but defeats educational purpose.

---

**Document Status:** 🟡 In Review  
**Next Review:** Feb 10, 2026  
**Approval Required:** Tech Lead sign-off

---

## How to Use This Document

**For Code Reviews:**

- Reference section numbers when discussing design decisions
- Link to specific alternatives considered
- Update "Open Questions" as decisions are made

**For Implementation:**

- Treat this as source of truth for architecture
- Update when significant changes occur
- Keep in sync with actual code

**For Interviews:**

- Use as narrative structure
- Reference specific trade-offs made
- Demonstrate systematic thinking

---

**Last Updated:** 2026-02-04  
**Maintained by:** [Sai]  
**Questions?** Create GitHub issue tagged `design-doc`
