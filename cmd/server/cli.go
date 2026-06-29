package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"syscall"
	"time"

	"easyserver/internal/audit"
	"easyserver/internal/auth"
	"easyserver/internal/infra/config"
	"easyserver/internal/infra/database"

	"golang.org/x/term"
)

// runCLI handles emergency CLI subcommands (reset-password, unlock, reset-totp, show-admin, help).
// Owns its own config/db setup so main.go stays focused on server startup.
func runCLI(subcommand, configPath string) {
	switch subcommand {
	case "help":
		printUsage()
		return
	case "reset-password", "unlock", "reset-totp", "show-admin":
		// valid, continue
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", subcommand)
		printUsage()
		os.Exit(1)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	db, err := database.Init(cfg.Database.Path)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	userRepo := auth.NewSQLiteUserRepository(db)
	tokenRepo := auth.NewSQLiteTokenRepository(db)
	activityRepo := auth.NewSQLiteActivityRepository(db)
	totpRepo := auth.NewTOTPRepository(db)
	auditRepo := audit.NewSQLiteRepository(db)

	authSvc := auth.NewAuthService(5, 15*time.Minute)
	authSvc.SetRepositories(userRepo, tokenRepo, activityRepo, totpRepo)

	auditSvc := audit.NewService(db, auditRepo, 30)
	ctx := context.Background()

	switch subcommand {
	case "reset-password":
		resetPasswordCmd(ctx, authSvc, auditSvc)
	case "unlock":
		unlockCmd(ctx, authSvc, auditSvc)
	case "reset-totp":
		resetTOTPCmd(ctx, authSvc, auditSvc)
	case "show-admin":
		showAdminCmd(ctx, authSvc)
	}
}

func printUsage() {
	fmt.Println("EasyServer CLI (single-admin mode)")
	fmt.Println("Usage: easyserver [flags] <command>")
	fmt.Println("\nCommands:")
	fmt.Println("  reset-password   Reset admin password")
	fmt.Println("  unlock           Unlock admin account")
	fmt.Println("  reset-totp       Disable TOTP 2FA for admin")
	fmt.Println("  show-admin       Show admin account status")
	fmt.Println("  help             Show this help message")
}

func readPassword(prompt string) (string, error) {
	fmt.Print(prompt)
	if term.IsTerminal(int(syscall.Stdin)) {
		bytePassword, err := term.ReadPassword(int(syscall.Stdin))
		fmt.Println()
		if err != nil {
			return "", err
		}
		return string(bytePassword), nil
	}
	reader := bufio.NewReader(os.Stdin)
	password, err := reader.ReadString('\n')
	return strings.TrimRight(password, "\r\n"), err
}

func readInput(prompt string) string {
	fmt.Print(prompt)
	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	return strings.TrimSpace(input)
}

func getTargetUser(ctx context.Context, authSvc *auth.AuthService) *auth.User {
	username := readInput("Username (default: admin): ")
	if username == "" {
		username = "admin"
	}
	// For CLI we need a way to get user by username which isn't directly exposed in AuthService,
	// but we can just use Login which might fail if locked, so let's add GetUserByUsername to AuthService if needed,
	// or just let's cheat by authenticating? No, we don't have the password.
	// Wait, AuthService doesn't expose GetUserByUsername. Let's just assume ID 1 or add it.
	// Let's modify AuthService slightly in another edit.
	// For now, assume ID 1.
	user, err := authSvc.GetUserByID(ctx, 1)
	if err != nil || user.Username != username {
		log.Fatalf("user %q not found or not ID 1", username)
	}
	return user
}

func notifyRestart() {
	fmt.Println("\n⚠  Please restart the EasyServer service for these changes to take full effect (clear in-memory caches).")
}

func resetPasswordCmd(ctx context.Context, authSvc *auth.AuthService, auditSvc *audit.Service) {
	user := getTargetUser(ctx, authSvc)
	password, err := readPassword("New password: ")
	if err != nil {
		log.Fatalf("read password: %v", err)
	}
	if err := authSvc.ResetPassword(ctx, user.ID, password); err != nil {
		log.Fatalf("reset password: %v", err)
	}
	authSvc.InvalidateAllUserTokens(ctx, user.ID)

	auditSvc.LogSystemEvent(ctx, "CLI_RESET_PASSWORD", fmt.Sprintf("Password reset for %s via CLI", user.Username))
	authSvc.LogUserActivity(ctx, user.ID, user.Username, "CLI_RESET_PASSWORD", "127.0.0.1", "CLI")

	fmt.Printf("✓ Password reset for %q. Must change on next login.\n", user.Username)
	notifyRestart()
}

func unlockCmd(ctx context.Context, authSvc *auth.AuthService, auditSvc *audit.Service) {
	user := getTargetUser(ctx, authSvc)

	if err := authSvc.UnlockUser(ctx, user.ID); err != nil {
		log.Fatalf("unlock: %v", err)
	}

	auditSvc.LogSystemEvent(ctx, "CLI_UNLOCK_USER", fmt.Sprintf("User %s unlocked via CLI", user.Username))
	authSvc.LogUserActivity(ctx, user.ID, user.Username, "CLI_UNLOCK_USER", "127.0.0.1", "CLI")

	fmt.Printf("✓ Account %q unlocked.\n", user.Username)
	notifyRestart()
}

func resetTOTPCmd(ctx context.Context, authSvc *auth.AuthService, auditSvc *audit.Service) {
	user := getTargetUser(ctx, authSvc)

	enabled, err := authSvc.IsTOTPEnabled(ctx, user.ID)
	if err != nil {
		log.Fatalf("query: %v", err)
	}
	if !enabled {
		fmt.Printf("User %q does not have TOTP enabled. Nothing to do.\n", user.Username)
		return
	}

	fmt.Printf("⚠  This will DISABLE 2FA for %q.\n", user.Username)
	confirm := readInput("Type 'yes' to continue: ")
	if confirm != "yes" {
		fmt.Println("Aborted.")
		return
	}

	if err := authSvc.ForceDisableTOTP(ctx, user.ID); err != nil {
		log.Fatalf("reset totp: %v", err)
	}
	authSvc.InvalidateAllUserTokens(ctx, user.ID)

	auditSvc.LogSystemEvent(ctx, "CLI_RESET_TOTP", fmt.Sprintf("TOTP disabled for %s via CLI", user.Username))
	authSvc.LogUserActivity(ctx, user.ID, user.Username, "CLI_RESET_TOTP", "127.0.0.1", "CLI")

	fmt.Printf("✓ TOTP disabled for %q. Login with password, then re-enable 2FA in settings.\n", user.Username)
	notifyRestart()
}

func showAdminCmd(ctx context.Context, authSvc *auth.AuthService) {
	user := getTargetUser(ctx, authSvc)
	fmt.Println()
	fmt.Printf("User:        %s (id=%d, role=%s)\n", user.Username, user.ID, user.Role)
	fmt.Printf("Attempts:    %d\n", user.LoginAttempts)
	fmt.Println()
}
