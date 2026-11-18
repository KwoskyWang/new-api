package relaytiming

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"strings"
	"testing"
	"time"
)

// 性能基准：用本地 httptest 模拟上游，对比"开启诊断（挂载 httptrace + 记录 attempt +
// 生成快照）"与"关闭诊断（直接请求）"在短请求、大请求、并发场景下的耗时与内存分配。
//
// 这些基准隔离的是"开启诊断相对关闭诊断"的增量开销，即本次改动在请求路径上新增的
// 全部工作量（attempt 创建、trace 回调、快照序列化准备）。基准不引入任何同步的
// 数据库、文件或网络写入。
//
// 运行示例：
//   go test -run x -bench 'BenchmarkRelay' -benchmem ./pkg/relaytiming/

const smallBody = `{"model":"test","messages":[{"role":"user","content":"hi"}]}`

func makeLargeBody() string {
	// ~256KB 请求体，模拟大请求（长上下文/附带大量内容）。
	var b strings.Builder
	b.WriteString(`{"model":"test","messages":[{"role":"user","content":"`)
	chunk := strings.Repeat("x", 1024)
	for i := 0; i < 256; i++ {
		b.WriteString(chunk)
	}
	b.WriteString(`"}]}`)
	return b.String()
}

func newBenchServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
}

// doOnce 执行一次请求；当 diagnose=true 时模拟开启诊断的完整路径。
func doOnce(client *http.Client, url, body string, diagnose bool) {
	var rt *RequestTiming
	var at *Attempt
	if diagnose {
		rt = New(time.Now())
		// 模拟本地处理阶段打点。
		rt.AddLocalPhase("auth_parse", time.Millisecond)
		rt.AddLocalPhase("token_estimate", time.Millisecond)
	}

	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewBufferString(body))

	if diagnose {
		at = rt.NewAttempt(0, 1, 1)
		if req.ContentLength > 0 {
			at.SetRequestBodySize(req.ContentLength)
		}
		req = req.WithContext(httptrace.WithClientTrace(req.Context(), at.Trace()))
		at.MarkRequestStart()
		rt.MarkUpstreamStart()
	}

	resp, err := client.Do(req)
	if err != nil {
		return
	}
	if diagnose {
		at.MarkResponseReturned()
		at.SetProtocol(resp.Proto)
		at.MarkFirstData(FirstDataKindFirstData)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	if diagnose {
		// 模拟日志写入前的快照序列化准备（不落盘）。
		_ = rt.ToAdminInfoValue()
	}
}

func benchRelay(b *testing.B, body string, diagnose bool) {
	srv := newBenchServer()
	defer srv.Close()
	client := srv.Client()
	// 预热一次，建立可复用连接。
	doOnce(client, srv.URL, body, diagnose)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		doOnce(client, srv.URL, body, diagnose)
	}
}

func BenchmarkRelayShortOff(b *testing.B) { benchRelay(b, smallBody, false) }
func BenchmarkRelayShortOn(b *testing.B)  { benchRelay(b, smallBody, true) }

func BenchmarkRelayLargeOff(b *testing.B) { benchRelay(b, makeLargeBody(), false) }
func BenchmarkRelayLargeOn(b *testing.B)  { benchRelay(b, makeLargeBody(), true) }

func benchRelayParallel(b *testing.B, diagnose bool) {
	srv := newBenchServer()
	defer srv.Close()
	client := srv.Client()
	doOnce(client, srv.URL, smallBody, diagnose)

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			doOnce(client, srv.URL, smallBody, diagnose)
		}
	})
}

func BenchmarkRelayConcurrentOff(b *testing.B) { benchRelayParallel(b, false) }
func BenchmarkRelayConcurrentOn(b *testing.B)  { benchRelayParallel(b, true) }

// BenchmarkSnapshotOnly 单独测量快照生成（含指针字段构建）的开销。
func BenchmarkSnapshotOnly(b *testing.B) {
	rt := New(time.Now())
	rt.AddLocalPhase("auth_parse", time.Millisecond)
	for i := 0; i < 3; i++ {
		at := rt.NewAttempt(i, i+1, 1)
		now := time.Now()
		at.getConnTime = now
		at.gotConnTime = now.Add(time.Millisecond)
		at.wroteRequestTime = now.Add(2 * time.Millisecond)
		at.gotFirstByteTime = now.Add(5 * time.Millisecond)
		at.firstDataTime = now.Add(100 * time.Millisecond)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = rt.Snapshot()
	}
}
