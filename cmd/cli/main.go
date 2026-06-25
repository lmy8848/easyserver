package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"syscall"

	"easyserver/internal/config"
	"easyserver/internal/database"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/term"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	configPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	db, err := database.Init(cfg.Database.Path)
	if err != nil {
		log.Fatalf("init db: %v", err)
	}
	defer db.Close()

	// Get the subcommand (first non-flag argument)
	subcommand := os.Args[1]

	switch subcommand {
	case "reset-password":
		resetPasswordCmd(db)
	case "unlock":
		unlockCmd(db)
	case "reset-totp":
		resetTOTPCmd(db)
	case "show-admin":
		showAdminCmd(db)
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", subcommand)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("EasyServer CLI (single-admin mode)")
	fmt.Println()
	fmt.Println("Usage: easyserver-cli <command> [flags]")
	fmt.Println()
	fmt.Println("Flags:")
	fmt.Println("  -config string   path to config file (default \"config.yaml\")")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  reset-password   Reset admin password (prompts for new password)")
	fmt.Println("  unlock           Unlock admin account (clear lockout + login attempts)")
	fmt.Println("  reset-totp       Disable TOTP 2FA for admin (phone lost scenario)")
	fmt.Println("  show-admin       Show admin account status")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  easyserver-cli -config /opt/easyserver/config.yaml reset-password")
	fmt.Println("  easyserver-cli unlock")
	fmt.Println("  easyserver-cli reset-totp")
}

func readPassword(prompt string) (string, error) {
	fmt.Print(prompt)
	// Try to read password without echo
	if term.IsTerminal(int(syscall.Stdin)) {
		bytePassword, err := term.ReadPassword(int(syscall.Stdin))
		fmt.Println() // New line after password input
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(bytePassword)), nil
	}
	// Fallback: read from stdin (piped input)
	var password string
	_, err := fmt.Scanln(&password)
	return strings.TrimSpace(password), err
}

func readInput(prompt string) string {
	fmt.Print(prompt)
	var input string
	fmt.Scanln(&input)
	return strings.TrimSpace(input)
}

