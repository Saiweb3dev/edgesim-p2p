package main

import (
	"bufio"
	"encoding/binary"
	"io"
	"strings"
)

// parseMessage splits a wire message into its type and arguments.
func parseMessage(message string) (string, []string) {
	parts := strings.Split(message, "|")
	if len(parts) == 0 {
		return "", nil
	}
	msgType := strings.ToUpper(strings.TrimSpace(parts[0]))
	return msgType, parts[1:]
}

// splitAndTrim converts a comma-separated list into cleaned items.
func splitAndTrim(value string) []string {
	items := strings.Split(value, ",")
	clean := make([]string, 0, len(items))
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		clean = append(clean, trimmed)
	}
	return clean
}

// readFrame reads a length-prefixed frame (4-byte big-endian header + payload).
func readFrame(reader *bufio.Reader) ([]byte, error) {
	var header [4]byte
	if _, err := io.ReadFull(reader, header[:]); err != nil {
		return nil, err
	}

	length := binary.BigEndian.Uint32(header[:])
	if length == 0 {
		return []byte{}, nil
	}

	payload := make([]byte, length)
	if _, err := io.ReadFull(reader, payload); err != nil {
		return nil, err
	}

	return payload, nil
}

// writeFrame writes a length-prefixed frame (4-byte big-endian header + payload).
func writeFrame(writer *bufio.Writer, payload []byte) error {
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(payload)))

	if _, err := writer.Write(header[:]); err != nil {
		return err
	}
	if len(payload) > 0 {
		if _, err := writer.Write(payload); err != nil {
			return err
		}
	}

	return writer.Flush()
}
