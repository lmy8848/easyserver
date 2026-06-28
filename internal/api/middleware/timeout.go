package middleware

import (
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// WriteTimeout overrides http.Server.WriteTimeout for the current request,
// using http.ResponseController introduced in Go 1.20. Apply to route groups
// whose handlers run long subprocess operations (docker pull, runtime install,
// package install) so the global server timeout can stay short.
func WriteTimeout(d time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		// ErrNotSupported means the writer doesn't expose a deadline — falls
		// back to server-level WriteTimeout. Anything else is unexpected and
		// would silently regress the route, so log it.
		if err := http.NewResponseController(c.Writer).SetWriteDeadline(time.Now().Add(d)); err != nil && !errors.Is(err, http.ErrNotSupported) {
			log.Printf("middleware.WriteTimeout: SetWriteDeadline failed on %s %s: %v", c.Request.Method, c.Request.URL.Path, err)
		}
		c.Next()
	}
}
