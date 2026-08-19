package util

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"time"

	infrasystemd "easyserver/internal/infra/systemd"
)

// TimeLayout is the standard human-readable timestamp layout used across the app.
const TimeLayout = "2006-01-02 15:04:05"

// SystemdUnitActive reports whether a systemd unit is in the active state.
func SystemdUnitActive(ctx context.Context, unit string) bool {
	prop, err := infrasystemd.DefaultClient().GetUnitPropertyContext(ctx, unit, "ActiveState")
	if err != nil || prop == nil {
		return false
	}
	str, ok := prop.Value.Value().(string)
	return ok && str == "active"
}

// SystemdUnitEnabled reports whether a systemd unit is enabled at boot.
func SystemdUnitEnabled(ctx context.Context, unit string) bool {
	prop, err := infrasystemd.DefaultClient().GetUnitPropertyContext(ctx, unit, "UnitFileState")
	if err != nil || prop == nil {
		return false
	}
	str, ok := prop.Value.Value().(string)
	return ok && str == "enabled"
}

// RandomHex returns n cryptographically random bytes hex-encoded.
func RandomHex(n int) (string, error) {
	if n <= 0 {
		return "", errors.New("length must be positive")
	}
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// RandomBase64 returns n cryptographically random bytes URL-safe base64-encoded.
func RandomBase64(n int) (string, error) {
	if n <= 0 {
		return "", errors.New("length must be positive")
	}
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// UnixMicros converts a microsecond epoch (e.g. journald __REALTIME_TIMESTAMP) to time.Time.
func UnixMicros(usec int64) time.Time {
	return time.Unix(usec/1e6, (usec%1e6)*1000)
}
