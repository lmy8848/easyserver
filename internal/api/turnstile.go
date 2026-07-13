package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"time"
)

// turnstileVerifyEndpoint is Cloudflare's siteverify API.
const turnstileVerifyEndpoint = "https://challenges.cloudflare.com/turnstile/v0/siteverify"

// turnstileTimeout bounds the outbound verification request.
const turnstileTimeout = 8 * time.Second

// siteverifyResponse mirrors Cloudflare's siteverify reply.
type siteverifyResponse struct {
	Success    bool     `json:"success"`
	ErrorCodes []string `json:"error-codes,omitempty"`
}

// TurnstileVerifier checks a Turnstile response token with Cloudflare.
type TurnstileVerifier interface {
	Verify(ctx context.Context, secret, token, remoteIP string) bool
}

// verifier is the process-wide Turnstile verifier. Tests may swap it.
var verifier TurnstileVerifier = newHTTPTurnstileVerifier()

// httpTurnstileVerifier implements TurnstileVerifier using a package-level
// *http.Client so real calls share one client.
type httpTurnstileVerifier struct {
	httpClient *http.Client
}

func newHTTPTurnstileVerifier() *httpTurnstileVerifier {
	return &httpTurnstileVerifier{
		httpClient: &http.Client{Timeout: turnstileTimeout},
	}
}

// Verify checks a Turnstile response token with Cloudflare. It returns true
// only when Cloudflare reports success. Network/parse failures are treated as
// a failure to verify (closed), callers may decide to hard- or soft-fail.
func (v *httpTurnstileVerifier) Verify(ctx context.Context, secret, token, remoteIP string) bool {
	if secret == "" {
		// No secret configured -> nothing to verify against; treat as valid so
		// the panel still works when Turnstile is unconfigured.
		return true
	}
	if token == "" {
		return false
	}

	form := url.Values{}
	form.Set("secret", secret)
	form.Set("response", token)
	if remoteIP != "" {
		form.Set("remoteip", remoteIP)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, turnstileVerifyEndpoint, nil)
	if err != nil {
		return false
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Body = io.NopCloser(bytes.NewReader([]byte(form.Encode())))

	resp, err := v.httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if err != nil {
		return false
	}
	var sr siteverifyResponse
	if err := json.Unmarshal(body, &sr); err != nil {
		return false
	}
	return sr.Success
}
