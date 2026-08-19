package util

import (
	"testing"
	"time"
)

func TestRandomHex(t *testing.T) {
	s, err := RandomHex(8)
	if err != nil {
		t.Fatalf("RandomHex(8) failed: %v", err)
	}
	if len(s) != 16 {
		t.Errorf("RandomHex(8) length = %d, want 16", len(s))
	}

	// Non-positive n should return error
	if _, err := RandomHex(0); err == nil {
		t.Errorf("RandomHex(0) expected error, got nil")
	}
	if _, err := RandomHex(-5); err == nil {
		t.Errorf("RandomHex(-5) expected error, got nil")
	}
}

func TestRandomBase64(t *testing.T) {
	s, err := RandomBase64(16)
	if err != nil {
		t.Fatalf("RandomBase64(16) failed: %v", err)
	}
	if len(s) == 0 {
		t.Errorf("RandomBase64(16) produced empty string")
	}

	// Non-positive n should return error
	if _, err := RandomBase64(0); err == nil {
		t.Errorf("RandomBase64(0) expected error, got nil")
	}
	if _, err := RandomBase64(-10); err == nil {
		t.Errorf("RandomBase64(-10) expected error, got nil")
	}
}

func TestUnixMicros(t *testing.T) {
	epoch := int64(1700000000_123456)
	tm := UnixMicros(epoch)
	expected := time.Unix(1700000000, 123456000)
	if !tm.Equal(expected) {
		t.Errorf("UnixMicros(%d) = %v, want %v", epoch, tm, expected)
	}
}