func resetPasswordCmd(db *sql.DB) {
	username := readInput("Username (default: admin): ")
	if username == "" {
		username = "admin"
	}

	// Check if user exists
	var exists int
	err := db.QueryRow("SELECT COUNT(*) FROM users WHERE username = ?", username).Scan(&exists)
	if err != nil || exists == 0 {
		log.Fatalf("user %q not found", username)
	}

	password, err := readPassword("New password: ")
	if err != nil {
		log.Fatalf("read password: %v", err)
	}
	if len(password) < 8 {
		log.Fatal("password must be at least 8 characters")
	}

	// Validate password strength
	if err := validatePasswordStrength(password); err != nil {
		log.Fatalf("weak password: %v", err)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Fatalf("hash: %v", err)
	}

	// Reset password + unlock + set must_change_pass
	result, err := db.Exec(
		`UPDATE users
		 SET password_hash = ?, must_change_pass = 1, login_attempts = 0,
		     locked_until = NULL, updated_at = CURRENT_TIMESTAMP
		 WHERE username = ?`,
		string(hash), username,
	)
	if err != nil {
		log.Fatalf("update: %v", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		log.Fatalf("user %q not found", username)
	}

	// Clear token blacklist and sessions
	db.Exec("DELETE FROM token_blacklist WHERE user_id IN (SELECT id FROM users WHERE username = ?)", username)
	db.Exec("DELETE FROM sessions WHERE user_id IN (SELECT id FROM users WHERE username = ?)", username)

	fmt.Printf("✓ Password reset for %q. Must change on next login.\n", username)
}

func unlockCmd(db *sql.DB) {
	username := readInput("Username (default: admin): ")
	if username == "" {
		username = "admin"
	}

	result, err := db.Exec(
		`UPDATE users SET locked_until = NULL, login_attempts = 0, updated_at = CURRENT_TIMESTAMP
		 WHERE username = ?`, username,
	)
	if err != nil {
		log.Fatalf("unlock: %v", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		log.Fatalf("user %q not found", username)
	}

	// Clear token blacklist
	db.Exec("DELETE FROM token_blacklist WHERE user_id IN (SELECT id FROM users WHERE username = ?)", username)

	fmt.Printf("✓ Account %q unlocked.\n", username)
}

func resetTOTPCmd(db *sql.DB) {
	username := readInput("Username (default: admin): ")
	if username == "" {
		username = "admin"
	}

	// Check if user exists and has TOTP enabled
	var totpEnabled int
	err := db.QueryRow("SELECT COALESCE(totp_enabled, 0) FROM users WHERE username = ?", username).Scan(&totpEnabled)
	if err != nil {
		log.Fatalf("user %q not found", username)
	}

	if totpEnabled == 0 {
		fmt.Printf("User %q does not have TOTP enabled. Nothing to do.\n", username)
		return
	}

	// Confirm dangerous operation
	fmt.Printf("⚠  This will DISABLE 2FA for %q.\n", username)
	confirm := readInput("Type 'yes' to continue: ")
	if confirm != "yes" {
		fmt.Println("Aborted.")
		return
	}

	result, err := db.Exec(
		`UPDATE users
		 SET totp_secret = '', totp_enabled = 0, totp_backup_codes = '[]', updated_at = CURRENT_TIMESTAMP
		 WHERE username = ?`, username,
	)
	if err != nil {
		log.Fatalf("reset totp: %v", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		log.Fatalf("user %q not found", username)
	}

	fmt.Printf("✓ TOTP disabled for %q. Login with password, then re-enable 2FA in settings.\n", username)
}

func showAdminCmd(db *sql.DB) {
	username := readInput("Username (default: admin): ")
	if username == "" {
		username = "admin"
	}

	var (
		id            int64
		role          string
		mustChange    int
		loginAttempts int
		lockedUntil   sql.NullString
		lastLogin     sql.NullString
		lastLoginIP   string
		totpEnabled   int
		ipWhitelist   string
		expiresAt     sql.NullString
	)
	err := db.QueryRow(
		`SELECT id, username, role, must_change_pass, login_attempts,
		        locked_until, last_login, last_login_ip, COALESCE(totp_enabled, 0),
		        ip_whitelist, expires_at
		 FROM users WHERE username = ?`, username,
	).Scan(&id, &username, &role, &mustChange, &loginAttempts,
		&lockedUntil, &lastLogin, &lastLoginIP, &totpEnabled,
		&ipWhitelist, &expiresAt)
	if err != nil {
		log.Fatalf("query: %v", err)
	}

	fmt.Println()
	fmt.Printf("User:        %s (id=%d, role=%s)\n", username, id, role)
	fmt.Printf("Locked:      %s\n", lockedOrNo(lockedUntil))
	fmt.Printf("Attempts:    %d\n", loginAttempts)
	fmt.Printf("MustChange:  %s\n", yesOrNo(mustChange))
	fmt.Printf("TOTP:        %s\n", yesOrNo(totpEnabled))
	fmt.Printf("LastLogin:   %s (IP: %s)\n", nullOrValue(lastLogin), lastLoginIP)
	fmt.Printf("IPWhitelist: %s\n", emptyOrValue(ipWhitelist, "(all allowed)"))
	fmt.Printf("ExpiresAt:   %s\n", nullOrValue(expiresAt))
	fmt.Println()
}

func lockedOrNo(ns sql.NullString) string {
	if !ns.Valid || ns.String == "" {
		return "No"
	}
	return fmt.Sprintf("Yes (until %s)", ns.String)
}

func yesOrNo(v int) string {
	if v == 0 {
		return "No"
	}
	return "Yes"
}

func nullOrValue(ns sql.NullString) string {
	if !ns.Valid || ns.String == "" {
		return "-"
	}
	return ns.String
}

func emptyOrValue(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

func validatePasswordStrength(password string) error {
	if len(password) < 8 {
		return fmt.Errorf("must be at least 8 characters")
	}
	if len(password) > 128 {
		return fmt.Errorf("must be less than 128 characters")
	}

	var hasUpper, hasLower, hasDigit bool
	for _, ch := range password {
		switch {
		case 'A' <= ch && ch <= 'Z':
			hasUpper = true
		case 'a' <= ch && ch <= 'z':
			hasLower = true
		case '0' <= ch && ch <= '9':
			hasDigit = true
		}
	}
	if !(hasUpper && hasLower && hasDigit) {
		return fmt.Errorf("must contain upper, lower case and digit")
	}
	return nil
}
