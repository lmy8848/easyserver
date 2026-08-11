package database

import "testing"

func TestIsTruthy(t *testing.T) {
	cases := []struct {
		row  []any
		i    int
		want bool
	}{
		{[]any{"no"}, 0, false},
		{[]any{"OFF"}, 0, false},
		{[]any{"0"}, 0, false},
		{[]any{""}, 0, false},
		{[]any{"yes"}, 0, true},
		{[]any{"YES"}, 0, true},
		{[]any{"pri"}, 0, true},
		{[]any{[]byte("no")}, 0, false},
		{[]any{[]byte("yes")}, 0, true},
		{[]any{[]byte("pri")}, 0, true},
		{[]any{false}, 0, false},
		{[]any{true}, 0, true},
		{[]any{nil}, 0, false},
		{[]any{"x"}, 99, false}, // 越界
	}
	for _, c := range cases {
		if got := isTruthy(c.row, c.i); got != c.want {
			t.Fatalf("isTruthy(%v, %d) = %v, want %v", c.row, c.i, got, c.want)
		}
	}
}
