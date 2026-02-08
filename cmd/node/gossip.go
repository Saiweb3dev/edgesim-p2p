package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math/rand"
	"net"
	"strconv"
	"time"

	"github.com/Saiweb3dev/edgesim-p2p/pkg/sensor"
	"github.com/sirupsen/logrus"
)

const gossipMsgType = "GOSSIP"

func gossipBroadcast(ctx context.Context, log *logrus.Entry, cfg config, state *nodeState, reading sensor.Reading) error {
	payload, err := sensor.EncodeReading(reading)
	if err != nil {
		return fmt.Errorf("encode reading: %w", err)
	}

	encoded := base64.StdEncoding.EncodeToString(payload)
	messageID := gossipMessageID(cfg.nodeID, reading.Timestamp)
	state.shouldProcessGossip(messageID)

	targets := selectGossipTargets(state, cfg.coordinatorAddr, cfg.gossipFanout)
	if len(targets) == 0 {
		return nil
	}

	// Message format: GOSSIP|<message_id>|<ttl>|<base64_json_reading>.
	msg := fmt.Sprintf("%s|%s|%d|%s", gossipMsgType, messageID, cfg.gossipTTL, encoded)
	for _, addr := range targets {
		if err := sendGossipFrame(ctx, log, addr, msg); err != nil {
			log.WithError(err).WithField("peer_addr", addr).Warn("gossip send failed")
		}
	}

	return nil
}

func handleGossipMessage(ctx context.Context, log *logrus.Entry, state *nodeState, cfg config, parts []string) {
	if len(parts) < 3 {
		log.Warn("invalid gossip message")
		return
	}

	messageID := parts[0]
	if !state.shouldProcessGossip(messageID) {
		return
	}

	ttl, err := strconv.Atoi(parts[1])
	if err != nil || ttl < 0 {
		log.WithField("message_id", messageID).Warn("invalid gossip ttl")
		return
	}

	payload, err := base64.StdEncoding.DecodeString(parts[2])
	if err != nil {
		log.WithError(err).Warn("gossip payload decode failed")
		return
	}

	reading, err := sensor.DecodeReading(payload)
	if err != nil {
		log.WithError(err).Warn("gossip reading decode failed")
		return
	}

	state.setReading(reading)
	log.WithFields(logrus.Fields{
		"node_id":       reading.NodeID,
		"temperature_c": reading.TemperatureC,
		"ttl":           ttl,
	}).Info("gossip reading received")

	if ttl == 0 {
		return
	}

	targets := selectGossipTargets(state, cfg.coordinatorAddr, cfg.gossipFanout)
	if len(targets) == 0 {
		return
	}

	forward := fmt.Sprintf("%s|%s|%d|%s", gossipMsgType, messageID, ttl-1, parts[2])
	for _, addr := range targets {
		if err := sendGossipFrame(ctx, log, addr, forward); err != nil {
			log.WithError(err).WithField("peer_addr", addr).Warn("gossip forward failed")
		}
	}
}

func selectGossipTargets(state *nodeState, coordinatorAddr string, fanout int) []string {
	peers := state.peerAddrs()
	if coordinatorAddr != "" {
		peers = append(peers, coordinatorAddr)
	}

	filtered := make([]string, 0, len(peers))
	for _, peer := range peers {
		if peer == state.self.addr {
			continue
		}
		filtered = append(filtered, peer)
	}
	if fanout <= 0 || len(filtered) == 0 {
		return nil
	}

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	if fanout >= len(filtered) {
		return filtered
	}

	indices := rng.Perm(len(filtered))
	selected := make([]string, 0, fanout)
	for i := 0; i < fanout; i++ {
		selected = append(selected, filtered[indices[i]])
	}
	return selected
}

func sendGossipFrame(ctx context.Context, log *logrus.Entry, addr string, message string) error {
	dialer := net.Dialer{Timeout: 2 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()

	writer := bufio.NewWriter(conn)
	if err := writeFrame(writer, []byte(message)); err != nil {
		return err
	}
	log.WithField("peer_addr", addr).Debug("gossip sent")
	return nil
}

func gossipMessageID(nodeID string, timestamp time.Time) string {
	payload := fmt.Sprintf("%s-%d", nodeID, timestamp.UnixNano())
	hash := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(hash[:8])
}
