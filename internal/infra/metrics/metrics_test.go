package metrics

import (
	"strings"
	"testing"
)

func TestRenderContainsCounterAndHistogram(t *testing.T) {
	r := NewRegistry()
	k := seriesKey{"GET", "/api/health", "200"}
	r.counts[k] = 3
	r.histo[k] = []uint64{1, 1, 1, 0, 0, 0, 0, 0, 0, 0}
	r.sumMs[k] = 42.5
	r.sessions = 2

	out := r.Render()
	for _, want := range []string{
		`easyserver_http_requests_total{method="GET",route="/api/health",status="200"} 3`,
		`le="5"} 1`,
		`le="+Inf"} 3`,
		`easyserver_http_request_duration_milliseconds_sum{method="GET",route="/api/health",status="200"} 42.500`,
		"easyserver_sessions_active 2",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in output:\n%s", want, out)
		}
	}
}
