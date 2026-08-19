package container

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestDemuxLogs(t *testing.T) {
	var stream bytes.Buffer

	// Frame 1: stdout, 5 bytes: "hello"
	header1 := make([]byte, 8)
	header1[0] = 1
	binary.BigEndian.PutUint32(header1[4:8], 5)
	stream.Write(header1)
	stream.WriteString("hello")

	// Frame 2: stderr, 5 bytes: "error"
	header2 := make([]byte, 8)
	header2[0] = 2
	binary.BigEndian.PutUint32(header2[4:8], 5)
	stream.Write(header2)
	stream.WriteString("error")

	// Frame 3: stdout, 6 bytes: " world"
	header3 := make([]byte, 8)
	header3[0] = 1
	binary.BigEndian.PutUint32(header3[4:8], 6)
	stream.Write(header3)
	stream.WriteString(" world")

	var stdout, stderr bytes.Buffer
	if err := DemuxLogs(&stream, &stdout, &stderr); err != nil {
		t.Fatalf("DemuxLogs failed: %v", err)
	}

	if stdout.String() != "hello world" {
		t.Errorf("expected stdout 'hello world', got '%s'", stdout.String())
	}
	if stderr.String() != "error" {
		t.Errorf("expected stderr 'error', got '%s'", stderr.String())
	}
}
