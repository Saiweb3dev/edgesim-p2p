EdgeSim is a distributed P2P coordination simulator for edge sensors. It focuses
on Kademlia DHT discovery, gossip propagation, and Raft-style coordination, with
Prometheus and Grafana for observability.

## Project Context

EdgeSim models a fleet of edge sensors that discover peers via a Kademlia DHT,
propagate readings with gossip, and use a Raft coordinator cluster to keep a
consistent view of aggregated sensor state. The project is designed to be
production-like (metrics, logging, failure handling) while keeping the codebase
approachable for learning distributed systems in Go.

## System Components

- Sensor nodes: emit periodic temperature readings and gossip updates.
- Coordinator nodes (Raft): replicate sensor aggregation state and serve the
  query API.
- Observability: Prometheus scrapes metrics, Grafana visualizes Raft and gossip
  health.

## Run Conditions and Expectations

- Default dev stack simulates 20 sensors and 3 coordinators.
- Raft leader election should converge in under 2 seconds after a leader stop.
- Heartbeats keep followers from triggering elections unless the leader is lost.
- During partitions, the majority side should elect a leader; the isolated node
  must not commit new entries.

## Quickstart

Run the node simulation with monitoring:

```
docker compose -f deployments/compose/docker-compose.dev.yml up --build
```

Open:

- Grafana: http://localhost:3000 (admin/admin)
- Prometheus: http://localhost:9090

Run the DHT simulation:

```
docker compose -f deployments/compose/docker-compose.dhtsim.yml up --build
```

## Chaos and Failover Testing

Start the stack with Toxiproxy enabled:

```
docker compose -f deployments/compose/docker-compose.dev.yml \
	-f deployments/compose/docker-compose.chaos.yml up --build
```

Create the proxies:

```
powershell -ExecutionPolicy Bypass -File scripts/chaos/setup_toxiproxy.ps1
```

Partition and restore examples (see [docs/chaos.md](docs/chaos.md) for full
scenarios):

```
powershell -ExecutionPolicy Bypass -File scripts/chaos/partition_raft.ps1 -Names coord-1
powershell -ExecutionPolicy Bypass -File scripts/chaos/restore_raft.ps1 -Names coord-1
```

## Development

Run tests:

```
make test
```

Or:

```
go test ./...
```

Run DHT benchmarks:

```
go test ./pkg/dht -bench=BenchmarkIterativeFindNode -benchmem
```

## Repository Layout

- cmd/ binaries (node, dhtsim)
- pkg/ core libraries (dht, gossip, raft, sensor)
- deployments/ docker and compose files
- monitoring/ Prometheus and Grafana configuration
- docs/ design documentation
