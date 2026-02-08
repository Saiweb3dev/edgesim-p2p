package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type config struct {
	nodeID          string
	listenAddr      string
	metricsAddr     string
	advertiseAddr   string
	peerAddr        string
	bootstrapAddrs  []string
	message         string
	retryInterval   time.Duration
	sensorInterval  time.Duration
	coordinatorAddr string
	gossipFanout    int
	gossipTTL       int
}

func loadConfig() config {
	var cfg config
	flag.StringVar(&cfg.nodeID, "node-id", getenvDefault("NODE_ID", ""), "logical node identifier")
	flag.StringVar(&cfg.listenAddr, "listen", getenvDefault("LISTEN_ADDR", "0.0.0.0:8000"), "TCP listen address")
	flag.StringVar(&cfg.metricsAddr, "metrics", getenvDefault("METRICS_ADDR", "0.0.0.0:9100"), "metrics listen address")
	flag.StringVar(&cfg.advertiseAddr, "advertise", getenvDefault("ADVERTISE_ADDR", ""), "address shared with peers")
	flag.StringVar(&cfg.peerAddr, "peer", getenvDefault("PEER_ADDR", ""), "peer address to connect")
	bootstrap := flag.String("bootstrap", getenvDefault("BOOTSTRAP_ADDRS", ""), "comma-separated bootstrap addrs")
	flag.StringVar(&cfg.message, "message", getenvDefault("MESSAGE", "hello"), "message to send to peer")
	retry := flag.Duration("retry", 2*time.Second, "retry interval for peer connection")
	sensorInterval := flag.Duration("sensor-interval", durationFromEnv("SENSOR_INTERVAL", 5*time.Second), "sensor reading interval")
	flag.StringVar(&cfg.coordinatorAddr, "coordinator", getenvDefault("COORDINATOR_ADDR", ""), "coordinator gossip address")
	flag.IntVar(&cfg.gossipFanout, "gossip-fanout", intFromEnv("GOSSIP_FANOUT", 3), "gossip fanout per tick")
	flag.IntVar(&cfg.gossipTTL, "gossip-ttl", intFromEnv("GOSSIP_TTL", 3), "gossip hop limit")
	flag.Parse()

	cfg.retryInterval = *retry
	cfg.sensorInterval = *sensorInterval
	if cfg.nodeID == "" {
		generated, err := generateNodeID()
		if err != nil {
			fallback := fmt.Sprintf("node-%d", time.Now().UnixNano())
			cfg.nodeID = fallback
		} else {
			cfg.nodeID = generated
		}
	}
	if cfg.advertiseAddr == "" {
		cfg.advertiseAddr = cfg.listenAddr
	}
	if *bootstrap != "" {
		cfg.bootstrapAddrs = splitAndTrim(*bootstrap)
	}
	return cfg
}

func generateNodeID() (string, error) {
	hostname, err := os.Hostname()
	if err != nil || strings.TrimSpace(hostname) == "" {
		hostname = "node"
	}

	// Hash hostname + time + random bytes to avoid collisions across containers.
	var randBytes [16]byte
	if _, err := rand.Read(randBytes[:]); err != nil {
		return "", err
	}

	input := fmt.Sprintf("%s-%d-%s", hostname, time.Now().UnixNano(), hex.EncodeToString(randBytes[:]))
	hash := sha256.Sum256([]byte(input))
	return fmt.Sprintf("%s-%s", hostname, hex.EncodeToString(hash[:])), nil
}

func getenvDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func durationFromEnv(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func intFromEnv(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}
