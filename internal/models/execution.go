package models

import "time"

type Execution struct {
	ID            int64      `json:"id"`
	HookID        string     `json:"hook_id"`
	TriggerSource string     `json:"trigger_source"`
	Status        string     `json:"status"`
	Output        string     `json:"output"`
	Error         string     `json:"error"`
	StartedAt     time.Time  `json:"started_at"`
	FinishedAt    *time.Time `json:"finished_at,omitempty"`
}
