package channel_latency

import "time"

// Sample is one upstream channel attempt (sizes + timings, no body content).
// Group is request context only (for DB analytics); hot window/score keys are (model, channel).
type Sample struct {
	Timestamp   time.Time `json:"ts"`
	RequestID   string    `json:"request_id,omitempty"`
	Group       string    `json:"group,omitempty"`
	Model       string    `json:"model"`
	ChannelID   int       `json:"channel_id"`
	ChannelType int       `json:"channel_type,omitempty"`
	RetryIndex  int       `json:"retry_index"`
	Success     bool      `json:"success"`
	StatusCode  int       `json:"status_code,omitempty"`
	ErrorStage  string    `json:"error_stage,omitempty"`
	ReqBytes    int64     `json:"req_bytes"`
	RespBytes   int64     `json:"resp_bytes"`
	TSendMs     int64     `json:"t_send_ms"`
	TWaitMs     int64     `json:"t_wait_ms"`
	TRecvMs     int64     `json:"t_recv_ms"`
	TTotalMs    int64     `json:"t_total_ms"`
}

// Score is the hot score published for routing, keyed by (model, channel).
type Score struct {
	Model       string  `json:"model"`
	ChannelID   int     `json:"channel_id"`
	Samples     int     `json:"samples"`
	SuccessRate float64 `json:"success_rate"`
	P50WaitMs   int64   `json:"p50_wait_ms"`
	P95WaitMs   int64   `json:"p95_wait_ms"`
	P95TotalMs  int64   `json:"p95_total_ms"`
	Score       float64 `json:"score"` // (0,1]
	UpdatedAt   int64   `json:"updated_at"`
}
