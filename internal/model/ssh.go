package model

import "easyserver/internal/ssh"

// Domain types moved to easyserver/internal/ssh.
// Kept as aliases for backward compatibility.

type SSHConfig = ssh.Config
type SSHSession = ssh.Session
type SSHLoginRecord = ssh.LoginRecord
type SSHKey = ssh.Key
