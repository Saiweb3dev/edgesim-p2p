package main

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"os"
	"sort"
	"time"

	"github.com/Saiweb3dev/edgesim-p2p/pkg/dht"
	"github.com/sirupsen/logrus"
)

type peerDistance struct {
	peer     dht.Peer
	distance *big.Int
}

func main() {
	logger := logrus.New()
	logger.SetFormatter(&logrus.TextFormatter{FullTimestamp: true})
	log := logger.WithField("component", "dhtsim")

	localID, err := randomNodeID()
	if err != nil {
		log.WithError(err).Error("failed to generate local id")
		os.Exit(1)
	}

	// Build a 5-node table (local + 4 peers) with randomized IDs.
	rt := dht.NewRoutingTable(localID, 20)
	peers, err := addInitialPeers(rt, 4)
	if err != nil {
		log.WithError(err).Error("failed to add initial peers")
		os.Exit(1)
	}

	log.WithFields(logrus.Fields{
		"local_id": localID.String(),
		"peers":    peerIDs(peers),
	}).Info("initial routing table built")

	// Validate closest-peer ordering by XOR distance before churn.
	if !checkClosestOrdering(log, rt, localID, peers, "initial") {
		os.Exit(1)
	}

	// Simulate churn by removing two peers and adding two new ones.
	peers = churnPeers(rt, peers, log)
	// Validate ordering after churn.
	if !checkClosestOrdering(log, rt, localID, peers, "after-churn") {
		os.Exit(1)
	}

	log.Info("dhtsim completed")
}

func randomNodeID() (dht.NodeID, error) {
	var id dht.NodeID
	_, err := rand.Read(id[:])
	return id, err
}

func addInitialPeers(rt *dht.RoutingTable, count int) ([]dht.Peer, error) {
	peers := make([]dht.Peer, 0, count)
	for i := 0; i < count; i++ {
		id, err := randomNodeID()
		if err != nil {
			return nil, err
		}
		peer := dht.Peer{
			ID:       id,
			Addr:     fmt.Sprintf("node-%d:8000", i+1),
			LastSeen: time.Now(),
		}
		if err := rt.AddPeer(peer); err != nil {
			return nil, err
		}
		peers = append(peers, peer)
	}
	return peers, nil
}

func churnPeers(rt *dht.RoutingTable, peers []dht.Peer, log *logrus.Entry) []dht.Peer {
	if len(peers) < 2 {
		return peers
	}

	log.WithFields(logrus.Fields{
		"removed": []string{peers[0].ID.String(), peers[1].ID.String()},
	}).Info("churn removing peers")
	_ = rt.RemovePeer(peers[0].ID)
	_ = rt.RemovePeer(peers[1].ID)

	peers = peers[2:]

	for i := 0; i < 2; i++ {
		id, err := randomNodeID()
		if err != nil {
			log.WithError(err).Error("failed to generate churn peer")
			continue
		}
		peer := dht.Peer{
			ID:       id,
			Addr:     fmt.Sprintf("churn-%d:8000", i+1),
			LastSeen: time.Now(),
		}
		if err := rt.AddPeer(peer); err != nil {
			log.WithError(err).Error("failed to add churn peer")
			continue
		}
		peers = append(peers, peer)
	}

	log.WithField("peers", peerIDs(peers)).Info("churn added peers")
	return peers
}

func checkClosestOrdering(log *logrus.Entry, rt *dht.RoutingTable, target dht.NodeID, peers []dht.Peer, label string) bool {
	expected := sortByDistance(target, peers)
	closest := rt.GetClosestPeers(target, 3)

	log.WithFields(logrus.Fields{
		"label":          label,
		"expected":       peerIDs(expected),
		"closest_actual": peerIDs(closest),
	}).Info("closest peer check")

	expectedClosest := expected
	if len(expectedClosest) > 3 {
		expectedClosest = expectedClosest[:3]
	}

	if len(closest) != len(expectedClosest) {
		log.WithFields(logrus.Fields{
			"label": label,
			"want":  len(expectedClosest),
			"got":   len(closest),
		}).Error("unexpected closest peer count")
		return false
	}

	for i := 0; i < len(closest); i++ {
		if closest[i].ID != expectedClosest[i].ID {
			log.WithFields(logrus.Fields{
				"label":    label,
				"position": i,
				"want":     expectedClosest[i].ID.String(),
				"got":      closest[i].ID.String(),
			}).Error("closest peer ordering mismatch")
			return false
		}
	}

	return true
}

func sortByDistance(target dht.NodeID, peers []dht.Peer) []dht.Peer {
	items := make([]peerDistance, 0, len(peers))
	for _, peer := range peers {
		items = append(items, peerDistance{peer: peer, distance: xorDistance(target, peer.ID)})
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].distance.Cmp(items[j].distance) < 0
	})

	result := make([]dht.Peer, 0, len(items))
	for _, item := range items {
		result = append(result, item.peer)
	}
	return result
}

func xorDistance(a dht.NodeID, b dht.NodeID) *big.Int {
	buf := make([]byte, len(a))
	for i := 0; i < len(a); i++ {
		buf[i] = a[i] ^ b[i]
	}
	return new(big.Int).SetBytes(buf)
}

func peerIDs(peers []dht.Peer) []string {
	ids := make([]string, 0, len(peers))
	for _, peer := range peers {
		ids = append(ids, peer.ID.String())
	}
	return ids
}
