package relaytiming

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"sync"
	"testing"
	"time"
)

// TestTraceAgainstHTTPTestServer 用真实的 httptest 服务器与 httptrace 验证端到端采集，
// 不调用任何真实收费模型。覆盖：普通请求正常完成，协议/连接/写出/首字节被采集。
func TestTraceAgainstHTTPTestServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	rt := New(time.Now())
	at := rt.NewAttempt(0, 1, 1)

	req, err := http.NewRequest(http.MethodPost, srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx := httptrace.WithClientTrace(req.Context(), at.Trace())
	req = req.WithContext(ctx)

	at.MarkRequestStart()
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	at.MarkResponseReturned()
	at.SetProtocol(resp.Proto)
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	snap := rt.Snapshot().Attempts[0]
	if snap.GetConnMs == nil {
		t.Error("get_conn_ms should be captured")
	}
	if snap.WriteMs == nil {
		t.Error("write_ms should be captured")
	}
	if snap.WaitRespMs == nil {
		t.Error("wait_resp_ms should be captured")
	}
	if snap.RespHeaderMs == nil {
		t.Error("resp_header_ms should be captured")
	}
	if snap.Protocol == "" {
		t.Error("protocol should be captured")
	}
	// 第一次连接：ConnReused 应为 false（非 nil）。
	if snap.ConnReused == nil {
		t.Error("conn_reused should be recorded (got GotConn)")
	}
}

// TestTraceConnectionReuse 验证复用连接时 DNS/TLS 不被误记、conn_reused=true。
func TestTraceConnectionReuse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()
	client := srv.Client()

	rt := New(time.Now())

	// 第一次请求，建立连接。
	do := func(index int) AttemptSnapshot {
		at := rt.NewAttempt(index, 1, 1)
		req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
		req = req.WithContext(httptrace.WithClientTrace(req.Context(), at.Trace()))
		at.MarkRequestStart()
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		at.MarkResponseReturned()
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		return rt.Snapshot().Attempts[index]
	}

	_ = do(0)
	second := do(1)

	// 第二次通常复用连接（httptest 是 HTTP，无 TLS）。
	if second.ConnReused == nil {
		t.Fatal("second attempt should have conn_reused recorded")
	}
	if *second.ConnReused {
		// 复用时不应有 DNS/TCP 记录。
		if second.DNSMs != nil {
			t.Error("DNS should not be recorded on reused connection")
		}
		if second.TCPMs != nil {
			t.Error("TCP should not be recorded on reused connection")
		}
	}
}

// TestConcurrentTraceCallbacksNoRace 验证并发回调无数据竞争（配合 -race 运行）。
func TestConcurrentTraceCallbacksNoRace(t *testing.T) {
	rt := New(time.Now())
	at := rt.NewAttempt(0, 1, 1)
	trace := at.Trace()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			trace.GetConn("host")
			trace.DNSStart(httptrace.DNSStartInfo{})
			trace.DNSDone(httptrace.DNSDoneInfo{})
			trace.ConnectStart("tcp", "addr")
			trace.ConnectDone("tcp", "addr", nil)
			trace.GotConn(httptrace.GotConnInfo{Reused: true})
			trace.WroteRequest(httptrace.WroteRequestInfo{})
			trace.GotFirstResponseByte()
		}()
	}
	// 同时并发读取快照。
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = rt.Snapshot()
			_ = at.HasWroteRequest()
		}()
	}
	wg.Wait()

	snap := rt.Snapshot().Attempts[0]
	if snap.DNSCount == 0 {
		t.Error("expected DNS events counted")
	}
}
