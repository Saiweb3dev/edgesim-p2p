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
	"time"

	"github.com/Saiweb3dev/edgesim-p2p/pkg/sensor"
	"github.com/sirupsen/logrus"
)

type config struct {
	gossipAddr  string
	httpAddr    string
	totalNodes  int
	targetRatio float64
}

func main() {
	cfg := loadConfig()
	logger := logrus.New()
	logger.SetFormatter(&logrus.TextFormatter{FullTimestamp: true})
	log := logger.WithField("component", "coordinator")

	store := newReadingStore(cfg.totalNodes, cfg.targetRatio)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := runGossipServer(ctx, log, cfg.gossipAddr, store); err != nil {
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

func runGossipServer(ctx context.Context, log *logrus.Entry, addr string, store *readingStore) error {
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

		go handleGossipConn(log, conn, store)
	}
}

func handleGossipConn(log *logrus.Entry, conn net.Conn, store *readingStore) {
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

	count, reached, duration := store.upsert(reading)
	log.WithFields(logrus.Fields{
		"node_id":       reading.NodeID,
		"temperature_c": reading.TemperatureC,
		"readings":      count,
	}).Info("gossip reading stored")

	if reached {
		log.WithFields(logrus.Fields{
			"target_count": store.targetCount,
			"duration_ms":  duration.Milliseconds(),
		}).Info("gossip target reached")
	}
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
