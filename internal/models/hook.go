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
	WorkingDir      string      `json:"working_dir"`
	ResponseMessage string      `json:"response_message"`
	HMACSecret      string      `json:"hmac_secret,omitempty"`
	HMACAlgorithm   string      `json:"hmac_algorithm"`
	TriggerToken    string      `json:"trigger_token,omitempty"`
	PassArguments   StringArray `json:"pass_arguments"`
	PassHeaders     StringArray `json:"pass_headers"`
	PassPayloadTo   string      `json:"pass_payload_to"`
	CreatedAt       time.Time   `json:"created_at"`
	UpdatedAt       time.Time   `json:"updated_at"`
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
	return nil
}
