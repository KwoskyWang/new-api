package relaytiming

import (
	"crypto/tls"
	"net/http/httptrace"
	"sync"
	"time"
)

// Attempt 表示一次上游渠道尝试的诊断计时。
//
// 一次 attempt 的生命周期：
//
//	NewAttempt -> Trace() 挂载到请求 -> MarkRequestStart -> client.Do ...
//	  httptrace 回调并发写入各时间点 -> MarkGotResponse / MarkError -> MarkFirstData
//
// 所有字段写入都通过 mu 保护，因为 httptrace 回调可能在不同 goroutine 并发执行
// （例如连接建立与 DNS 解析）。
type Attempt struct {
	mu sync.Mutex

	index       int
	channelId   int
	channelType int

	// requestStart 是本次 attempt 开始（即将调用 client.Do）的时间点，
	// 作为本 attempt 内所有相对耗时的基准。
	requestStart time.Time
	hasStart     bool

	// ── httptrace 采集的时间点（绝对时刻） ──
	getConnTime          time.Time // GetConn：开始获取连接
	gotConnTime          time.Time // GotConn：拿到连接
	dnsStartTime         time.Time
	dnsDoneTime          time.Time
	connectStartTime     time.Time // 首次 TCP 拨号开始（可能多次，仅记第一次）
	connectDoneTime      time.Time // 最后一次 TCP 拨号完成
	tlsStartTime         time.Time
	tlsDoneTime          time.Time
	wroteRequestTime     time.Time // WroteRequest：请求（含头与体）写出完成
	gotFirstByteTime     time.Time // GotFirstResponseByte：收到响应头首字节
	firstDataTime        time.Time // 第一条有效上游 SSE data（由 relay 主流程打点）
	responseReturnedTime time.Time // client.Do 返回响应头的时间（由 relay 主流程打点）

	// ── 布尔/标量状态 ──
	connReused      bool   // 连接是否复用
	connReusedSet   bool   // 是否收到过 GotConn 回调
	dnsOccurred     bool   // 是否发生过 DNS 解析
	tcpOccurred     bool   // 是否发生过 TCP 连接
	tlsOccurred     bool   // 是否发生过 TLS 握手
	tlsVersion      uint16 // TLS 版本（0 表示未知）
	connectCount    int    // TCP 拨号次数（可能多次）
	dnsCount        int    // DNS 解析次数
	protocol        string // 响应协议，如 "HTTP/1.1" / "HTTP/2.0"
	errStage        string // 错误发生的阶段，见 ErrorStage* 常量
	requestBodySize int64  // 应用层请求体字节数（-1 表示未知）
	firstDataKind   string // 第一条有效 data 的类型命名，见 FirstDataKind* 常量
}

// 错误阶段常量。
const (
	ErrorStageNone       = ""
	ErrorStageGetConn    = "get_conn"      // 获取连接阶段失败（含 DNS/TCP/TLS）
	ErrorStageWriteReq   = "write_request" // 请求写出阶段失败
	ErrorStageWaitResp   = "wait_response" // 等待上游响应头阶段失败
	ErrorStageReadStream = "read_stream"   // 读取流式响应阶段失败
	ErrorStageGeneric    = "do_request"    // 未能归类到上述阶段的 client.Do 失败
)

// 第一条有效 data 的类型命名。
// 注意：relay 现有打点只能可靠测到"第一条有效上游 data"，
// 不能断言它一定是"首个可见文字 Token"，因此如实命名。
const (
	FirstDataKindUnknown   = ""
	FirstDataKindFirstData = "first_data" // 第一条有效 SSE data（现有打点语义）
)

func newAttempt(index, channelId, channelType int) *Attempt {
	return &Attempt{
		index:           index,
		channelId:       channelId,
		channelType:     channelType,
		requestBodySize: -1,
		firstDataKind:   FirstDataKindUnknown,
	}
}

// SetRequestBodySize 记录最终发往上游的应用层请求体字节数。
// 这是应用层字节数，不包含 TLS/HTTP 等线上开销。
func (a *Attempt) SetRequestBodySize(n int64) {
	if a == nil || n < 0 {
		return
	}
	a.mu.Lock()
	a.requestBodySize = n
	a.mu.Unlock()
}

// MarkRequestStart 标记本次 attempt 即将调用 client.Do 的时间点。
func (a *Attempt) MarkRequestStart() {
	if a == nil {
		return
	}
	now := time.Now()
	a.mu.Lock()
	if !a.hasStart {
		a.requestStart = now
		a.hasStart = true
	}
	a.mu.Unlock()
}

// MarkResponseReturned 标记 client.Do 返回响应头的时间点。
func (a *Attempt) MarkResponseReturned() {
	if a == nil {
		return
	}
	now := time.Now()
	a.mu.Lock()
	if a.responseReturnedTime.IsZero() {
		a.responseReturnedTime = now
	}
	a.mu.Unlock()
}

// MarkError 记录错误阶段。若已记录则不覆盖（保留最早的错误阶段）。
func (a *Attempt) MarkError(stage string) {
	if a == nil || stage == "" {
		return
	}
	a.mu.Lock()
	if a.errStage == ErrorStageNone {
		a.errStage = stage
	}
	a.mu.Unlock()
}

