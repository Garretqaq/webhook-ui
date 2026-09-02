package models

import "time"

type Execution struct {
	ID            int64      `json:"id"`
	HookID        string     `json:"hook_id"`
	TriggerSource string     `json:"trigger_source"`
	ExecTarget    string     `json:"exec_target"`
	Status        string     `json:"status"`
	// Output can be megabytes (LOG_TAIL_BYTES caps it at 5MB per stream), so
	// list responses leave it out — the omitempty json tag only skips an empty
	// string, hence the pointer. Get (detail) fills it in.
	Output        *string    `json:"output,omitempty"`
	Error         string     `json:"error"`
	StartedAt     time.Time  `json:"started_at"`
	FinishedAt    *time.Time `json:"finished_at,omitempty"`
}

// ExecutionLogChunk is one slice of a running execution's output, in the order
// it was read. Seq is unique per execution and never reused, so it stays a
// valid cursor after older chunks are rolled off.
type ExecutionLogChunk struct {
	Seq    int64  `json:"seq"`
	Stream string `json:"stream"`
	Text   string `json:"text"`
}
