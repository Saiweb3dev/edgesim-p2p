package main

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

func bootstrapDiscovery(ctx context.Context, log *logrus.Entry, cfg config, state *nodeState, metrics *nodeMetrics) error {
	if len(cfg.bootstrapAddrs) == 0 && cfg.peerAddr == "" {
		return nil
	}

	if cfg.peerAddr != "" {
		cfg.bootstrapAddrs = append(cfg.bootstrapAddrs, cfg.peerAddr)
	}

	var peers []string
	for _, addr := range cfg.bootstrapAddrs {
		if addr == "" {
			continue
		}
		start := time.Now()
		bootstrapPeers, err := helloAndGetPeers(ctx, log, cfg, addr)
		metrics.ObserveLookupLatency(cfg.nodeID, time.Since(start))
		if err != nil {
			log.WithError(err).WithField("bootstrap", addr).Warn("bootstrap failed")
			continue
		}
		peers = append(peers, bootstrapPeers...)
	}

	for _, addr := range peers {
		if addr == "" || addr == cfg.advertiseAddr {
			continue
		}
		state.upsertPeer(peerInfo{nodeID: "", addr: addr, lastSeen: time.Now()})
	}

	for _, peer := range state.peerAddrs() {
		if peer == cfg.advertiseAddr {
			continue
		}
		if err := sendMessageToPeer(log, cfg, peer); err != nil {
			log.WithError(err).WithField("peer_addr", peer).Warn("send failed")
		}
	}

	return nil
}

func helloAndGetPeers(ctx context.Context, log *logrus.Entry, cfg config, addr string) ([]string, error) {
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		return nil, fmt.Errorf("dial bootstrap %s: %w", addr, err)
	}
	defer conn.Close()

	writer := bufio.NewWriter(conn)
	reader := bufio.NewReader(conn)

	hello := fmt.Sprintf("HELLO|%s|%s", cfg.nodeID, cfg.advertiseAddr)
	if err := writeFrame(writer, []byte(hello)); err != nil {
		return nil, fmt.Errorf("send hello: %w", err)
	}
	if _, err := readFrame(reader); err != nil {
		log.WithError(err).WithField("bootstrap", addr).Warn("hello ack missing")
	}

	if err := writeFrame(writer, []byte("GETPEERS")); err != nil {
		return nil, fmt.Errorf("send getpeers: %w", err)
	}
	response, err := readFrame(reader)
	if err != nil {
		return nil, fmt.Errorf("read peers: %w", err)
	}

	msgType, parts := parseMessage(strings.TrimSpace(string(response)))
	if msgType != "PEERS" || len(parts) == 0 {
		return nil, fmt.Errorf("unexpected peers response: %s", string(response))
	}
	peers := splitAndTrim(parts[0])

	log.WithFields(logrus.Fields{
		"bootstrap": addr,
		"peers":     peers,
	}).Info("bootstrap peers received")

	return peers, nil
}

func sendMessageToPeer(log *logrus.Entry, cfg config, addr string) error {
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()

	return sendMessage(conn, log, cfg)
}
