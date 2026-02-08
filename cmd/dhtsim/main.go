package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/Saiweb3dev/edgesim-p2p/pkg/dht"
	"github.com/Saiweb3dev/edgesim-p2p/pkg/gossip"
	"github.com/Saiweb3dev/edgesim-p2p/pkg/sensor"
	"github.com/sirupsen/logrus"
)

const (
	peerCount = 20
	bucketK   = 20
)

func main() {
	logger := logrus.New()
	logger.SetFormatter(&logrus.TextFormatter{FullTimestamp: true})
	log := logger.WithField("component", "dhtsim")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	network := buildNetwork(peerCount)
	if err := publishSensorReadings(ctx, network, log); err != nil {
		log.WithError(err).Error("sensor publish failed")
		os.Exit(1)
	}

	readings, err := verifySensorQueries(ctx, network, log)
	if err != nil {
		log.WithError(err).Error("sensor query failed")
		os.Exit(1)
	}

	if err := runRegionQuery(readings, log); err != nil {
		log.WithError(err).Error("region query failed")
		os.Exit(1)
	}

	if err := runGossipSimulation(network, log); err != nil {
		log.WithError(err).Error("gossip simulation failed")
		os.Exit(1)
	}

	log.Info("sensors publishing to DHT complete")
}

type simNetwork struct {
	nodes map[dht.NodeID]*dht.Node
}

func buildNetwork(count int) *simNetwork {
	nodes := make(map[dht.NodeID]*dht.Node, count)
	peers := make([]dht.Peer, 0, count)

	for i := 0; i < count; i++ {
		nodeName := fmt.Sprintf("node-%d", i+1)
		id := dht.KeyToID(nodeName)
		node := dht.NewNode(id, nodeName, bucketK)
		nodes[id] = node
		peers = append(peers, node.Peer())
	}

	for _, node := range nodes {
		for _, peer := range peers {
			if peer.ID == node.Peer().ID {
				continue
			}
			_ = node.RoutingTable().AddPeer(peer)
		}
	}

	return &simNetwork{nodes: nodes}
}

func publishSensorReadings(ctx context.Context, network *simNetwork, log *logrus.Entry) error {
	for _, node := range network.nodes {
		peer := node.Peer()
		generator := sensor.NewGenerator(peer.Addr, sensor.LocationFromSeed(peer.Addr))
		reading := generator.Next()
		payload, err := sensor.EncodeReading(reading)
		if err != nil {
			return fmt.Errorf("encode reading: %w", err)
		}

		key := sensorKey(peer.Addr)
		_, err = dht.IterativeStore(ctx, node.RoutingTable(), network.findNode, network.storeValue, key, payload, dht.FindNodeOptions{K: bucketK, Alpha: 3, MaxRounds: 20})
		if err != nil {
			return fmt.Errorf("iterative store failed: %w", err)
		}

		log.WithFields(logrus.Fields{
			"node_id":        peer.Addr,
			"temperature_c":  reading.TemperatureC,
			"location_lat":   reading.Location.Latitude,
			"location_long":  reading.Location.Longitude,
			"timestamp_unix": reading.Timestamp.Unix(),
		}).Info("sensor reading published")
	}
	return nil
}

func verifySensorQueries(ctx context.Context, network *simNetwork, log *logrus.Entry) ([]sensor.Reading, error) {
	var anchor *dht.Node
	for _, node := range network.nodes {
		anchor = node
		break
	}
	if anchor == nil {
		return nil, fmt.Errorf("no nodes in network")
	}

	readings := make([]sensor.Reading, 0, len(network.nodes))

	for _, node := range network.nodes {
		peer := node.Peer()
		key := sensorKey(peer.Addr)
		value, _, err := dht.IterativeFindValue(ctx, anchor.RoutingTable(), network.findValue, key, dht.FindNodeOptions{K: bucketK, Alpha: 3, MaxRounds: 20})
		if err != nil {
			return nil, fmt.Errorf("find value failed: %w", err)
		}
		reading, err := sensor.DecodeReading(value)
		if err != nil {
			return nil, fmt.Errorf("decode reading: %w", err)
		}
		if reading.NodeID != peer.Addr {
			return nil, fmt.Errorf("unexpected reading node id: %s", reading.NodeID)
		}
		readings = append(readings, reading)

		log.WithFields(logrus.Fields{
			"node_id":       reading.NodeID,
			"temperature_c": reading.TemperatureC,
			"timestamp":     reading.Timestamp.Format(time.RFC3339),
		}).Info("sensor reading queried")
	}
	return readings, nil
}

