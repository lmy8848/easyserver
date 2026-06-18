package api

import (
	"database/sql"
	"strconv"
	"time"

	"easyserver/internal/service"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

type UserHandler struct {
	db             *sql.DB
	authService    *service.AuthService
	sessionService *service.SessionService
}

func NewUserHandler(db *sql.DB, authService *service.AuthService, sessionService *service.SessionService) *UserHandler {
	return &UserHandler{
		db:             db,
		authService:    authService,
		sessionService: sessionService,
	}
}

type UserResponse struct {
	ID           int64  `json:"id"`
	Username     string `json:"username"`
	Role         string `json:"role"`
	IsLocked     bool   `json:"is_locked"`
	IPWhitelist  string `json:"ip_whitelist"`
	LastLoginAt  string `json:"last_login_at"`
	CreatedAt    string `json:"created_at"`
}

type CreateUserRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required,min=6"`
	Role     string `json:"role" binding:"required,oneof=admin operator viewer"`
}

type UpdateUserRequest struct {
	Role     *string `json:"role"`
	IsLocked *bool   `json:"is_locked"`
}

// List returns all users (admin only)
func (h *UserHandler) List(c *gin.Context) {
	role := c.Query("role")

	query := "SELECT id, username, role, CASE WHEN locked_until IS NOT NULL AND locked_until > datetime('now') THEN 1 ELSE 0 END as is_locked, COALESCE(ip_whitelist, ''), last_login, created_at FROM users"
	args := []interface{}{}

	if role != "" {
		query += " WHERE role = ?"
		args = append(args, role)
	}

	query += " ORDER BY id ASC"

	rows, err := h.db.Query(query, args...)
	if err != nil {
		InternalError(c, "failed to query users")
		return
	}
	defer rows.Close()

	users := []UserResponse{}
	for rows.Next() {
		var u UserResponse
		var lastLogin sql.NullString
		err := rows.Scan(&u.ID, &u.Username, &u.Role, &u.IsLocked, &u.IPWhitelist, &lastLogin, &u.CreatedAt)
		if err != nil {
			continue
		}
		if lastLogin.Valid {
			u.LastLoginAt = lastLogin.String
		}
		users = append(users, u)
	}

	Success(c, users)
}

// Create creates a new user (admin only)
func (h *UserHandler) Create(c *gin.Context) {
	var req CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}

	// Check if username exists
	var count int
	err := h.db.QueryRow("SELECT COUNT(*) FROM users WHERE username = ?", req.Username).Scan(&count)
	if err != nil {
		InternalError(c, "failed to check username")
		return
	}
	if count > 0 {
		BadRequest(c, "username already exists")
		return
	}

	// Hash password
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		InternalError(c, "failed to hash password")
		return
	}

	// Insert user
	result, err := h.db.Exec(
		"INSERT INTO users (username, password_hash, role) VALUES (?, ?, ?)",
		req.Username, string(hash), req.Role,
	)
	if err != nil {
		InternalError(c, "failed to create user")
		return
	}

	id, _ := result.LastInsertId()

	Success(c, UserResponse{
		ID:       id,
		Username: req.Username,
		Role:     req.Role,
	})
}

// Update updates a user (admin only)
func (h *UserHandler) Update(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		BadRequest(c, "invalid user id")
		return
	}

	var req UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}

	// Check if user exists
	var exists int
	h.db.QueryRow("SELECT COUNT(*) FROM users WHERE id = ?", id).Scan(&exists)
	if exists == 0 {
		NotFound(c, "user not found")
		return
	}

	// Build update query
	updates := []string{}
	args := []interface{}{}

	if req.Role != nil {
		updates = append(updates, "role = ?")
		args = append(args, *req.Role)
	}
	if req.IsLocked != nil {
		if *req.IsLocked {
			updates = append(updates, "locked_until = datetime('now', '+1 year')")
			updates = append(updates, "login_attempts = 5")
			// Invalidate all user tokens
			if h.authService != nil {
				h.authService.InvalidateAllUserTokens(id)
			}
			// Remove all user sessions
			if h.sessionService != nil {
				h.sessionService.RemoveUserSessions(id)
			}
		} else {
			updates = append(updates, "locked_until = NULL")
			updates = append(updates, "login_attempts = 0")
		}
	}

	if len(updates) == 0 {
		BadRequest(c, "no fields to update")
		return
	}

	query := "UPDATE users SET "
	for i, u := range updates {
		if i > 0 {
			query += ", "
		}
		query += u
	}
	query += ", updated_at = datetime('now') WHERE id = ?"
	args = append(args, id)

	_, err = h.db.Exec(query, args...)
	if err != nil {
		InternalError(c, "failed to update user")
		return
	}

	Success(c, nil)
}

