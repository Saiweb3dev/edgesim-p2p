package main

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"strings"
	"time"

	"github.com/Saiweb3dev/edgesim-p2p/pkg/raft"
	"github.com/Saiweb3dev/edgesim-p2p/pkg/sensor"
	"github.com/sirupsen/logrus"
)

type config struct {
	gossipAddr  string
	httpAddr    string
	metricsAddr string
	raftID      string
	raftAddr    string
	raftPeers   map[string]string
	totalNodes  int
	targetRatio float64
}

func main() {
	cfg := loadConfig()
	logger := logrus.New()
	logger.SetFormatter(&logrus.TextFormatter{FullTimestamp: true})
	log := logger.WithField("component", "coordinator")

	store := newReadingStore(cfg.totalNodes, cfg.targetRatio)
	metrics := newCoordinatorMetrics(cfg.raftID)

	peerIDs := raftPeerIDs(cfg.raftPeers, cfg.raftID)
	transport := newTCPTransport(cfg.raftPeers)
	raftNode, err := raft.NewNode(raft.Config{
		ID:                 cfg.raftID,
		Peers:              peerIDs,
		Transport:          transport,
		Logger:             log,
		ElectionTimeoutMin: 150 * time.Millisecond,
		ElectionTimeoutMax: 300 * time.Millisecond,
		HeartbeatInterval:  50 * time.Millisecond,
		RPCTimeout:         200 * time.Millisecond,
		ApplyFunc: func(entry raft.LogEntry) {
			applyReadingCommand(log, store, entry.Command)
		},
	})
	if err != nil {
		log.WithError(err).Fatal("raft node init failed")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	startMetricsServer(ctx, log, cfg.metricsAddr)
	go updateRaftMetrics(ctx, metrics, raftNode, cfg.raftID)

	go func() {
		if err := runRaftServer(ctx, log, cfg.raftAddr, raftNode); err != nil {
			log.WithError(err).Error("raft server stopped")
			os.Exit(1)
		}
	}()

	go func() {
		if err := raftNode.Run(ctx); err != nil {
			log.WithError(err).Error("raft node stopped")
			os.Exit(1)
		}
	}()

	go func() {
		if err := runGossipServer(ctx, log, cfg.gossipAddr, store, raftNode, cfg.raftPeers); err != nil {
			log.WithError(err).Error("gossip server stopped")
			os.Exit(1)
		}
	}()

	if err := runHTTPServer(log, cfg.httpAddr, store); err != nil {
		log.WithError(err).Error("http server stopped")
		os.Exit(1)
	}
}

func loadConfig() config {
	cfg := config{
		gossipAddr:  getenvDefault("GOSSIP_ADDR", "0.0.0.0:9000"),
		httpAddr:    getenvDefault("HTTP_ADDR", "0.0.0.0:8080"),
		metricsAddr: getenvDefault("METRICS_ADDR", "0.0.0.0:9200"),
		raftID:      getenvDefault("RAFT_ID", "coord-1"),
		raftAddr:    getenvDefault("RAFT_ADDR", "0.0.0.0:7000"),
		raftPeers:   raftPeersFromEnv("RAFT_PEERS"),
		totalNodes:  intFromEnv("TOTAL_NODES", 20),
		targetRatio: floatFromEnv("TARGET_RATIO", 0.9),
	}
	return cfg
}

type readingStore struct {
	mu            sync.Mutex
	readings      map[string]sensor.Reading
	firstSeen     time.Time
	targetReached bool
	targetCount   int
}

func newReadingStore(totalNodes int, targetRatio float64) *readingStore {
	if totalNodes <= 0 {
		totalNodes = 1
	}
	if targetRatio <= 0 || targetRatio > 1 {
		targetRatio = 0.9
	}
	count := int(math.Ceil(float64(totalNodes) * targetRatio))
	return &readingStore{
		readings:    make(map[string]sensor.Reading, totalNodes),
		targetCount: count,
	}
}

func (s *readingStore) upsert(reading sensor.Reading) (int, bool, time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.firstSeen.IsZero() {
		// The first reading starts a propagation window used for timing.
		s.firstSeen = time.Now()
	}

	s.readings[reading.NodeID] = reading

	reached := len(s.readings)
	if !s.targetReached && reached >= s.targetCount {
		s.targetReached = true
		return reached, true, time.Since(s.firstSeen)
	}

	return reached, false, 0
}

func (s *readingStore) snapshot() []sensor.Reading {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := make([]sensor.Reading, 0, len(s.readings))
	for _, reading := range s.readings {
		result = append(result, reading)
	}
	return result
}

func runGossipServer(ctx context.Context, log *logrus.Entry, addr string, store *readingStore, raftNode *raft.Node, raftPeers map[string]string) error {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", addr, err)
	}
	log.WithField("gossip_addr", addr).Info("gossip server listening")

	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			log.WithError(err).Warn("gossip accept failed")
			continue
		}

		go handleGossipConn(ctx, log, conn, store, raftNode, raftPeers)
	}
}

