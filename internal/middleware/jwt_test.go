package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
)

// testErrorHandler is a simple error handler for tests
func testErrorHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if !c.Writer.Written() && len(c.Errors) > 0 {
			// Default to 401 for auth errors
			c.JSON(http.StatusUnauthorized, gin.H{"error": c.Errors.Last().Error()})
		}
	}
}

func TestGenerateToken(t *testing.T) {
	secret := "test-secret-key-at-least-32-bytes-long"
	userID := int64(1)
	username := "testuser"
	role := "admin"
	timeout := 24 * time.Hour

	token, err := GenerateToken(secret, userID, username, role, timeout)
	assert.NoError(t, err)
	assert.NotEmpty(t, token)
}

func TestGenerateToken_DifferentSecrets(t *testing.T) {
	secret1 := "test-secret-key-at-least-32-bytes-long"
	secret2 := "different-secret-key-at-least-32-bytes"
	userID := int64(1)
	username := "testuser"
	role := "admin"
	timeout := 24 * time.Hour

	token1, err := GenerateToken(secret1, userID, username, role, timeout)
	assert.NoError(t, err)

	token2, err := GenerateToken(secret2, userID, username, role, timeout)
	assert.NoError(t, err)

	assert.NotEqual(t, token1, token2)
}

func TestJWTMiddleware_ValidToken(t *testing.T) {
	secret := "test-secret-key-at-least-32-bytes-long"
	userID := int64(1)
	username := "testuser"
	role := "admin"
	timeout := 24 * time.Hour

	token, err := GenerateToken(secret, userID, username, role, timeout)
	assert.NoError(t, err)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(testErrorHandler(), JWTMiddleware(secret, nil))
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"user_id": c.GetInt64("user_id")})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestJWTMiddleware_MissingHeader(t *testing.T) {
	secret := "test-secret-key-at-least-32-bytes-long"

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(testErrorHandler(), JWTMiddleware(secret, nil))
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.NotEqual(t, http.StatusOK, w.Code)
}

func TestJWTMiddleware_InvalidFormat(t *testing.T) {
	secret := "test-secret-key-at-least-32-bytes-long"

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(testErrorHandler(), JWTMiddleware(secret, nil))
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	tests := []struct {
		name   string
		header string
	}{
		{"no bearer", "some-token"},
		{"wrong scheme", "Basic some-token"},
		{"empty token", "Bearer "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", nil)
			req.Header.Set("Authorization", tt.header)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.NotEqual(t, http.StatusOK, w.Code)
		})
	}
}

func TestJWTMiddleware_ExpiredToken(t *testing.T) {
	secret := "test-secret-key-at-least-32-bytes-long"
	userID := int64(1)
	username := "testuser"
	role := "admin"

	// Create a token that expired 1 hour ago
	claims := &JWTClaims{
		UserID:   userID,
		Username: username,
		Role:     role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
		},
	}
	tokenObj := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	token, err := tokenObj.SignedString([]byte(secret))
	assert.NoError(t, err)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(testErrorHandler(), JWTMiddleware(secret, nil))
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.NotEqual(t, http.StatusOK, w.Code)
}

func TestJWTMiddleware_WrongSecret(t *testing.T) {
	secret := "test-secret-key-at-least-32-bytes-long"
	wrongSecret := "wrong-secret-key-at-least-32-bytes-"
	userID := int64(1)
	username := "testuser"
	role := "admin"
	timeout := 24 * time.Hour

	// Generate token with one secret
	token, err := GenerateToken(secret, userID, username, role, timeout)
	assert.NoError(t, err)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	// Validate with different secret
	router.Use(testErrorHandler(), JWTMiddleware(wrongSecret, nil))
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.NotEqual(t, http.StatusOK, w.Code)
}

func TestJWTMiddleware_InvalidatedToken(t *testing.T) {
	secret := "test-secret-key-at-least-32-bytes-long"
	userID := int64(1)
	username := "testuser"
	role := "admin"
	timeout := 24 * time.Hour

	token, err := GenerateToken(secret, userID, username, role, timeout)
	assert.NoError(t, err)

	// Validator that always invalidates
	validator := func(uid int64, tokenStr string, issuedAt time.Time) (bool, error) {
		return true, nil
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(testErrorHandler(), JWTMiddleware(secret, nil, validator))
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.NotEqual(t, http.StatusOK, w.Code)
}

func TestJWTMiddleware_InvalidSession(t *testing.T) {
	secret := "test-secret-key-at-least-32-bytes-long"
	userID := int64(1)
	username := "testuser"
	role := "admin"
	timeout := 24 * time.Hour

	token, err := GenerateToken(secret, userID, username, role, timeout)
	assert.NoError(t, err)

	// Session validator that always rejects
	sessionValidator := func(tokenStr string) (bool, error) {
		return false, nil
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(testErrorHandler(), JWTMiddleware(secret, sessionValidator))
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.NotEqual(t, http.StatusOK, w.Code)
}

func TestGenerateTOTPTempToken(t *testing.T) {
	secret := "test-secret-key-at-least-32-bytes-long"
	userID := int64(1)

	token, err := GenerateTOTPTempToken(secret, userID)
	assert.NoError(t, err)
	assert.NotEmpty(t, token)
}

func TestValidateTOTPTempToken_Valid(t *testing.T) {
	secret := "test-secret-key-at-least-32-bytes-long"
	userID := int64(1)

	token, err := GenerateTOTPTempToken(secret, userID)
	assert.NoError(t, err)

	validatedUserID, err := ValidateTOTPTempToken(secret, token)
	assert.NoError(t, err)
	assert.Equal(t, userID, validatedUserID)
}

func TestValidateTOTPTempToken_InvalidSecret(t *testing.T) {
	secret := "test-secret-key-at-least-32-bytes-long"
	wrongSecret := "wrong-secret-key-at-least-32-bytes-"
	userID := int64(1)

	token, err := GenerateTOTPTempToken(secret, userID)
	assert.NoError(t, err)

	_, err = ValidateTOTPTempToken(wrongSecret, token)
	assert.Error(t, err)
}
