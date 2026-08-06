package models

import (
	"errors"
	"time"
)

const (
	SSHAuthKey      = "key"
	SSHAuthPassword = "password"
)

// TargetOS selects how remote commands are built: POSIX shell syntax for
// Linux, cmd.exe syntax for Windows (the default shell of Windows OpenSSH).
const (
	TargetOSLinux   = "linux"
	TargetOSWindows = "windows"
)

type SSHHost struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Host       string    `json:"host"`
	Port       int       `json:"port"`
	User       string    `json:"user"`
	AuthType   string    `json:"auth_type"`
	TargetOS   string    `json:"target_os"`
	Credential string    `json:"credential,omitempty"`
	HostKey    string    `json:"host_key,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (h *SSHHost) Validate() error {
	if h.Name == "" {
		return errors.New("name is required")
	}
	if h.Host == "" {
		return errors.New("host is required")
	}
	if h.Port <= 0 || h.Port > 65535 {
		return errors.New("port must be between 1 and 65535")
	}
	if h.User == "" {
		return errors.New("user is required")
	}
	if h.AuthType != SSHAuthKey && h.AuthType != SSHAuthPassword {
		return errors.New("auth_type must be key or password")
	}
	if h.TargetOS != TargetOSLinux && h.TargetOS != TargetOSWindows {
		return errors.New("target_os must be linux or windows")
	}
	if h.Credential == "" {
		return errors.New("credential is required")
	}
	return nil
}
