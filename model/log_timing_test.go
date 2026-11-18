package model

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
)

// TestFormatUserLogsStripsRelayTiming 验证普通用户可见的日志经 formatUserLogs 处理后，
// admin_info（含 relay_timing 诊断信息）被完整剥离，而普通字段保持不变。
// 这是"仅管理员可见诊断信息"后端强制剥离的核心保证，不能只靠前端隐藏。
func TestFormatUserLogsStripsRelayTiming(t *testing.T) {
	other := map[string]interface{}{
		"model_ratio": 2.0,
		"frt":         float64(1200),
		"admin_info": map[string]interface{}{
			"use_channel": []interface{}{1, 2},
			"relay_timing": map[string]interface{}{
				"local_total_ms":      float64(50),
				"local_to_upstream_ms": float64(40),
				"attempts": []interface{}{
					map[string]interface{}{
						"index":        0,
						"wait_resp_ms": float64(2000),
						"protocol":     "HTTP/2.0",
					},
				},
			},
		},
		"stream_status": map[string]interface{}{"status": "ok"},
	}

	logs := []*Log{
		{Other: common.MapToJsonStr(other), ChannelName: "secret-channel"},
	}

	formatUserLogs(logs, 0)

	got := logs[0].Other
	if strings.Contains(got, "relay_timing") {
		t.Fatalf("relay_timing must be stripped for user logs, got: %s", got)
	}
	if strings.Contains(got, "admin_info") {
		t.Fatalf("admin_info must be stripped for user logs, got: %s", got)
	}
	if strings.Contains(got, "stream_status") {
		t.Fatalf("stream_status must be stripped for user logs, got: %s", got)
	}
	if strings.Contains(got, "wait_resp_ms") || strings.Contains(got, "HTTP/2.0") {
		t.Fatalf("timing detail leaked into user logs: %s", got)
	}
	// 普通字段必须保留。
	parsed, err := common.StrToMap(got)
	if err != nil {
		t.Fatalf("failed to parse formatted other: %v", err)
	}
	if _, ok := parsed["model_ratio"]; !ok {
		t.Fatal("model_ratio should be preserved for user logs")
	}
	if _, ok := parsed["frt"]; !ok {
		t.Fatal("frt should be preserved for user logs")
	}
	// ChannelName 对普通用户应清空。
	if logs[0].ChannelName != "" {
		t.Fatalf("channel name should be cleared for user logs, got %q", logs[0].ChannelName)
	}
}

// TestFormatUserLogsHandlesLegacyLogs 验证旧日志（无 admin_info）不受影响。
func TestFormatUserLogsHandlesLegacyLogs(t *testing.T) {
	logs := []*Log{
		{Other: `{"model_ratio":2.0,"frt":1000}`},
		{Other: ""},
	}
	formatUserLogs(logs, 0)
	if !strings.Contains(logs[0].Other, "model_ratio") {
		t.Fatalf("legacy log fields should be preserved, got %s", logs[0].Other)
	}
}