func handleGossipConn(ctx context.Context, log *logrus.Entry, conn net.Conn, store *readingStore, raftNode *raft.Node, raftPeers map[string]string) {
	defer conn.Close()

	reader := bufio.NewReader(conn)
	payload, err := readFrame(reader)
	if err != nil {
		log.WithError(err).Warn("gossip read failed")
		return
	}

	msgType, parts := parseMessage(string(payload))
	if msgType != "GOSSIP" || len(parts) < 3 {
		log.WithField("message", string(payload)).Warn("unknown gossip message")
		return
	}

	encoded := parts[2]
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		log.WithError(err).Warn("gossip payload decode failed")
		return
	}

	reading, err := sensor.DecodeReading(data)
	if err != nil {
		log.WithError(err).Warn("gossip reading decode failed")
		return
	}

	if raftNode == nil {
		storeAndLog(log, store, reading)
		return
	}

	command, err := encodeReadingCommand(reading)
	if err != nil {
		log.WithError(err).Warn("gossip reading encode failed")
		return
	}

	if err := raftNode.Propose(ctx, command); err == nil {
		return
	}

	if errors.Is(err, raft.ErrNotLeader) {
		leaderID := raftNode.LeaderID()
		leaderAddr, ok := raftPeers[leaderID]
		if !ok || leaderAddr == "" {
			log.WithField("leader_id", leaderID).Warn("leader address unknown")
			return
		}
		rpcCtx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
		defer cancel()
		resp, err := sendPropose(rpcCtx, leaderAddr, command)
		if err != nil {
			log.WithError(err).WithField("leader_id", leaderID).Warn("forward to leader failed")
			return
		}
		if !resp.OK {
			log.WithFields(logrus.Fields{
				"leader_id": leaderID,
				"error":     resp.Error,
			}).Warn("leader rejected propose")
		}
		return
	}

	log.WithError(err).Warn("propose failed")
}

func runHTTPServer(log *logrus.Entry, addr string, store *readingStore) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/region", func(w http.ResponseWriter, r *http.Request) {
		center, radiusKm, err := parseRegionQuery(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		readings := store.snapshot()
		avgTemp, count, err := sensor.AverageTemperatureInRadius(readings, center, radiusKm)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		matched := sensor.FilterByRadius(readings, center, radiusKm)
		response := regionResponse{
			Center:   center,
			RadiusKm: radiusKm,
			AvgTempC: avgTemp,
			Samples:  count,
			Readings: matched,
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			log.WithError(err).Warn("region response encode failed")
		}
	})

	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.WithField("http_addr", addr).Info("http server listening")
	return server.ListenAndServe()
}

type regionResponse struct {
	Center   sensor.Location  `json:"center"`
	RadiusKm float64          `json:"radius_km"`
	AvgTempC float64          `json:"avg_temp_c"`
	Samples  int              `json:"samples"`
	Readings []sensor.Reading `json:"readings"`
}

func parseRegionQuery(r *http.Request) (sensor.Location, float64, error) {
	query := r.URL.Query()
	lat, err := parseFloat(query.Get("lat"))
	if err != nil {
		return sensor.Location{}, 0, fmt.Errorf("invalid lat")
	}
	lon, err := parseFloat(query.Get("lon"))
	if err != nil {
		return sensor.Location{}, 0, fmt.Errorf("invalid lon")
	}
	radius, err := parseFloat(query.Get("radius_km"))
	if err != nil || radius <= 0 {
		return sensor.Location{}, 0, fmt.Errorf("invalid radius_km")
	}

	return sensor.Location{Latitude: lat, Longitude: lon}, radius, nil
}

func parseFloat(value string) (float64, error) {
	if value == "" {
		return 0, errors.New("missing value")
	}
	return strconv.ParseFloat(value, 64)
}

// splitAndTrim converts a comma-separated list into cleaned items.
func splitAndTrim(value string) []string {
	items := strings.Split(value, ",")
	clean := make([]string, 0, len(items))
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		clean = append(clean, trimmed)
	}
	return clean
}

func raftPeersFromEnv(key string) map[string]string {
	value := os.Getenv(key)
	if value == "" {
		return map[string]string{}
	}

	entries := splitAndTrim(value)
	peers := make(map[string]string, len(entries))
	for _, entry := range entries {
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) != 2 {
			continue
		}
		id := strings.TrimSpace(parts[0])
		addr := strings.TrimSpace(parts[1])
		if id == "" || addr == "" {
			continue
		}
		peers[id] = addr
	}
	return peers
}

func raftPeerIDs(peers map[string]string, selfID string) []string {
	ids := make([]string, 0, len(peers))
	for id := range peers {
		if id == selfID {
			continue
		}
		ids = append(ids, id)
	}
	return ids
}

func getenvDefault(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func intFromEnv(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func floatFromEnv(key string, fallback float64) float64 {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fallback
	}
	return parsed
}