// SetProtocol 记录响应协议（如 "HTTP/2.0"）。
func (a *Attempt) SetProtocol(proto string) {
	if a == nil || proto == "" {
		return
	}
	a.mu.Lock()
	if a.protocol == "" {
		a.protocol = proto
	}
	a.mu.Unlock()
}

// MarkFirstData 记录第一条有效上游 data 的时间点及其类型命名。
// 复用现有 SetFirstResponseTime 的语义：第一条有效 SSE data。
func (a *Attempt) MarkFirstData(kind string) {
	if a == nil {
		return
	}
	now := time.Now()
	a.mu.Lock()
	if a.firstDataTime.IsZero() {
		a.firstDataTime = now
		if kind != "" {
			a.firstDataKind = kind
		}
	}
	a.mu.Unlock()
}

// HasWroteRequest 返回是否已采集到 WroteRequest 事件（请求写出完成）。
func (a *Attempt) HasWroteRequest() bool {
	if a == nil {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return !a.wroteRequestTime.IsZero()
}

// HasGotConn 返回是否已拿到连接（GotConn 事件）。
func (a *Attempt) HasGotConn() bool {
	if a == nil {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return !a.gotConnTime.IsZero()
}

// HasGotFirstByte 返回是否已收到响应头首字节（GotFirstResponseByte 事件）。
func (a *Attempt) HasGotFirstByte() bool {
	if a == nil {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return !a.gotFirstByteTime.IsZero()
}

// Trace 返回一个 *httptrace.ClientTrace，用于挂载到出站请求。
// 回调内部通过 a.mu 保护，可安全并发执行。
//
// 注意：
//   - GetConn 时间可能包含 DNS/TCP/TLS，不应与它们重复求和。
//   - DNS/TCP 可能有多次事件；这里对 DNS 采用"首开始/末完成"的汇总，并记录次数；
//     TCP 同理，另记录拨号次数。
//   - WroteRequest 不代表上游已经完整收到请求。
//   - GotFirstResponseByte 是响应头首字节，不是模型首个文字 Token。
func (a *Attempt) Trace() *httptrace.ClientTrace {
	if a == nil {
		return nil
	}
	return &httptrace.ClientTrace{
		GetConn: func(string) {
			now := time.Now()
			a.mu.Lock()
			if a.getConnTime.IsZero() {
				a.getConnTime = now
			}
			a.mu.Unlock()
		},
		GotConn: func(info httptrace.GotConnInfo) {
			now := time.Now()
			a.mu.Lock()
			if a.gotConnTime.IsZero() {
				a.gotConnTime = now
			}
			a.connReused = info.Reused
			a.connReusedSet = true
			a.mu.Unlock()
		},
		DNSStart: func(httptrace.DNSStartInfo) {
			now := time.Now()
			a.mu.Lock()
			a.dnsOccurred = true
			a.dnsCount++
			if a.dnsStartTime.IsZero() {
				a.dnsStartTime = now
			}
			a.mu.Unlock()
		},
		DNSDone: func(httptrace.DNSDoneInfo) {
			now := time.Now()
			a.mu.Lock()
			a.dnsDoneTime = now // 末次完成
			a.mu.Unlock()
		},
		ConnectStart: func(network, addr string) {
			now := time.Now()
			a.mu.Lock()
			a.tcpOccurred = true
			a.connectCount++
			if a.connectStartTime.IsZero() {
				a.connectStartTime = now
			}
			a.mu.Unlock()
		},
		ConnectDone: func(network, addr string, err error) {
			now := time.Now()
			a.mu.Lock()
			a.connectDoneTime = now // 末次完成
			if err != nil && a.errStage == ErrorStageNone {
				a.errStage = ErrorStageGetConn
			}
			a.mu.Unlock()
		},
		TLSHandshakeStart: func() {
			now := time.Now()
			a.mu.Lock()
			a.tlsOccurred = true
			if a.tlsStartTime.IsZero() {
				a.tlsStartTime = now
			}
			a.mu.Unlock()
		},
		TLSHandshakeDone: func(state tls.ConnectionState, err error) {
			now := time.Now()
			a.mu.Lock()
			a.tlsDoneTime = now
			a.tlsVersion = state.Version
			if err != nil && a.errStage == ErrorStageNone {
				a.errStage = ErrorStageGetConn
			}
			a.mu.Unlock()
		},
		WroteRequest: func(info httptrace.WroteRequestInfo) {
			now := time.Now()
			a.mu.Lock()
			if a.wroteRequestTime.IsZero() {
				a.wroteRequestTime = now
			}
			if info.Err != nil && a.errStage == ErrorStageNone {
				a.errStage = ErrorStageWriteReq
			}
			a.mu.Unlock()
		},
		GotFirstResponseByte: func() {
			now := time.Now()
			a.mu.Lock()
			if a.gotFirstByteTime.IsZero() {
				a.gotFirstByteTime = now
			}
			a.mu.Unlock()
		},
	}
}
