package http

import "testing"

func TestValidIPOrCIDR(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"192.168.1.1", true},
		{"10.0.0.1", true},
		{"192.168.1.0/24", true},
		{"::1", true},
		{"2001:db8::/32", true},
		{"", false},
		{"not-an-ip", false},
		{"256.1.1.1", false},
		{"1.2.3.4/99", false},
	}
	for _, c := range cases {
		if got := validIPOrCIDR(c.in); got != c.want {
			t.Errorf("validIPOrCIDR(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
