package middleware

import (
	"fmt"
	"strings"

	"easyserver/internal/infra/apperror"
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
				c.Error(apperror.ErrUnauthorized.WithMessage("invalid authorization format"))
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
		if tokenString == "" && (c.Request.Method == "GET" || c.Request.Method == "HEAD") {
			tokenString = c.Query("access_token")
		}

		if tokenString == "" {
			c.Error(apperror.ErrUnauthorized.WithMessage("missing authorization header"))
			c.Abort()
			return
		}
		claims := &JWTClaims{}
		token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return []byte(secret), nil
		})

		if err != nil || !token.Valid {
			c.Error(apperror.ErrTokenExpired.WithMessage("invalid or expired token"))
			c.Abort()
			return
		}

		// Check session validator (single session per user)
		if sessionValidator != nil {
			valid, err := sessionValidator(tokenString)
			if err != nil {
				c.Error(apperror.ErrInternal.WithMessage("session validation error"))
				c.Abort()
				return
			}
			if !valid {
				c.Error(apperror.ErrUnauthorized.WithMessage("session expired, please login again"))
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
