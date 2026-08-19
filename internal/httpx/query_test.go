package httpx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestParsePagination(t *testing.T) {
	gin.SetMode(gin.TestMode)

	newCtx := func(qs string) *gin.Context {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/?"+qs, nil)
		return c
	}

	tests := []struct {
		name   string
		qs     string
		expect Pagination
	}{
		{"defaults", "", Pagination{1, 50, 0}},
		{"page 3", "page=3", Pagination{3, 50, 100}},
		{"size over max clamps", "page_size=999", Pagination{1, 50, 0}},
		{"size 0 clamps", "page_size=0", Pagination{1, 50, 0}},
		{"page 0 clamps", "page=0", Pagination{1, 50, 0}},
		{"negative page clamps", "page=-5", Pagination{1, 50, 0}},
		{"huge page clamps to MaxPage", "page=999999999", Pagination{MaxPage, 50, (MaxPage - 1) * 50}},
		{"garbage falls back", "page=abc&page_size=xyz", Pagination{1, 50, 0}},
		{"valid within bounds", "page=2&page_size=20", Pagination{2, 20, 20}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ParsePagination(newCtx(tt.qs), 50, 200); got != tt.expect {
				t.Errorf("ParsePagination(%q) = %+v, want %+v", tt.qs, got, tt.expect)
			}
		})
	}
}
