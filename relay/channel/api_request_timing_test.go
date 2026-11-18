package channel

import (
	"net/http/httptrace"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/pkg/relaytiming"
)

func mustTimeNow() time.Time { return time.Now() }

func feedGotConn(at *relaytiming.Attempt) {
	tr := at.Trace()
	tr.GetConn("host")
	tr.GotConn(httptrace.GotConnInfo{})
}

func feedWroteRequest(at *relaytiming.Attempt) {
	tr := at.Trace()
	tr.WroteRequest(httptrace.WroteRequestInfo{})
}

// TestClassifyDoRequestErrorStage 验证在 client.Do 返回错误时，依据 httptrace 已采集到的
// 时间点推断错误阶段的逻辑：连接未建立 -> get_conn；已连接未写完 -> write_request；
// 已写出未收到首字节 -> wait_response。
func TestClassifyDoRequestErrorStage(t *testing.T) {
	// nil attempt -> generic
	if got := classifyDoRequestErrorStage(nil); got != relaytiming.ErrorStageGeneric {
		t.Fatalf("nil attempt: expected %q, got %q", relaytiming.ErrorStageGeneric, got)
	}

	// 未拿到连接 -> get_conn
	rt := relaytiming.New(mustTimeNow())
	at := rt.NewAttempt(0, 1, 1)
	if got := classifyDoRequestErrorStage(at); got != relaytiming.ErrorStageGetConn {
		t.Fatalf("no conn: expected %q, got %q", relaytiming.ErrorStageGetConn, got)
	}

	// 已拿到连接但未写完请求 -> write_request
	at2 := rt.NewAttempt(1, 1, 1)
	feedGotConn(at2)
	if got := classifyDoRequestErrorStage(at2); got != relaytiming.ErrorStageWriteReq {
		t.Fatalf("conn only: expected %q, got %q", relaytiming.ErrorStageWriteReq, got)
	}

	// 已写出但未收到首字节 -> wait_response
	at3 := rt.NewAttempt(2, 1, 1)
	feedGotConn(at3)
	feedWroteRequest(at3)
	if got := classifyDoRequestErrorStage(at3); got != relaytiming.ErrorStageWaitResp {
		t.Fatalf("wrote only: expected %q, got %q", relaytiming.ErrorStageWaitResp, got)
	}
}
