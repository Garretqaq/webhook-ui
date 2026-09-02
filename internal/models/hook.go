package models

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"
)

type StringArray []string

func (s *StringArray) Scan(value interface{}) error {
	if value == nil {
		*s = []string{}
		return nil
	}
	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return errors.New("invalid type for StringArray")
	}
	if len(bytes) == 0 {
		*s = []string{}
		return nil
	}
	return json.Unmarshal(bytes, s)
}

func (s StringArray) Value() (driver.Value, error) {
	if len(s) == 0 {
		return "[]", nil
	}
	return json.Marshal(s)
}

type Hook struct {
	ID              string      `json:"id"`
	Name            string      `json:"name"`
	Command         string      `json:"command"`
	ScriptID        string      `json:"script_id"`
	SSHHostID       string      `json:"ssh_host_id"`
	WorkingDir      string      `json:"working_dir"`
	ResponseMessage string      `json:"response_message"`
	HMACSecret      string      `json:"hmac_secret,omitempty"`
	HMACAlgorithm   string      `json:"hmac_algorithm"`
	TriggerToken    string      `json:"trigger_token,omitempty"`
	PassArguments   StringArray `json:"pass_arguments"`
	PassHeaders     StringArray `json:"pass_headers"`
	PassPayloadTo   string      `json:"pass_payload_to"`
	// Async returns as soon as the execution is accepted instead of holding
	// the request open until the hook finishes.
	Async bool `json:"async"`
	// TimeoutSeconds bounds one execution; 0 means no limit.
	TimeoutSeconds int       `json:"timeout_seconds"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (h *Hook) Validate() error {
	if h.ID == "" {
		return errors.New("id is required")
	}
	if h.Name == "" {
		return errors.New("name is required")
	}
	if h.Command != "" && h.ScriptID != "" {
		return errors.New("command and script are mutually exclusive")
	}
	if h.Command == "" && h.ScriptID == "" {
		return errors.New("command or script is required")
	}
	if h.HMACSecret != "" && h.TriggerToken != "" {
		return errors.New("hmac_secret and trigger_token are mutually exclusive: pick one auth method")
	}
	if h.TimeoutSeconds < 0 {
		return errors.New("timeout_seconds must be 0 (no limit) or positive")
	}
	if !h.Async && h.TimeoutSeconds == 0 {
		// A synchronous hook holds the HTTP request open for as long as it
		// runs, so an unlimited one would pin a connection forever.
		return errors.New("only an async hook may have an unlimited timeout")
	}
	return nil
}

// Timeout renders TimeoutSeconds as a duration; 0 means no limit.
func (h *Hook) Timeout() time.Duration {
	return time.Duration(h.TimeoutSeconds) * time.Second
}

// ExecutionStatus values. queued and interrupted only occur for async hooks.
const (
	StatusQueued      = "queued"
	StatusRunning     = "running"
	StatusSuccess     = "success"
	StatusFailed      = "failed"
	StatusInterrupted = "interrupted"
	StatusCanceled    = "canceled"
	// StatusTimeout separates a run its time budget stopped from one that
	// failed on its own. Executions still record a timeout as failed; only
	// script test runs, which have no stored error to explain themselves, use
	// it today.
	StatusTimeout = "timeout"
)
