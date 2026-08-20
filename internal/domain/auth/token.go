package auth

import (
	"fmt"
	"time"

	"easyserver/internal/httpx/middleware"

	"github.com/golang-jwt/jwt/v5"
)

type JWTClaims = middleware.JWTClaims

// SessionValidator is a function type for session validation
type SessionValidator = middleware.SessionValidator

func GenerateToken(secret string, userID int64, username string, sessionTimeout time.Duration) (string, error) {
	if sessionTimeout <= 0 {
		sessionTimeout = 24 * time.Hour
	}
	return generateToken(secret, userID, username, sessionTimeout)
}

// GenerateTOTPTempToken generates a short-lived token for TOTP verification
func GenerateTOTPTempToken(secret string, userID int64) (string, error) {
	return generateToken(secret, userID, "", 5*time.Minute)
}

// generateToken is the shared signing logic. TOTP 临时 token（无 username、
// 短 TTL）与正式登录 token 的区别仅是内容与有效期；它不写入会话列表，
// 因此中间件的会话校验天然拒绝它用于业务 API。
func generateToken(secret string, userID int64, username string, ttl time.Duration) (string, error) {
	claims := &JWTClaims{
		UserID:   userID,
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// ValidateTOTPTempToken validates a TOTP temporary token and returns userID
func ValidateTOTPTempToken(secret string, tokenString string) (int64, error) {
	claims := &JWTClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(secret), nil
	})

	if err != nil {
		return 0, err
	}
	if !token.Valid {
		return 0, ErrInvalidToken
	}

	return claims.UserID, nil
}