// Delete deletes a user (admin only)
func (h *UserHandler) Delete(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		BadRequest(c, "invalid user id")
		return
	}

	// Prevent deleting self
	currentUserID, _ := c.Get("user_id")
	if currentUserID.(int64) == id {
		BadRequest(c, "cannot delete yourself")
		return
	}

	// Check if user exists
	var exists int
	h.db.QueryRow("SELECT COUNT(*) FROM users WHERE id = ?", id).Scan(&exists)
	if exists == 0 {
		NotFound(c, "user not found")
		return
	}

	_, err = h.db.Exec("DELETE FROM users WHERE id = ?", id)
	if err != nil {
		InternalError(c, "failed to delete user")
		return
	}

	Success(c, nil)
}

// Unlock unlocks a user (admin only)
func (h *UserHandler) Unlock(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		BadRequest(c, "invalid user id")
		return
	}

	_, err = h.db.Exec(
		"UPDATE users SET locked_until = NULL, login_attempts = 0 WHERE id = ?",
		id,
	)
	if err != nil {
		InternalError(c, "failed to unlock user")
		return
	}

	// Clear token blacklist for this user
	h.db.Exec("DELETE FROM token_blacklist WHERE user_id = ?", id)

	Success(c, nil)
}

// ResetPassword resets a user's password (admin only)
func (h *UserHandler) ResetPassword(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		BadRequest(c, "invalid user id")
		return
	}

	var req struct {
		Password string `json:"password" binding:"required,min=6"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}

	// Check if user exists
	var exists int
	h.db.QueryRow("SELECT COUNT(*) FROM users WHERE id = ?", id).Scan(&exists)
	if exists == 0 {
		NotFound(c, "user not found")
		return
	}

	if err := h.authService.ResetPassword(id, req.Password); err != nil {
		InternalError(c, err.Error())
		return
	}

	// Log activity
	currentUserID, _ := c.Get("user_id")
	currentUsername, _ := c.Get("username")
	if currentUserID != nil && h.authService != nil {
		h.authService.LogUserActivity(
			currentUserID.(int64),
			currentUsername.(string),
			"RESET_PASSWORD",
			c.ClientIP(),
			c.Request.UserAgent(),
		)
	}

	Success(c, nil)
}

// GetActivities returns user activity log
func (h *UserHandler) GetActivities(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		BadRequest(c, "invalid user id")
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))

	activities, err := h.authService.GetUserActivities(id, limit)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, activities)
}

// GetAllActivities returns all user activities
func (h *UserHandler) GetAllActivities(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))

	activities, err := h.authService.GetAllActivities(limit)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, activities)
}

// SetExpiry sets account expiration date
func (h *UserHandler) SetExpiry(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		BadRequest(c, "invalid user id")
		return
	}

	var req struct {
		ExpiresAt *string `json:"expires_at"` // null to clear
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}

	// Check if user exists
	var exists int
	h.db.QueryRow("SELECT COUNT(*) FROM users WHERE id = ?", id).Scan(&exists)
	if exists == 0 {
		NotFound(c, "user not found")
		return
	}

	var expiresAt *time.Time
	if req.ExpiresAt != nil && *req.ExpiresAt != "" {
		t, err := time.Parse("2006-01-02T15:04:05Z", *req.ExpiresAt)
		if err != nil {
			BadRequest(c, "invalid date format")
			return
		}
		expiresAt = &t
	}

	if err := h.authService.SetAccountExpiry(id, expiresAt); err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, nil)
}

// SetIPWhitelist sets IP whitelist for a user
func (h *UserHandler) SetIPWhitelist(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		BadRequest(c, "invalid user id")
		return
	}

	var req struct {
		IPWhitelist string `json:"ip_whitelist"` // comma-separated IPs, empty to allow all
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}

	// Check if user exists
	var exists int
	h.db.QueryRow("SELECT COUNT(*) FROM users WHERE id = ?", id).Scan(&exists)
	if exists == 0 {
		NotFound(c, "user not found")
		return
	}

	if err := h.authService.SetIPWhitelist(id, req.IPWhitelist); err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, nil)
}

// GetSessions returns active sessions
func (h *UserHandler) GetSessions(c *gin.Context) {
	if h.sessionService == nil {
		Success(c, []interface{}{})
		return
	}

	sessions, err := h.sessionService.GetActiveSessions()
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, sessions)
}
