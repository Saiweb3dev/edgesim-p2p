package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
)

type config struct {
	nodeID         string
	listenAddr     string
	advertiseAddr  string
	peerAddr       string
	bootstrapAddrs []string
	message        string
	retryInterval  time.Duration
}

type peerInfo struct {
	nodeID   string
	addr     string
	lastSeen time.Time
}

type nodeState struct {
	mu    sync.RWMutex
	self  peerInfo
	peers map[string]peerInfo
}

func main() {
	cfg := loadConfig()
	logger := logrus.New()
	logger.SetFormatter(&logrus.TextFormatter{FullTimestamp: true})

	log := logger.WithFields(logrus.Fields{
		"node_id":        cfg.nodeID,
		"listen_addr":    cfg.listenAddr,
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

	errCh := make(chan error, 2)

	go func() {
		if err := runTCPServer(ctx, log, cfg.listenAddr, state); err != nil {
			errCh <- err
		}
	}()

	go func() {
		if err := bootstrapDiscovery(ctx, log, cfg, state); err != nil {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		log.Info("shutdown signal received")
	case err := <-errCh:
		log.WithError(err).Error("node stopped due to error")
	}
}

func loadConfig() config {
	var cfg config
	flag.StringVar(&cfg.nodeID, "node-id", getenvDefault("NODE_ID", ""), "logical node identifier")
	flag.StringVar(&cfg.listenAddr, "listen", getenvDefault("LISTEN_ADDR", "0.0.0.0:8000"), "TCP listen address")
	flag.StringVar(&cfg.advertiseAddr, "advertise", getenvDefault("ADVERTISE_ADDR", ""), "address shared with peers")
	flag.StringVar(&cfg.peerAddr, "peer", getenvDefault("PEER_ADDR", ""), "peer address to connect")
	bootstrap := flag.String("bootstrap", getenvDefault("BOOTSTRAP_ADDRS", ""), "comma-separated bootstrap addrs")
	flag.StringVar(&cfg.message, "message", getenvDefault("MESSAGE", "hello"), "message to send to peer")
	retry := flag.Duration("retry", 2*time.Second, "retry interval for peer connection")
	flag.Parse()

	cfg.retryInterval = *retry
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

func runTCPServer(ctx context.Context, log *logrus.Entry, listenAddr string, state *nodeState) error {
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

		go handleConn(ctx, log, conn, state)
	}
}

func handleConn(ctx context.Context, log *logrus.Entry, conn net.Conn, state *nodeState) {
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

func bootstrapDiscovery(ctx context.Context, log *logrus.Entry, cfg config, state *nodeState) error {
	if len(cfg.bootstrapAddrs) == 0 && cfg.peerAddr == "" {
		return nil
	}

	if cfg.peerAddr != "" {
		cfg.bootstrapAddrs = append(cfg.bootstrapAddrs, cfg.peerAddr)
	}

	var peers []string
	for _, addr := range cfg.bootstrapAddrs {
		if addr == "" {
			continue
		}
		bootstrapPeers, err := helloAndGetPeers(ctx, log, cfg, addr)
		if err != nil {
			log.WithError(err).WithField("bootstrap", addr).Warn("bootstrap failed")
			continue
		}
		peers = append(peers, bootstrapPeers...)
	}

	for _, addr := range peers {
		if addr == "" || addr == cfg.advertiseAddr {
			continue
		}
		state.upsertPeer(peerInfo{nodeID: "", addr: addr, lastSeen: time.Now()})
	}

	for _, peer := range state.peerAddrs() {
		if peer == cfg.advertiseAddr {
			continue
		}
		if err := sendMessageToPeer(log, cfg, peer); err != nil {
			log.WithError(err).WithField("peer_addr", peer).Warn("send failed")
		}
	}

	return nil
}

func helloAndGetPeers(ctx context.Context, log *logrus.Entry, cfg config, addr string) ([]string, error) {
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		return nil, fmt.Errorf("dial bootstrap %s: %w", addr, err)
	}
	defer conn.Close()

	writer := bufio.NewWriter(conn)
	reader := bufio.NewReader(conn)

	hello := fmt.Sprintf("HELLO|%s|%s", cfg.nodeID, cfg.advertiseAddr)
	if err := writeFrame(writer, []byte(hello)); err != nil {
		return nil, fmt.Errorf("send hello: %w", err)
	}
	if _, err := readFrame(reader); err != nil {
		log.WithError(err).WithField("bootstrap", addr).Warn("hello ack missing")
	}

	if err := writeFrame(writer, []byte("GETPEERS")); err != nil {
		return nil, fmt.Errorf("send getpeers: %w", err)
	}
	response, err := readFrame(reader)
	if err != nil {
		return nil, fmt.Errorf("read peers: %w", err)
	}

	msgType, parts := parseMessage(strings.TrimSpace(string(response)))
	if msgType != "PEERS" || len(parts) == 0 {
		return nil, fmt.Errorf("unexpected peers response: %s", string(response))
	}
	peers := splitAndTrim(parts[0])

	log.WithFields(logrus.Fields{
		"bootstrap": addr,
		"peers":     peers,
	}).Info("bootstrap peers received")

	return peers, nil
}

func sendMessageToPeer(log *logrus.Entry, cfg config, addr string) error {
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()

	return sendMessage(conn, log, cfg)
}

func (s *nodeState) upsertPeer(peer peerInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if peer.addr == "" {
		return
	}
	peer.lastSeen = time.Now()
	s.peers[peer.addr] = peer
}

func (s *nodeState) peerAddrs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	addrs := make([]string, 0, len(s.peers)+1)
	addrs = append(addrs, s.self.addr)
	for addr := range s.peers {
		addrs = append(addrs, addr)
	}
	sort.Strings(addrs)
	return addrs
}

func getenvDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
