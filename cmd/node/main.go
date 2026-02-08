package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

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
		peers: make(map[string]peerInfo),
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	metrics := newNodeMetrics(cfg.nodeID)
	startMetricsServer(ctx, log, cfg.metricsAddr)

	errCh := make(chan error, 2)

	go func() {
		if err := runTCPServer(ctx, log, cfg.listenAddr, state); err != nil {
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

	select {
	case <-ctx.Done():
		log.Info("shutdown signal received")
	case err := <-errCh:
		log.WithError(err).Error("node stopped due to error")
	}
}
