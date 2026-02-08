package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

func runTCPServer(ctx context.Context, log *logrus.Entry, listenAddr string, state *nodeState, cfg config) error {
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", listenAddr, err)
	}
	log.WithField("listen_addr", listenAddr).Info("tcp server listening")

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
			log.WithError(err).Warn("accept failed")
			continue
		}

		go handleConn(ctx, log, conn, state, cfg)
	}
}

func handleConn(ctx context.Context, log *logrus.Entry, conn net.Conn, state *nodeState, cfg config) {
	defer conn.Close()
	remote := conn.RemoteAddr().String()
	log.WithField("remote", remote).Info("connection accepted")

	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)

	for {
		payload, err := readFrame(reader)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
				break
			}
			log.WithError(err).WithField("remote", remote).Warn("connection read error")
			break
		}

		message := strings.TrimSpace(string(payload))
		if message == "" {
			continue
		}

		msgType, parts := parseMessage(message)
		switch msgType {
		case "HELLO":
			if len(parts) < 2 {
				log.WithFields(logrus.Fields{"remote": remote, "message": message}).Warn("invalid hello")
				break
			}
			peer := peerInfo{nodeID: parts[0], addr: parts[1], lastSeen: time.Now()}
			state.upsertPeer(peer)
			log.WithFields(logrus.Fields{
				"remote":    remote,
				"peer_id":   peer.nodeID,
				"peer_addr": peer.addr,
			}).Info("peer registered")

			if err := writeFrame(writer, []byte("ACK|HELLO")); err != nil {
				log.WithError(err).WithField("remote", remote).Warn("failed to send ack")
				break
			}
		case "GETPEERS":
			peers := state.peerAddrs()
			response := "PEERS|" + strings.Join(peers, ",")
			if err := writeFrame(writer, []byte(response)); err != nil {
				log.WithError(err).WithField("remote", remote).Warn("failed to send peers")
				break
			}
		case "MESSAGE":
			if len(parts) < 2 {
				log.WithFields(logrus.Fields{"remote": remote, "message": message}).Warn("invalid message")
				break
			}
			log.WithFields(logrus.Fields{
				"remote":  remote,
				"from":    parts[0],
				"message": parts[1],
			}).Info("message received")

			ack := fmt.Sprintf("ACK|MESSAGE|%s", time.Now().Format(time.RFC3339))
			if err := writeFrame(writer, []byte(ack)); err != nil {
				log.WithError(err).WithField("remote", remote).Warn("failed to send ack")
				break
			}
		case gossipMsgType:
			handleGossipMessage(ctx, log, state, cfg, parts)
		default:
			log.WithFields(logrus.Fields{
				"remote":  remote,
				"message": message,
			}).Warn("unknown message")
		}
	}

	select {
	case <-ctx.Done():
	default:
		log.WithField("remote", remote).Info("connection closed")
	}
}

func sendMessage(conn net.Conn, log *logrus.Entry, cfg config) error {
	payload := fmt.Sprintf("MESSAGE|%s|%s", cfg.nodeID, cfg.message)
	writer := bufio.NewWriter(conn)
	if err := writeFrame(writer, []byte(payload)); err != nil {
		return fmt.Errorf("send message: %w", err)
	}

	log.WithFields(logrus.Fields{
		"peer_addr": conn.RemoteAddr().String(),
		"payload":   payload,
	}).Info("message sent")

	reader := bufio.NewReader(conn)
	if ack, err := readFrame(reader); err != nil {
		log.WithError(err).WithField("peer_addr", conn.RemoteAddr().String()).Warn("ack read failed")
	} else {
		log.WithFields(logrus.Fields{
			"peer_addr": conn.RemoteAddr().String(),
			"ack":       strings.TrimSpace(string(ack)),
		}).Info("ack received")
	}

	return nil
}
