package auth

import (
	"errors"
	"fmt"
	"time"

	"easyserver/internal/httpx/middleware"

	"github.com/golang-jwt/jwt/v5"
)

type JWTClaims = middleware.JWTClaims

// TOTPTempClaims is used for temporary tokens during TOTP verification
type TOTPTempClaims struct {
	UserID  int64  `json:"user_id"`
	Purpose string `json:"purpose"` // "totp_pending"
	jwt.RegisteredClaims
}

// SessionValidator is a function type for session validation
type SessionValidator = middleware.SessionValidator

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
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// ValidateTOTPTempToken validates a TOTP temporary token and returns userID
func ValidateTOTPTempToken(secret string, tokenString string) (int64, error) {
	claims := &TOTPTempClaims{}
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
		return 0, errors.New("invalid token")
	}

	// Verify purpose
	if claims.Purpose != "totp_pending" {
		return 0, errors.New("invalid token purpose")
	}

	return claims.UserID, nil
}
