package qrlogin

import (
	"errors"
	"time"
)

// Sentinel errors for the confirm flow. The API layer maps these to HTTP
// responses (expired vs. already-confirmed vs. generic).
var (
	ErrNotPending = errors.New("二维码已失效或已确认")
	ErrExpired    = errors.New("二维码已过期")
)

// Status constants for a QR login session's lifecycle.
const (
	StatusPending   = "pending"   // QR displayed, waiting for mobile to scan+confirm
	StatusConfirmed = "confirmed" // Mobile confirmed; web_token issued, awaiting web pickup
	StatusCancelled = "cancelled" // User cancelled before confirmation
)

// QRLoginSession is a one-time scan-to-login session.
type QRLoginSession struct {
	ID          int64      `json:"id"`
	QRToken     string     `json:"qr_token"`
	Status      string     `json:"status"`
	UserID      int64      `json:"user_id"`
	WebToken    string     `json:"-"` // never serialize the issued web token
	UserJSON    string     `json:"-"` // {user, must_change_pass} for web pickup
	CreatedAt   time.Time  `json:"created_at"`
	ExpiresAt   time.Time  `json:"expires_at"`
	ConfirmedAt *time.Time `json:"confirmed_at,omitempty"`
}

// CreateResult is returned when a web client requests a new QR session. The
// QRCodeBase64 encodes "esqr:<qr_token>" and is rendered as an <img>.
type CreateResult struct {
	QRToken      string    `json:"qr_token"`
	QRCodeBase64 string    `json:"qr_code_base64"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// StatusResult is returned to the polling web client. On the first confirmed
// poll the web_token + user are returned and the session is consumed (deleted).
type StatusResult struct {
	Status         string    `json:"status"`
	ExpiresAt      time.Time `json:"expires_at"`
	Token          string    `json:"token,omitempty"`
	User           any       `json:"user,omitempty"`
	MustChangePass bool      `json:"must_change_pass,omitempty"`
}

// ConfirmRequest is the body the mobile app sends after scanning the QR.
type ConfirmRequest struct {
	QRToken string `json:"qr_token" binding:"required"`
}

// LoginPayload is the {user, must_change_pass} blob stored at confirm time and
// handed to the web client on pickup. It mirrors the password-login response
// shape (minus the token, which is carried separately as WebToken).
type LoginPayload struct {
	User           any  `json:"user"`
	MustChangePass bool `json:"must_change_pass"`
}
