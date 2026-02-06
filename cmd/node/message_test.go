package main

import (
	"bufio"
	"bytes"
	"testing"
)

func TestFrameRoundTrip(t *testing.T) {
	payloads := [][]byte{
		[]byte("hello"),
		bytes.Repeat([]byte("a"), 256),
		bytes.Repeat([]byte("b"), 1024),
	}

	var buffer bytes.Buffer
	writer := bufio.NewWriter(&buffer)
	for _, payload := range payloads {
		if err := writeFrame(writer, payload); err != nil {
			t.Fatalf("writeFrame failed: %v", err)
		}
	}

	reader := bufio.NewReader(&buffer)
	for i, payload := range payloads {
		got, err := readFrame(reader)
		if err != nil {
			t.Fatalf("readFrame failed at index %d: %v", i, err)
		}
		if !bytes.Equal(got, payload) {
			t.Fatalf("payload mismatch at index %d: got %q want %q", i, string(got), string(payload))
		}
	}
}

func TestFrameZeroLength(t *testing.T) {
	var buffer bytes.Buffer
	writer := bufio.NewWriter(&buffer)
	if err := writeFrame(writer, []byte{}); err != nil {
		t.Fatalf("writeFrame failed: %v", err)
	}

	reader := bufio.NewReader(&buffer)
	got, err := readFrame(reader)
	if err != nil {
		t.Fatalf("readFrame failed: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty payload, got %q", string(got))
	}
}
