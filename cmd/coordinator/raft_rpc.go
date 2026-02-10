package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/Saiweb3dev/edgesim-p2p/pkg/raft"
	"github.com/sirupsen/logrus"
)

const (
	rpcRequestVote   = "REQUEST_VOTE"
	rpcAppendEntries = "APPEND_ENTRIES"
	rpcPropose       = "PROPOSE"
)

type raftEnvelope struct {
	Type          string                     `json:"type"`
	RequestVote   *raft.RequestVoteRequest   `json:"request_vote,omitempty"`
	AppendEntries *raft.AppendEntriesRequest `json:"append_entries,omitempty"`
	Propose       *proposeRequest            `json:"propose,omitempty"`
}

type proposeRequest struct {
	Command string `json:"command"`
}

type proposeResponse struct {
	OK       bool   `json:"ok"`
	LeaderID string `json:"leader_id,omitempty"`
	Error    string `json:"error,omitempty"`
}

func runRaftServer(ctx context.Context, log *logrus.Entry, addr string, node *raft.Node) error {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", addr, err)
	}
	log.WithField("raft_addr", addr).Info("raft rpc server listening")

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
			log.WithError(err).Warn("raft accept failed")
			continue
		}
		go handleRaftConn(ctx, log, conn, node)
	}
}

func handleRaftConn(ctx context.Context, log *logrus.Entry, conn net.Conn, node *raft.Node) {
	defer conn.Close()

	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)

	payload, err := readFrame(reader)
	if err != nil {
		log.WithError(err).Warn("raft read failed")
		return
	}

	var envelope raftEnvelope
	if err := json.Unmarshal(payload, &envelope); err != nil {
		log.WithError(err).Warn("raft decode failed")
		return
	}

	switch envelope.Type {
	case rpcRequestVote:
		if envelope.RequestVote == nil {
			return
		}
		resp := node.HandleRequestVote(ctx, *envelope.RequestVote)
		writeJSONFrame(writer, resp)
	case rpcAppendEntries:
		if envelope.AppendEntries == nil {
			return
		}
		resp := node.HandleAppendEntries(ctx, *envelope.AppendEntries)
		writeJSONFrame(writer, resp)
	case rpcPropose:
		if envelope.Propose == nil {
			return
		}
		err := node.Propose(ctx, envelope.Propose.Command)
		resp := proposeResponse{OK: err == nil}
		if err != nil {
			resp.Error = err.Error()
			if errors.Is(err, raft.ErrNotLeader) {
				resp.LeaderID = node.LeaderID()
			}
		}
		writeJSONFrame(writer, resp)
	default:
		log.WithField("type", envelope.Type).Warn("unknown raft rpc type")
	}
}

func sendPropose(ctx context.Context, addr string, command string) (proposeResponse, error) {
	req := raftEnvelope{
		Type:    rpcPropose,
		Propose: &proposeRequest{Command: command},
	}
	var resp proposeResponse
	if err := sendRaftRPC(ctx, addr, req, &resp); err != nil {
		return proposeResponse{}, err
	}
	return resp, nil
}

func sendRaftRPC(ctx context.Context, addr string, req raftEnvelope, resp interface{}) error {
	dialer := net.Dialer{Timeout: 2 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()

	payload, err := json.Marshal(req)
	if err != nil {
		return err
	}

	writer := bufio.NewWriter(conn)
	if err := writeFrame(writer, payload); err != nil {
		return err
	}

	reader := bufio.NewReader(conn)
	responsePayload, err := readFrame(reader)
	if err != nil {
		return err
	}

	return json.Unmarshal(responsePayload, resp)
}

func writeJSONFrame(writer *bufio.Writer, payload interface{}) {
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	_ = writeFrame(writer, data)
}
