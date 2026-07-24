package config

import (
	"crypto/rand"
	"encoding/base64"
	"os"
	"strings"
)

type Config struct {
	Port            string
	DataDir         string
	SessionSecret   string
	AdminPassword   string
	AllowedCommands []string
}

func Load() *Config {
	cfg := &Config{
		Port:            getEnv("PORT", "9000"),
		DataDir:         getEnv("DATA_DIR", "./data"),
		AdminPassword:   getEnv("ADMIN_PASSWORD", ""),
		AllowedCommands: getEnvSlice("ALLOWED_COMMANDS", []string{"/usr/bin/git", "/usr/bin/curl", "/bin/bash", "/bin/sh"}),
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
