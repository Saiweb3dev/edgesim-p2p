package main

import (
	"context"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
)

type nodeMetrics struct {
	lookupLatency    *prometheus.HistogramVec
	routingTableSize *prometheus.GaugeVec
	temperatureC     *prometheus.GaugeVec
}

func newNodeMetrics(nodeID string) *nodeMetrics {
	lookupLatency := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "edgesim_dht_lookup_latency_seconds",
		Help:    "Peer lookup latency in seconds",
		Buckets: prometheus.ExponentialBuckets(0.001, 2, 12),
	}, []string{"node_id"})

	routingTableSize := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "edgesim_dht_routing_table_size",
		Help: "Number of peers known to the node",
	}, []string{"node_id"})

	temperatureC := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "edgesim_sensor_temperature_celsius",
		Help: "Current temperature reading in Celsius",
	}, []string{"node_id"})

	prometheus.MustRegister(lookupLatency, routingTableSize, temperatureC)
	return &nodeMetrics{lookupLatency: lookupLatency, routingTableSize: routingTableSize, temperatureC: temperatureC}
}

func (m *nodeMetrics) ObserveLookupLatency(nodeID string, duration time.Duration) {
	if m == nil {
		return
	}
	m.lookupLatency.WithLabelValues(nodeID).Observe(duration.Seconds())
}

func (m *nodeMetrics) SetRoutingTableSize(nodeID string, size int) {
	if m == nil {
		return
	}
	m.routingTableSize.WithLabelValues(nodeID).Set(float64(size))
}

func (m *nodeMetrics) SetTemperature(nodeID string, temperatureC float64) {
	if m == nil {
		return
	}
	m.temperatureC.WithLabelValues(nodeID).Set(temperatureC)
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
