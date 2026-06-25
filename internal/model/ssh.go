package model

// SSHConfig represents SSH server configuration
type SSHConfig struct {
	Port                   int    `json:"port"`
	PermitRootLogin        string `json:"permit_root_login"`
	PasswordAuthentication string `json:"password_auth"`
	PubkeyAuthentication   string `json:"pubkey_auth"`
	MaxAuthTries           int    `json:"max_auth_tries"`
	LoginGraceTime         int    `json:"login_grace_time"`
	ClientAliveInterval    int    `json:"client_alive_interval"`
	ClientAliveCountMax    int    `json:"client_alive_count_max"`
	AllowUsers             string `json:"allow_users"`
	DenyUsers              string `json:"deny_users"`
}

// SSHSession represents an active SSH session
type SSHSession struct {
	PID       int    `json:"pid"`
	User      string `json:"user"`
	TTY       string `json:"tty"`
	From      string `json:"from"`
	LoginTime string `json:"login_time"`
	Command   string `json:"command"`
	Type      string `json:"type"` // interactive, non-interactive, ssh
}

// SSHLoginRecord represents an SSH login attempt
type SSHLoginRecord struct {
	Time   string `json:"time"`
	User   string `json:"user"`
	IP     string `json:"ip"`
	Port   int    `json:"port"`
	Status string `json:"status"` // success, failed
	Method string `json:"method"` // password, publickey
	TTY    string `json:"tty"`
}

// SSHKey represents an SSH public key
type SSHKey struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	KeyType     string `json:"key_type"`
	PublicKey   string `json:"public_key"`
	Fingerprint string `json:"fingerprint"`
	CreatedAt   string `json:"created_at"`
}
