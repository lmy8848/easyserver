package middleware

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

type JWTClaims struct {
	UserID   int64  `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

// TOTPTempClaims is used for temporary tokens during TOTP verification
type TOTPTempClaims struct {
	UserID  int64  `json:"user_id"`
	Purpose string `json:"purpose"` // "totp_pending"
	jwt.RegisteredClaims
}

// TokenValidator is a function type for token validation (e.g., blacklist check)
type TokenValidator func(userID int64, tokenString string, issuedAt time.Time) (bool, error)

// SessionValidator is a function type for session validation
type SessionValidator func(token string) (bool, error)

func JWTMiddleware(secret string, sessionValidator SessionValidator, validators ...TokenValidator) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":    40100,
				"message": "missing authorization header",
				"data":    nil,
			})
			c.Abort()
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":    40100,
				"message": "invalid authorization format",
				"data":    nil,
			})
			c.Abort()
			return
		}

		tokenString := parts[1]
		claims := &JWTClaims{}
		token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return []byte(secret), nil
		})

		if err != nil || !token.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":    40101,
				"message": "invalid or expired token",
				"data":    nil,
			})
			c.Abort()
			return
		}

		// Check token validators (e.g., blacklist)
		for _, validator := range validators {
			if validator != nil {
				invalidated, err := validator(claims.UserID, tokenString, claims.IssuedAt.Time)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{
						"code":    50000,
						"message": "token validation error",
						"data":    nil,
					})
					c.Abort()
					return
				}
				if invalidated {
					c.JSON(http.StatusUnauthorized, gin.H{
						"code":    40102,
						"message": "token has been revoked",
						"data":    nil,
					})
					c.Abort()
					return
				}
			}
		}

		// Check session validator (single session per user)
		if sessionValidator != nil {
			valid, err := sessionValidator(tokenString)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"code":    50000,
					"message": "session validation error",
					"data":    nil,
				})
				c.Abort()
				return
			}
			if !valid {
				c.JSON(http.StatusUnauthorized, gin.H{
					"code":    40103,
					"message": "session expired, please login again",
					"data":    nil,
				})
				c.Abort()
				return
			}
		}

		c.Set("user_id", claims.UserID)
		c.Set("username", claims.Username)
		c.Set("role", claims.Role)
		c.Next()
	}
}

func GenerateToken(secret string, userID int64, username, role string, sessionTimeout time.Duration) (string, error) {
	if sessionTimeout <= 0 {
		sessionTimeout = 24 * time.Hour
	}
	claims := &JWTClaims{
		UserID:   userID,
		Username: username,
		Role:     role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(sessionTimeout)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// GenerateTOTPTempToken generates a short-lived token for TOTP verification
func GenerateTOTPTempToken(secret string, userID int64) (string, error) {
	claims := &TOTPTempClaims{
		UserID:  userID,
		Purpose: "totp_pending",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)), // 5 minute expiry
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// ValidateTOTPTempToken validates a TOTP temporary token and returns userID
func ValidateTOTPTempToken(secret string, tokenString string) (int64, error) {
	claims := &TOTPTempClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(secret), nil
	})

	if err != nil {
		return 0, err
	}
	if !token.Valid {
		return 0, fmt.Errorf("invalid token")
	}

	// Verify purpose
	if claims.Purpose != "totp_pending" {
		return 0, fmt.Errorf("invalid token purpose")
	}

	return claims.UserID, nil
}
