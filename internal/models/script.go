package models

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

var validInterpreters = []string{"bash", "sh", "python3", "powershell"}

// windowsInterpreters run only on Windows targets; everything else in
// validInterpreters runs only on Linux ones. A script is not bound to a host,
// so Validate accepts the union and the target check happens at execution.
var windowsInterpreters = []string{"powershell"}

func IsValidInterpreter(name string) bool {
	return contains(validInterpreters, name)
}

// IsInterpreterForOS reports whether the interpreter can run on targetOS.
func IsInterpreterForOS(name, targetOS string) bool {
	if targetOS == TargetOSWindows {
		return contains(windowsInterpreters, name)
	}
	return IsValidInterpreter(name) && !contains(windowsInterpreters, name)
}

func contains(list []string, s string) bool {
	for _, it := range list {
		if it == s {
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
