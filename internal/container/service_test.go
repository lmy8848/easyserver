package container

import "testing"

func TestParsePortsString(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []PortMapping
	}{
		{"empty", "", []PortMapping{}},
		{"whitespace", "   ", []PortMapping{}},
		{
			"single ipv4",
			"0.0.0.0:8080->80/tcp",
			[]PortMapping{{HostPort: "0.0.0.0:8080", ContainerPort: "80", Protocol: "tcp"}},
		},
		{
			"ipv4 + ipv6 + udp",
			"0.0.0.0:8080->80/tcp, :::8080->80/tcp, 5353/udp",
			[]PortMapping{
				{HostPort: "0.0.0.0:8080", ContainerPort: "80", Protocol: "tcp"},
				{HostPort: ":::8080", ContainerPort: "80", Protocol: "tcp"},
				{HostPort: "5353/udp"}, // non-mapping token falls back to host-only
			},
		},
		{
			"no-ip prefix",
			"8080->80/tcp",
			[]PortMapping{{HostPort: "8080", ContainerPort: "80", Protocol: "tcp"}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parsePortsString(tc.in)
			if len(got) != len(tc.want) {
				t.Fatalf("len=%d want=%d (got=%+v)", len(got), len(tc.want), got)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("[%d] got=%+v want=%+v", i, got[i], tc.want[i])
				}
			}
		})
	}
}
