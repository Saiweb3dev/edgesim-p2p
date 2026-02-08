package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Saiweb3dev/edgesim-p2p/pkg/sensor"
	"github.com/sirupsen/logrus"
)

func main() {
	cfg := loadConfig()
	logger := logrus.New()
	logger.SetFormatter(&logrus.TextFormatter{FullTimestamp: true})

	log := logger.WithFields(logrus.Fields{
		"node_id":        cfg.nodeID,
		"listen_addr":    cfg.listenAddr,
		"metrics_addr":   cfg.metricsAddr,
		"advertise_addr": cfg.advertiseAddr,
		"peer_addr":      cfg.peerAddr,
	})

	state := &nodeState{
		self: peerInfo{
			nodeID:   cfg.nodeID,
			addr:     cfg.advertiseAddr,
			lastSeen: time.Now(),
		},
		peers:      make(map[string]peerInfo),
		readings:   make(map[string]sensor.Reading),
		seenGossip: make(map[string]time.Time),
	}
	sensors := newSensorState()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	metrics := newNodeMetrics(cfg.nodeID)
	startMetricsServer(ctx, log, cfg.metricsAddr)

	errCh := make(chan error, 2)

	go func() {
		if err := runTCPServer(ctx, log, cfg.listenAddr, state, cfg); err != nil {
			errCh <- err
		}
	}()

	go func() {
		if err := bootstrapDiscovery(ctx, log, cfg, state, metrics); err != nil {
			errCh <- err
		}
	}()

	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			metrics.SetRoutingTableSize(cfg.nodeID, state.peerCount())
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()

	go func() {
		generator := sensor.NewGenerator(cfg.nodeID, sensor.LocationFromSeed(cfg.nodeID))
		ticker := time.NewTicker(cfg.sensorInterval)
		defer ticker.Stop()
		for {
			reading := generator.Next()
			sensors.Set(reading)
			state.setReading(reading)
			metrics.SetTemperature(cfg.nodeID, reading.TemperatureC)
			log.WithFields(logrus.Fields{
				"temperature_c": reading.TemperatureC,
				"latitude":      reading.Location.Latitude,
				"longitude":     reading.Location.Longitude,
			}).Info("sensor reading")

			if err := gossipBroadcast(ctx, log, cfg, state, reading); err != nil {
				log.WithError(err).Warn("gossip broadcast failed")
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()

	select {
	case <-ctx.Done():
		log.Info("shutdown signal received")
	case err := <-errCh:
		log.WithError(err).Error("node stopped due to error")
	}
}
