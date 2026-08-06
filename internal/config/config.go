package config

import (
	"crypto/rand"
	"encoding/base64"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Port                 string
	DataDir              string
	SessionSecret        string
	AdminUsername        string
	AdminPassword        string
	TrustedProxies       []string
	LoginMaxFailures     int
	LoginLockoutMinutes  int
	LoginRateLimitPerMin int
	AllowedCommands      []string
	// LogTailBytes caps how much of one execution's output is retained, per
	// stream for the aggregate and in total for the streamed log rows. The
	// head is dropped, since the tail is what explains a failure.
	LogTailBytes int
}

func Load() *Config {
	cfg := &Config{
		Port:                 getEnv("PORT", "9000"),
		DataDir:              getEnv("DATA_DIR", "./data"),
		AdminUsername:        getEnv("ADMIN_USERNAME", "admin"),
		AdminPassword:        getEnv("ADMIN_PASSWORD", ""),
		TrustedProxies:       getEnvSlice("TRUSTED_PROXIES", []string{"127.0.0.1"}),
		LoginMaxFailures:     getEnvInt("LOGIN_MAX_FAILURES", 5),
		LoginLockoutMinutes:  getEnvInt("LOGIN_LOCKOUT_MINUTES", 15),
		LoginRateLimitPerMin: getEnvInt("LOGIN_RATE_LIMIT_PER_MIN", 10),
		AllowedCommands:      getEnvSlice("ALLOWED_COMMANDS", []string{"/usr/bin/git", "/usr/bin/curl", "/bin/bash", "/bin/sh", "/usr/bin/python3"}),
		LogTailBytes:         getEnvInt("LOG_TAIL_BYTES", 5*1024*1024),
	}

	cfg.SessionSecret = os.Getenv("SESSION_SECRET")
	if cfg.SessionSecret == "" {
		cfg.SessionSecret = generateRandomSecret()
	}

	return cfg
}

func getEnv(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return defaultValue
}

func getEnvSlice(key string, defaultValue []string) []string {
	if v := os.Getenv(key); v != "" {
		parts := strings.Split(v, ",")
		var result []string
		for _, p := range parts {
			if trimmed := strings.TrimSpace(p); trimmed != "" {
				result = append(result, trimmed)
			}
		}
		return result
	}
	return defaultValue
}

func generateRandomSecret() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.StdEncoding.EncodeToString(b)
}
