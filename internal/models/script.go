package models

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

var validInterpreters = []string{"bash", "sh", "python3"}

func IsValidInterpreter(name string) bool {
	for _, it := range validInterpreters {
		if name == it {
			return true
		}
	}
	return false
}

type Script struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Interpreter string    `json:"interpreter"`
	Content     string    `json:"content"`
	Description string    `json:"description"`
	SSHHostID   string    `json:"ssh_host_id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (s *Script) Validate() error {
	if s.Name == "" {
		return errors.New("name is required")
	}
	if !IsValidInterpreter(s.Interpreter) {
		return fmt.Errorf("interpreter must be one of: %s", strings.Join(validInterpreters, ", "))
	}
	if s.Content == "" {
		return errors.New("content is required")
	}
	return nil
}
