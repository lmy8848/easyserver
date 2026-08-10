package container

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"easyserver/internal/infra/executor"
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

func TestHumanSize(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0B"},
		{512, "512B"},
		{164982104, "165MB"},
		{1536, "1.5KB"},
		{1 << 20, "1MB"},
		{5 * 1 << 30, "5.4GB"},
	}
	for _, tc := range cases {
		if got := humanSize(tc.in); got != tc.want {
			t.Errorf("humanSize(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestExpandImageRef(t *testing.T) {
	cases := []struct{ in, want string }{
		{"nginx:latest", "docker.io/library/nginx:latest"},
		{"nginx", "docker.io/library/nginx"},
		{"redis:alpine", "docker.io/library/redis:alpine"},
		{"foo/bar:v1", "docker.io/foo/bar:v1"},
		{"docker.io/library/nginx:latest", "docker.io/library/nginx:latest"},
		{"ghcr.io/org/app:tag", "ghcr.io/org/app:tag"},
		{"localhost:5000/app", "localhost:5000/app"},
	}
	for _, tc := range cases {
		if got := expandImageRef(tc.in); got != tc.want {
			t.Errorf("expandImageRef(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestGetPodmanRegistryConfig(t *testing.T) {
	mock := executor.NewMockExecutor()
	mock.SetResponse("cat /etc/containers/registries.conf", executor.MockSuccess(`
unqualified-search-registries = ["docker.io"]

[[registry]]
location = "registry.local:5000"
insecure = true

[[registry]]
location = "docker.io"
insecure = false
`))
	s := NewService(mock)
	got, err := s.GetRegistryConfig(context.Background(), EnginePodman)
	if err != nil {
		t.Fatalf("GetRegistryConfig: %v", err)
	}
	want := RegistryConfig{
		Mirrors:            []string{"docker.io"},
		InsecureRegistries: []string{"registry.local:5000"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got=%+v want=%+v", got, want)
	}
}
