package relaytiming

import (
	"testing"
	"time"
)

// TestLocalPhasesAndTotal 验证本地处理阶段记录与总耗时语义。
func TestLocalPhasesAndTotal(t *testing.T) {
	enter := time.Now().Add(-100 * time.Millisecond)
	rt := New(enter)

	rt.AddLocalPhase("auth_parse", 10*time.Millisecond)
	rt.AddLocalPhase("sensitive_check", 5*time.Millisecond)

	snap := rt.Snapshot()
	if len(snap.LocalPhases) != 2 {
		t.Fatalf("expected 2 local phases, got %d", len(snap.LocalPhases))
	}
	if snap.LocalPhases[0].Name != "auth_parse" || snap.LocalPhases[0].DurationMs != 10 {
		t.Fatalf("unexpected first phase: %+v", snap.LocalPhases[0])
	}
	if snap.LocalTotalMs < 0 {
		t.Fatalf("local total should be recorded, got %d", snap.LocalTotalMs)
	}
}

// TestLocalTotalNoRecordWhenEnterZero 验证应用进入时间不可用时总耗时为 -1（无记录）。
func TestLocalTotalNoRecordWhenEnterZero(t *testing.T) {
	rt := New(time.Time{})
	snap := rt.Snapshot()
	if snap.LocalTotalMs != -1 {
		t.Fatalf("expected -1 (no record) when enter time missing, got %d", snap.LocalTotalMs)
	}
	if snap.LocalToUpstreamMs != -1 {
		t.Fatalf("expected -1 local_to_upstream when enter time missing, got %d", snap.LocalToUpstreamMs)
	}
}

// TestNegativeDurationClampedToZero 验证不产生误导性的负耗时。
func TestNegativeDurationClampedToZero(t *testing.T) {
	rt := New(time.Now())
	rt.AddLocalPhase("weird", -50*time.Millisecond)
	snap := rt.Snapshot()
	if len(snap.LocalPhases) != 1 || snap.LocalPhases[0].DurationMs != 0 {
		t.Fatalf("negative duration should clamp to 0, got %+v", snap.LocalPhases)
	}
}

// TestAttemptHeaderBeforeDataDistinct 验证"响应头先返回、SSE data 延迟返回"能区分两个阶段。
func TestAttemptHeaderBeforeDataDistinct(t *testing.T) {
	rt := New(time.Now())
	at := rt.NewAttempt(0, 1, 1)

	start := time.Now()
	at.requestStart = start
	at.hasStart = true
	at.gotConnTime = start
	at.wroteRequestTime = start.Add(10 * time.Millisecond)
	at.gotFirstByteTime = start.Add(30 * time.Millisecond)  // 响应头首字节
	at.responseReturnedTime = start.Add(31 * time.Millisecond)
	// SSE 第一条有效 data 比响应头晚很多（模型思考）
	at.firstDataTime = start.Add(2030 * time.Millisecond)
	at.firstDataKind = FirstDataKindFirstData

	snap := rt.Snapshot().Attempts[0]
	if snap.WaitRespMs == nil {
		t.Fatal("wait_resp_ms should be recorded")
	}
	if snap.FirstDataMs == nil {
		t.Fatal("first_data_ms should be recorded")
	}
	// 响应头等待 ~20ms，而首条 data 相对写出完成 ~2020ms，两者必须明显不同。
	if *snap.WaitRespMs >= *snap.FirstDataMs {
		t.Fatalf("wait_resp_ms (%d) should be much smaller than first_data_ms (%d)", *snap.WaitRespMs, *snap.FirstDataMs)
	}
	if snap.FirstDataKind != FirstDataKindFirstData {
		t.Fatalf("first_data_kind should be %q, got %q", FirstDataKindFirstData, snap.FirstDataKind)
	}
}

// TestReusedConnectionNoDNSTLS 验证连接复用时，未发生的 DNS/TLS 不被误记。
func TestReusedConnectionNoDNSTLS(t *testing.T) {
	rt := New(time.Now())
	at := rt.NewAttempt(0, 1, 1)

	start := time.Now()
	at.requestStart = start
	at.hasStart = true
	at.getConnTime = start
	at.gotConnTime = start.Add(1 * time.Millisecond)
	at.connReused = true
	at.connReusedSet = true
	// 不触发任何 DNS/TCP/TLS 事件
	at.wroteRequestTime = start.Add(2 * time.Millisecond)
	at.gotFirstByteTime = start.Add(5 * time.Millisecond)

	snap := rt.Snapshot().Attempts[0]
	if snap.DNSMs != nil {
		t.Fatalf("DNS should be nil (no record) on reused conn, got %v", *snap.DNSMs)
	}
	if snap.TLSMs != nil {
		t.Fatalf("TLS should be nil (no record) on reused conn, got %v", *snap.TLSMs)
	}
	if snap.TCPMs != nil {
		t.Fatalf("TCP should be nil (no record) on reused conn, got %v", *snap.TCPMs)
	}
	if snap.ConnReused == nil || !*snap.ConnReused {
		t.Fatal("conn_reused should be true")
	}
}

// TestRealZeroVsNoRecord 验证真实 0ms 与"无记录"的区别。
func TestRealZeroVsNoRecord(t *testing.T) {
	rt := New(time.Now())
	at := rt.NewAttempt(0, 1, 1)

	start := time.Now()
	at.getConnTime = start
	at.gotConnTime = start // 同一时刻 -> 真实 0ms

	snap := rt.Snapshot().Attempts[0]
	if snap.GetConnMs == nil {
		t.Fatal("get_conn_ms should be a real 0, not nil")
	}
	if *snap.GetConnMs != 0 {
		t.Fatalf("expected real 0ms, got %d", *snap.GetConnMs)
	}
	// 从未发生的字段应为 nil
	if snap.WaitRespMs != nil {
		t.Fatal("wait_resp_ms should be nil when not measured")
	}
}

