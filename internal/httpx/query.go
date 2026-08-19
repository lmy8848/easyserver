package httpx

import (
	"strconv"

	"github.com/gin-gonic/gin"
)

// Pagination holds parsed, clamped page/page_size params.
type Pagination struct {
	Page   int
	Size   int
	Offset int
}

// Page is the uniform paginated list response shape used by every list endpoint.
// The request's page/page_size are not echoed back; callers already know them.
type Page[T any] struct {
	Items []T   `json:"items"`
	Total int64 `json:"total"`
}

// Paginate wraps a full slice into a Page for the given pagination, applying
// the offset/size that ParsePagination already clamped. total = len(all).
func Paginate[T any](all []T, p Pagination) Page[T] {
	start := p.Offset
	if start >= len(all) {
		return Page[T]{Items: []T{}, Total: int64(len(all))}
	}
	end := min(start+p.Size, len(all))
	return Page[T]{Items: all[start:end], Total: int64(len(all))}
}

// QueryInt parses an integer query param, falling back to def when missing or invalid.
func QueryInt(c *gin.Context, key string, def int) int {
	v, err := strconv.Atoi(c.DefaultQuery(key, strconv.Itoa(def)))
	if err != nil {
		return def
	}
	return v
}

// MaxPage is the upper bound on the page query parameter to prevent integer overflow.
const MaxPage = 1_000_000

// ParsePagination parses page/page_size, clamping them, and returns Page, Size, and Offset.
// Clamping lives here (the HTTP boundary) so callers never need to re-validate.
func ParsePagination(c *gin.Context, defaultSize, maxSize int) Pagination {
	p := Pagination{
		Page: QueryInt(c, "page", 1),
		Size: QueryInt(c, "page_size", defaultSize),
	}
	if p.Page < 1 {
		p.Page = 1
	} else if p.Page > MaxPage {
		p.Page = MaxPage
	}
	if p.Size < 1 || p.Size > maxSize {
		p.Size = defaultSize
	}
	p.Offset = (p.Page - 1) * p.Size
	return p
}
