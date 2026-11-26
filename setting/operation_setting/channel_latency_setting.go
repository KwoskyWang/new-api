package operation_setting

import (
	"math/rand"
	"os"
	"strings"

	"github.com/QuantumNous/new-api/setting/config"
)

// ChannelLatencySetting controls latency scoring that soft-tunes channel weights.
// 渠道延时打分配置：用最近 N 次通信的延时/成功率软调有效权重，不改 DB 人工 Weight。
type ChannelLatencySetting struct {
	// Enabled turns on collection + Redis window + soft weight tuning.
	Enabled bool `json:"enabled"`
	// DetailDBEnabled controls async detail write to LOG_DB (sizes + timings only, no body).
	DetailDBEnabled bool `json:"detail_db_enabled"`
	// DetailRetentionDays how long to keep detail rows; 0 defaults to 7.
	DetailRetentionDays int `json:"detail_retention_days"`
	// WindowSize is the max sliding window length per (model, channel).
	// Scoring always uses whatever samples are currently in the window (1..WindowSize).
	WindowSize int `json:"window_size"`
	// ScoreIntervalSeconds is how often scores are recomputed.
	ScoreIntervalSeconds int `json:"score_interval_seconds"`
	// MinSamples: routing uses the hot score once Samples >= this (default 1).
	// Window need not be full; score is based on the actual sample count.
	MinSamples int `json:"min_samples"`
	// SoftExcludeThreshold: score below this excludes channel from the candidate pool.
	// 0 disables soft exclude.
	SoftExcludeThreshold float64 `json:"soft_exclude_threshold"`
	// ProbeIntervalSeconds: how often to probe quarantined (model, channel) pairs.
	// Consecutive fails ≥ threshold → quarantine; probe with max_tokens=1; success → recover.
	ProbeIntervalSeconds int `json:"probe_interval_seconds"`
	// ProbeFailThreshold: consecutive traffic failures required before entering probe pool.
	// 1 = quarantine on first fail; 2 (default) ignores single transient errors.
	ProbeFailThreshold int `json:"probe_fail_threshold"`
	// RoutingCacheSeconds: local TTL for score / probe lookups on the select path (0 disables).
	RoutingCacheSeconds int `json:"routing_cache_seconds"`
	// TraceSamplePercent: percent of scored requests that attach httptrace + write timing window.
	// Probe last-result always updates. 100 = every request; diagnostics remain 100%.
	TraceSamplePercent int `json:"trace_sample_percent"`
}

var channelLatencySetting = ChannelLatencySetting{
	Enabled:              false,
	DetailDBEnabled:      false,
	DetailRetentionDays:  7,
	WindowSize:           100,
	ScoreIntervalSeconds: 60,
	MinSamples:           1,
	SoftExcludeThreshold: 0.15,
	ProbeIntervalSeconds: 300,
	ProbeFailThreshold:   2,
	RoutingCacheSeconds:  2,
	TraceSamplePercent:   10,
}

func init() {
	config.GlobalConfig.Register("channel_latency_setting", &channelLatencySetting)
}

// ApplyChannelLatencyEnvBootstrap applies CHANNEL_LATENCY_* env after godotenv.Load.
// Call from startup (after .env is loaded). Option API / DB sync can still override later.
func ApplyChannelLatencyEnvBootstrap() {
	if envTruthy("CHANNEL_LATENCY_ENABLED") {
		channelLatencySetting.Enabled = true
	}
	if envTruthy("CHANNEL_LATENCY_DETAIL_DB_ENABLED") {
		channelLatencySetting.DetailDBEnabled = true
	}
}

func envTruthy(key string) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

func GetChannelLatencySetting() *ChannelLatencySetting {
	return &channelLatencySetting
}

func IsChannelLatencyScoringEnabled() bool {
	return channelLatencySetting.Enabled
}

func IsChannelLatencyDetailDBEnabled() bool {
	return channelLatencySetting.Enabled && channelLatencySetting.DetailDBEnabled
}

// ShouldSampleChannelLatencyTrace decides whether this request attaches httptrace
// and writes a timing-window sample. Probe last-result updates are independent.
func ShouldSampleChannelLatencyTrace() bool {
	if !channelLatencySetting.Enabled {
		return false
	}
	p := channelLatencySetting.TraceSamplePercent
	if p >= 100 {
		return true
	}
	if p <= 0 {
		return false
	}
	return rand.Intn(100) < p
}