// TestWriteFailurePreservesTimingAndStage 验证请求写出失败时保留已完成计时与错误阶段。
func TestWriteFailurePreservesTimingAndStage(t *testing.T) {
	rt := New(time.Now())
	at := rt.NewAttempt(0, 1, 1)

	start := time.Now()
	at.requestStart = start
	at.hasStart = true
	at.getConnTime = start
	at.gotConnTime = start.Add(2 * time.Millisecond) // 已拿到连接
	// 写出未完成即失败
	at.MarkError(ErrorStageWriteReq)

	snap := rt.Snapshot().Attempts[0]
	if snap.GetConnMs == nil {
		t.Fatal("get_conn_ms should be preserved even on failure")
	}
	if snap.ErrorStage != ErrorStageWriteReq {
		t.Fatalf("expected error stage %q, got %q", ErrorStageWriteReq, snap.ErrorStage)
	}
	if snap.WriteMs != nil {
		t.Fatal("write_ms should be nil since write never completed")
	}
}

// TestWaitResponseFailurePreservesStage 验证响应等待失败时保留错误阶段。
func TestWaitResponseFailurePreservesStage(t *testing.T) {
	rt := New(time.Now())
	at := rt.NewAttempt(0, 1, 1)

	start := time.Now()
	at.requestStart = start
	at.hasStart = true
	at.gotConnTime = start
	at.wroteRequestTime = start.Add(3 * time.Millisecond) // 已写出
	at.MarkError(ErrorStageWaitResp)                       // 等待响应时失败

	snap := rt.Snapshot().Attempts[0]
	if snap.WriteMs == nil {
		t.Fatal("write_ms should be preserved")
	}
	if snap.ErrorStage != ErrorStageWaitResp {
		t.Fatalf("expected error stage %q, got %q", ErrorStageWaitResp, snap.ErrorStage)
	}
}

// TestMarkErrorKeepsEarliest 验证已记录错误阶段不被后续覆盖。
func TestMarkErrorKeepsEarliest(t *testing.T) {
	at := newAttempt(0, 1, 1)
	at.MarkError(ErrorStageGetConn)
	at.MarkError(ErrorStageWaitResp) // 不应覆盖
	if at.errStage != ErrorStageGetConn {
		t.Fatalf("expected earliest error stage %q, got %q", ErrorStageGetConn, at.errStage)
	}
}

// TestMultipleAttemptsIndependent 验证多次尝试相互独立。
func TestMultipleAttemptsIndependent(t *testing.T) {
	rt := New(time.Now())

	a0 := rt.NewAttempt(0, 10, 1)
	s0 := time.Now()
	a0.gotConnTime = s0
	a0.wroteRequestTime = s0.Add(5 * time.Millisecond)
	a0.MarkError(ErrorStageWaitResp)

	a1 := rt.NewAttempt(1, 20, 1)
	s1 := time.Now()
	a1.getConnTime = s1
	a1.gotConnTime = s1.Add(1 * time.Millisecond)
	a1.wroteRequestTime = s1.Add(2 * time.Millisecond)
	a1.gotFirstByteTime = s1.Add(4 * time.Millisecond)

	snap := rt.Snapshot()
	if len(snap.Attempts) != 2 {
		t.Fatalf("expected 2 attempts, got %d", len(snap.Attempts))
	}
	if snap.Attempts[0].Index != 0 || snap.Attempts[0].ChannelId != 10 {
		t.Fatalf("attempt 0 mismatch: %+v", snap.Attempts[0])
	}
	if snap.Attempts[0].ErrorStage != ErrorStageWaitResp {
		t.Fatalf("attempt 0 should have error stage, got %q", snap.Attempts[0].ErrorStage)
	}
	if snap.Attempts[1].Index != 1 || snap.Attempts[1].ChannelId != 20 {
		t.Fatalf("attempt 1 mismatch: %+v", snap.Attempts[1])
	}
	// attempt 1 成功，无错误阶段
	if snap.Attempts[1].ErrorStage != "" {
		t.Fatalf("attempt 1 should have no error stage, got %q", snap.Attempts[1].ErrorStage)
	}
	// 两次尝试数据不混合
	if snap.Attempts[1].WaitRespMs == nil {
		t.Fatal("attempt 1 should have wait_resp_ms")
	}
}

// TestMaxAttemptsCap 验证尝试次数上限，避免诊断数据无限增长。
func TestMaxAttemptsCap(t *testing.T) {
	rt := New(time.Now())
	for i := 0; i < maxAttempts+5; i++ {
		rt.NewAttempt(i, i, 1)
	}
	snap := rt.Snapshot()
	if len(snap.Attempts) != maxAttempts {
		t.Fatalf("expected attempts capped at %d, got %d", maxAttempts, len(snap.Attempts))
	}
}

// TestToAdminInfoValueRoundTrip 验证导出为 admin_info map 的往返。
func TestToAdminInfoValueRoundTrip(t *testing.T) {
	rt := New(time.Now())
	at := rt.NewAttempt(0, 7, 1)
	at.gotConnTime = time.Now()
	at.wroteRequestTime = at.gotConnTime.Add(time.Millisecond)

	m := rt.ToAdminInfoValue()
	if m == nil {
		t.Fatal("expected non-nil admin info value")
	}
	if _, ok := m["attempts"]; !ok {
		t.Fatal("expected attempts key in exported map")
	}

	// 无数据时应返回 nil。
	empty := New(time.Time{})
	if empty.ToAdminInfoValue() != nil {
		t.Fatal("expected nil for empty timing")
	}
}
