package container

import (
	"encoding/json"
	"testing"
)

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

// TestParseJSONRowsPodman ensures the Podman output shape (single JSON array,
// lowercase-ish fields, typed arrays where Docker uses strings) parses via the
// centralized dispatch into the public Container/Image models.
func TestParseJSONRowsPodman(t *testing.T) {
	psOutput := `[
	  {"Id":"34359ee","Names":["debian-dev"],"Image":"docker.io/library/debian:13-slim",
	   "State":"exited","CreatedAt":"4 days ago","Command":["--init"],
	   "Ports":null,"Labels":{"a":"b"},"Mounts":["/dev"],"Networks":["podman"],"Size":0}
	]`
	rows, err := parseJSONRows(psOutput, func(line []byte) (any, bool) {
		var d podmanPSRow
		if err := json.Unmarshal(line, &d); err != nil {
			return nil, false
		}
		return d.toContainer(), true
	})
	if err != nil {
		t.Fatalf("parseJSONRows: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("len=%d want=1", len(rows))
	}
	c := rows[0].(Container)
	if c.Name != "debian-dev" || c.State != "exited" || c.Image == "" {
		t.Errorf("container mapping wrong: %+v", c)
	}

	imgOutput := `[{"Id":"b82115d","Repository":"docker.io/library/nginx","Tag":"1.25",
	  "Size":123456,"CreatedAt":"2026-08-05T16:18:31Z","Labels":{"k":"v"}}]`
	rows, err = parseJSONRows(imgOutput, func(line []byte) (any, bool) {
		var d podmanImageRow
		if err := json.Unmarshal(line, &d); err != nil {
			return nil, false
		}
		return d.toImage(), true
	})
	if err != nil {
		t.Fatalf("parseJSONRows images: %v", err)
	}
	img := rows[0].(Image)
	if img.Repository != "docker.io/library/nginx" || img.Tag != "1.25" {
		t.Errorf("image mapping wrong: %+v", img)
	}
}
