package main

import (
	"context"
	"encoding/base64"
	"time"

	"github.com/Saiweb3dev/edgesim-p2p/pkg/raft"
	"github.com/Saiweb3dev/edgesim-p2p/pkg/sensor"
	"github.com/sirupsen/logrus"
)

func encodeReadingCommand(reading sensor.Reading) (string, error) {
	payload, err := sensor.EncodeReading(reading)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(payload), nil
}

func applyReadingCommand(log *logrus.Entry, store *readingStore, command string) {
	payload, err := base64.StdEncoding.DecodeString(command)
	if err != nil {
		log.WithError(err).Warn("raft command decode failed")
		return
	}
	reading, err := sensor.DecodeReading(payload)
	if err != nil {
		log.WithError(err).Warn("raft reading decode failed")
		return
	}
	storeAndLog(log, store, reading)
}

func storeAndLog(log *logrus.Entry, store *readingStore, reading sensor.Reading) {
	count, reached, duration := store.upsert(reading)
	log.WithFields(logrus.Fields{
		"node_id":       reading.NodeID,
		"temperature_c": reading.TemperatureC,
		"readings":      count,
	}).Info("reading stored")

	if reached {
		log.WithFields(logrus.Fields{
			"target_count": store.targetCount,
			"duration_ms":  duration.Milliseconds(),
		}).Info("target reached")
	}
}

func updateRaftMetrics(ctx context.Context, metrics *coordinatorMetrics, node *raft.Node, nodeID string) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			isLeader := node.State() == raft.StateLeader
			metrics.Update(nodeID, isLeader, node.CurrentTerm(), node.CommitIndex(), node.LastHeartbeatTime())
		}
	}
}
