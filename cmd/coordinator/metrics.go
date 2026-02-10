package main

import (
	"context"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
)

type coordinatorMetrics struct {
	isLeader      *prometheus.GaugeVec
	currentTerm   *prometheus.GaugeVec
	commitIndex   *prometheus.GaugeVec
	lastHeartbeat *prometheus.GaugeVec
}

func newCoordinatorMetrics(nodeID string) *coordinatorMetrics {
	isLeader := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "edgesim_raft_is_leader",
		Help: "1 if this coordinator is the Raft leader, 0 otherwise",
	}, []string{"node_id"})

	currentTerm := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "edgesim_raft_current_term",
		Help: "Current Raft term for the coordinator",
	}, []string{"node_id"})

	commitIndex := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "edgesim_raft_commit_index",
		Help: "Highest committed log index on the coordinator",
	}, []string{"node_id"})

	lastHeartbeat := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "edgesim_raft_last_heartbeat_timestamp_seconds",
		Help: "Unix timestamp of the last observed heartbeat",
	}, []string{"node_id"})

	prometheus.MustRegister(isLeader, currentTerm, commitIndex, lastHeartbeat)
	return &coordinatorMetrics{
		isLeader:      isLeader,
		currentTerm:   currentTerm,
		commitIndex:   commitIndex,
		lastHeartbeat: lastHeartbeat,
	}
}

func (m *coordinatorMetrics) Update(nodeID string, isLeader bool, term uint64, commitIndex uint64, lastHeartbeat time.Time) {
	if m == nil {
		return
	}
	leaderValue := 0.0
	if isLeader {
		leaderValue = 1.0
	}
	m.isLeader.WithLabelValues(nodeID).Set(leaderValue)
	m.currentTerm.WithLabelValues(nodeID).Set(float64(term))
	m.commitIndex.WithLabelValues(nodeID).Set(float64(commitIndex))
	if !lastHeartbeat.IsZero() {
		m.lastHeartbeat.WithLabelValues(nodeID).Set(float64(lastHeartbeat.Unix()))
	}
}

func startMetricsServer(ctx context.Context, log *logrus.Entry, addr string) {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())

	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	go func() {
		log.WithField("metrics_addr", addr).Info("metrics server listening")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.WithError(err).Error("metrics server failed")
		}
	}()
}
