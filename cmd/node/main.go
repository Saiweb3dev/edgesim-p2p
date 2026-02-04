package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
)

type config struct {
	nodeID        string
	listenAddr    string
	peerAddr      string
	message       string
	retryInterval time.Duration
}

func main() {
	cfg := loadConfig()
	logger := logrus.New()
	logger.SetFormatter(&logrus.TextFormatter{FullTimestamp: true})

	log := logger.WithFields(logrus.Fields{
		"node_id":     cfg.nodeID,
		"listen_addr": cfg.listenAddr,
		"peer_addr":   cfg.peerAddr,
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 2)

	go func() {
		if err := runTCPServer(ctx, log, cfg.listenAddr); err != nil {
			errCh <- err
		}
	}()

	if cfg.peerAddr != "" {
		go func() {
			if err := connectAndSend(ctx, log, cfg); err != nil {
				errCh <- err
			}
		}()
	}

	select {
	case <-ctx.Done():
		log.Info("shutdown signal received")
	case err := <-errCh:
		log.WithError(err).Error("node stopped due to error")
	}
}

func loadConfig() config {
	var cfg config
	flag.StringVar(&cfg.nodeID, "node-id", getenvDefault("NODE_ID", "node-1"), "logical node identifier")
	flag.StringVar(&cfg.listenAddr, "listen", getenvDefault("LISTEN_ADDR", "0.0.0.0:8000"), "TCP listen address")
	flag.StringVar(&cfg.peerAddr, "peer", getenvDefault("PEER_ADDR", ""), "peer address to connect")
	flag.StringVar(&cfg.message, "message", getenvDefault("MESSAGE", "hello"), "message to send to peer")
	retry := flag.Duration("retry", 2*time.Second, "retry interval for peer connection")
	flag.Parse()

	cfg.retryInterval = *retry
	return cfg
}

func runTCPServer(ctx context.Context, log *logrus.Entry, listenAddr string) error {
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

		go handleConn(ctx, log, conn)
	}
}

func handleConn(ctx context.Context, log *logrus.Entry, conn net.Conn) {
	defer conn.Close()
	remote := conn.RemoteAddr().String()
	log.WithField("remote", remote).Info("connection accepted")

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		log.WithFields(logrus.Fields{
			"remote":  remote,
			"message": line,
		}).Info("message received")

		if _, err := fmt.Fprintf(conn, "ack from %s at %s\n", conn.LocalAddr().String(), time.Now().Format(time.RFC3339)); err != nil {
			log.WithError(err).WithField("remote", remote).Warn("failed to send ack")
		}
	}

	if err := scanner.Err(); err != nil && !errors.Is(err, net.ErrClosed) {
		log.WithError(err).WithField("remote", remote).Warn("connection read error")
	}

	select {
	case <-ctx.Done():
	default:
		log.WithField("remote", remote).Info("connection closed")
	}
}

func connectAndSend(ctx context.Context, log *logrus.Entry, cfg config) error {
	ticker := time.NewTicker(cfg.retryInterval)
	defer ticker.Stop()

	for {
		conn, err := net.Dial("tcp", cfg.peerAddr)
		if err != nil {
			log.WithError(err).WithField("peer_addr", cfg.peerAddr).Warn("peer dial failed")
		} else {
			if err := sendMessage(conn, log, cfg); err != nil {
				_ = conn.Close()
				return err
			}
			_ = conn.Close()
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func sendMessage(conn net.Conn, log *logrus.Entry, cfg config) error {
	payload := fmt.Sprintf("%s|%s|%s", cfg.nodeID, time.Now().Format(time.RFC3339), cfg.message)
	if _, err := fmt.Fprintf(conn, "%s\n", payload); err != nil {
		return fmt.Errorf("send message: %w", err)
	}

	log.WithFields(logrus.Fields{
		"peer_addr": conn.RemoteAddr().String(),
		"payload":   payload,
	}).Info("message sent")

	reader := bufio.NewReader(conn)
	if ack, err := reader.ReadString('\n'); err != nil {
		log.WithError(err).WithField("peer_addr", conn.RemoteAddr().String()).Warn("ack read failed")
	} else {
		log.WithFields(logrus.Fields{
			"peer_addr": conn.RemoteAddr().String(),
			"ack":       strings.TrimSpace(ack),
		}).Info("ack received")
	}

	return nil
}

func getenvDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