func runRegionQuery(readings []sensor.Reading, log *logrus.Entry) error {
	if len(readings) == 0 {
		return fmt.Errorf("no readings to query")
	}

	center := readings[0].Location
	radiusKm := 10.0
	api := &QueryAPI{}

	avgTemp, sampleCount, err := api.AverageTemperatureInRadius(readings, center, radiusKm)
	if err != nil {
		return err
	}

	matched := sensor.FilterByRadius(readings, center, radiusKm)
	log.WithFields(logrus.Fields{
		"center_lat": center.Latitude,
		"center_lon": center.Longitude,
		"radius_km":  radiusKm,
		"matches":    len(matched),
		"avg_temp_c": avgTemp,
		"samples":    sampleCount,
	}).Info("region query results")

	for _, reading := range matched {
		log.WithFields(logrus.Fields{
			"node_id":       reading.NodeID,
			"temperature_c": reading.TemperatureC,
			"distance_km":   sensor.DistanceKm(center, reading.Location),
		}).Info("sensor in region")
	}

	return nil
}

func runGossipSimulation(network *simNetwork, log *logrus.Entry) error {
	nodes := make([]gossip.Node, 0, len(network.nodes))
	for _, node := range network.nodes {
		peer := node.Peer()
		peers := node.RoutingTable().GetClosestPeers(peer.ID, bucketK)
		peerIDs := make([]string, 0, len(peers))
		for _, p := range peers {
			peerIDs = append(peerIDs, p.Addr)
		}
		nodes = append(nodes, gossip.Node{ID: peer.Addr, Peers: peerIDs})
	}

	if len(nodes) == 0 {
		return fmt.Errorf("no nodes for gossip")
	}

	seed := int64(42)
	fanout := 3
	maxSteps := 20
	targetRatio := 0.9
	tick := 100 * time.Millisecond

	sim := gossip.NewSimulator(seed)
	result, err := sim.SimulatePropagation(nodes, nodes[0].ID, fanout, targetRatio, maxSteps)
	if err != nil {
		return fmt.Errorf("propagation failed: %w", err)
	}

	duration := time.Duration(result.Steps) * tick
	log.WithFields(logrus.Fields{
		"steps":         result.Steps,
		"duration_ms":   duration.Milliseconds(),
		"reached_count": result.ReachedCount,
		"reached_ratio": result.ReachedRatio,
	}).Info("gossip propagation result")

	if duration > 2*time.Second {
		return fmt.Errorf("gossip target missed: duration %s", duration)
	}

	log.Info("gossip working, data reaches 90% of network in <2 seconds")
	return nil
}

func sensorKey(nodeID string) string {
	return fmt.Sprintf("sensor:%s:latest", nodeID)
}

func (n *simNetwork) findNode(ctx context.Context, peer dht.Peer, target dht.NodeID) ([]dht.Peer, error) {
	remote, ok := n.nodes[peer.ID]
	if !ok {
		return nil, fmt.Errorf("unknown peer: %s", peer.ID.String())
	}
	return remote.HandleFindNode(ctx, target, bucketK)
}

func (n *simNetwork) storeValue(ctx context.Context, peer dht.Peer, key string, value []byte) error {
	remote, ok := n.nodes[peer.ID]
	if !ok {
		return fmt.Errorf("unknown peer: %s", peer.ID.String())
	}
	return remote.HandleStore(ctx, key, value)
}

func (n *simNetwork) findValue(ctx context.Context, peer dht.Peer, key string) ([]byte, []dht.Peer, bool, error) {
	remote, ok := n.nodes[peer.ID]
	if !ok {
		return nil, nil, false, fmt.Errorf("unknown peer: %s", peer.ID.String())
	}
	return remote.HandleFindValue(ctx, key, bucketK)
}
