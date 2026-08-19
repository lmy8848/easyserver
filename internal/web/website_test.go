package web

import (
	"strings"
	"testing"
)

func TestValidateDomainName(t *testing.T) {
	tests := []struct {
		name      string
		domain    string
		expectErr bool
	}{
		{"valid domain", "example.com", false},
		{"valid subdomain", "app.example.com", false},
		{"valid multi-level", "a.b.c.example.com", false},
		{"valid with hyphen", "my-app.example-site.com", false},
		{"valid max length 253", strings.Repeat("a", 63) + "." + strings.Repeat("b", 63) + "." + strings.Repeat("c", 63) + "." + strings.Repeat("d", 57) + ".com", false},
		{"empty domain", "", true},
		{"too long 254", strings.Repeat("a", 63) + "." + strings.Repeat("b", 63) + "." + strings.Repeat("c", 63) + "." + strings.Repeat("d", 58) + ".com", true},
		{"invalid characters", "example$.com", true},
		{"leading hyphen", "-example.com", true},
		{"trailing hyphen", "example-.com", true},
		{"starts with dot", ".example.com", true},
		{"ends with dot", "example.com.", true},
		{"double dot", "example..com", true},
		{"contains space", "example .com", true},
		{"contains slash", "example.com/test", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDomainName(tt.domain)
			if (err != nil) != tt.expectErr {
				t.Errorf("ValidateDomainName(%q) error = %v, expectErr = %v", tt.domain, err, tt.expectErr)
			}
		})
	}
}

func TestUpdateWebsiteRequest_ValidateDomain(t *testing.T) {
	// nil domain is valid (no update)
	reqNil := UpdateWebsiteRequest{}
	if err := reqNil.ValidateDomain(); err != nil {
		t.Errorf("expected nil domain to be valid, got %v", err)
	}

	// valid domain
	validDom := "valid.domain.com"
	reqValid := UpdateWebsiteRequest{Domain: &validDom}
	if err := reqValid.ValidateDomain(); err != nil {
		t.Errorf("expected valid domain, got %v", err)
	}

	// invalid domain too long
	tooLongDom := strings.Repeat("a", 250) + ".com"
	reqTooLong := UpdateWebsiteRequest{Domain: &tooLongDom}
	if err := reqTooLong.ValidateDomain(); err == nil {
		t.Errorf("expected error for too long domain in update request")
	}
}
