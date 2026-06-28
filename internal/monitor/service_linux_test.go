//go:build linux

package monitor

import "testing"

func TestDeltaU64(t *testing.T) {
	cases := []struct {
		name      string
		cur, prev uint64
		want      uint64
	}{
		{"normal", 1000, 600, 400},
		{"equal", 1000, 1000, 0},
		{"counter_reset", 100, 1000, 0}, // 关键：网卡重置 / 内存账目异常时绝不回绕
		{"prev_zero", 1000, 0, 1000},
		{"both_zero", 0, 0, 0},
		{"near_overflow", 1 << 63, (1 << 63) - 1, 1}, // 接近边界仍为小正差
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := deltaU64(tc.cur, tc.prev); got != tc.want {
				t.Errorf("deltaU64(%d,%d)=%d want=%d", tc.cur, tc.prev, got, tc.want)
			}
		})
	}
}
