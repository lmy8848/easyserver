package middleware

import (
	"fmt"
	"net/http"
	"strings"

	"easyserver/internal/infra/errx"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

type JWTClaims struct {
	UserID   int64  `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

// AuthCookieName 是 Web 端登录态 cookie 名（HttpOnly，浏览器自动携带）。
// Web 走 cookie，移动端仍走 Authorization header（双通道）。
const AuthCookieName = "easyserver_token"

// SessionValidator is a function type for session validation
type SessionValidator func(token string) (bool, error)

func JWTMiddleware(secret string, sessionValidator SessionValidator) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Extract token from the Authorization header (preferred, mobile),
		// or the HttpOnly cookie (web).
		var tokenString string
		if authHeader := c.GetHeader("Authorization"); authHeader != "" {
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || parts[0] != "Bearer" {
				c.Error(errx.Unauthorized("invalid authorization format"))
				c.Abort()
				return
			}
			tokenString = parts[1]
		} else if cookie, err := c.Cookie(AuthCookieName); err == nil && cookie != "" {
			tokenString = cookie
		}

		// Fallback: allow the token via the access_token query parameter, but
		// only for safe methods (GET/HEAD). Browser media tags
		// (<audio>/<video>/<img>/<iframe>) cannot send custom headers, so media
		// preview/playback needs the token in the URL. Restricting to safe
		// methods keeps state-changing endpoints (POST/PUT/DELETE) requiring
		// the header, preserving CSRF protection.
		if tokenString == "" && (c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead) {
			tokenString = c.Query("access_token")
		}

		if tokenString == "" {
			c.Error(errx.Unauthorized("missing authorization header"))
			c.Abort()
			return
		}
		claims := &JWTClaims{}
		token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (any, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return []byte(secret), nil
		})

		if err != nil || !token.Valid {
			c.Error(&errx.Error{Kind: errx.KindUnauthorized, Code: errx.CodeTokenExpired, Message: "invalid or expired token"})
			c.Abort()
			return
		}

		// Check session validator (single session per user)
		if sessionValidator != nil {
			valid, err := sessionValidator(tokenString)
			if err != nil {
				c.Error(errx.Internal("session validation error: %w", err))
				c.Abort()
				return
			}
			if !valid {
				c.Error(errx.Unauthorized("session expired, please login again"))
				c.Abort()
				return
			}
		}

		c.Set("user_id", claims.UserID)
		c.Set("username", claims.Username)
		c.Set("role", claims.Role)
		c.Set("token", tokenString)
		c.Next()
	}
}
