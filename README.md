EdgeSim is a distributed P2P coordination simulator for edge sensors. It focuses
on Kademlia DHT discovery, gossip propagation, and Raft-style coordination, with
Prometheus and Grafana for observability.

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

## Development

Run tests:

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
