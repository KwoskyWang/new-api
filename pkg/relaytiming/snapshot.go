package relaytiming

import "time"

// Snapshot 是 RequestTiming 的只读快照，用于序列化进日志（other.admin_info.relay_timing）。
//
// 时长单位统一为毫秒。为了区分"未发生/未知"与"真实 0ms"：
//   - 可选的耗时字段使用 *int64 指针：nil 表示该阶段未发生或无法采集（前端显示"无记录"）；
//     非 nil 的 0 表示真实测得的 0 毫秒。
//   - 布尔/枚举类字段用指针或空字符串表达"未知"。
type Snapshot struct {
	// LocalTotalMs 为应用进入到快照时刻的本地处理总耗时；-1 表示无记录（应用进入时间不可用）。
	LocalTotalMs int64 `json:"local_total_ms"`
	// LocalToUpstreamMs 为应用进入到首次发起上游 HTTP 请求的本地处理耗时；
	// -1 表示无记录。这是定位"本地处理慢 vs 上游慢"的关键字段。
	LocalToUpstreamMs int64             `json:"local_to_upstream_ms"`
	LocalPhases       []LocalPhase      `json:"local_phases,omitempty"`
	Attempts          []AttemptSnapshot `json:"attempts,omitempty"`
}

// AttemptSnapshot 是单次上游尝试的只读快照。
//
// 字段计时边界（务必与前端展示文案保持一致）：
//   - RequestBodySize：最终发往上游的应用层请求体字节数（不含 TLS/HTTP 线上开销）；nil 表示未知。
//   - GetConnMs：从请求开始到拿到连接的耗时；可能包含 DNS/TCP/TLS，不应与它们重复求和。
//   - DNSMs/TCPMs/TLSMs：各自握手耗时；nil 表示本次未发生（如连接复用时 DNS/TLS 不发生）。
//   - WriteMs：从拿到连接到请求写出完成（WroteRequest）的耗时；包含头、体与传输等待，
//     不能简单称为"纯网络上传时间"。
//   - WaitRespMs：从请求写出完成到收到上游响应头首字节（GotFirstResponseByte）的耗时。
//   - RespHeaderMs：从请求开始到 client.Do 返回响应头的耗时。
//   - FirstDataMs：第一条有效上游 data 相对"请求写出完成"的耗时；FirstDataKind 说明其类型。
//   - ConnReused：连接是否复用；nil 表示未收到 GotConn 回调（未知）。
//   - Protocol：响应协议；空表示未知。
//   - ErrorStage：错误发生的阶段；空表示无错误。
type AttemptSnapshot struct {
	Index       int `json:"index"`
	ChannelId   int `json:"channel_id,omitempty"`
	ChannelType int `json:"channel_type,omitempty"`

	RequestBodySize *int64 `json:"request_body_size,omitempty"`

	GetConnMs    *int64 `json:"get_conn_ms,omitempty"`
	DNSMs        *int64 `json:"dns_ms,omitempty"`
	TCPMs        *int64 `json:"tcp_ms,omitempty"`
	TLSMs        *int64 `json:"tls_ms,omitempty"`
	WriteMs      *int64 `json:"write_ms,omitempty"`
	WaitRespMs   *int64 `json:"wait_resp_ms,omitempty"`
	RespHeaderMs *int64 `json:"resp_header_ms,omitempty"`
	FirstDataMs  *int64 `json:"first_data_ms,omitempty"`

	DNSCount     int `json:"dns_count,omitempty"`
	ConnectCount int `json:"connect_count,omitempty"`

	ConnReused    *bool  `json:"conn_reused,omitempty"`
	TLSVersion    string `json:"tls_version,omitempty"`
	Protocol      string `json:"protocol,omitempty"`
	ErrorStage    string `json:"error_stage,omitempty"`
	FirstDataKind string `json:"first_data_kind,omitempty"`
}

func ptrMs(d time.Duration) *int64 {
	ms := d.Milliseconds()
	if ms < 0 {
		ms = 0
	}
	return &ms
}

func ptrBool(b bool) *bool { return &b }

func tlsVersionString(v uint16) string {
	switch v {
	case 0:
		return ""
	case 0x0301:
		return "TLS 1.0"
	case 0x0302:
		return "TLS 1.1"
	case 0x0303:
		return "TLS 1.2"
	case 0x0304:
		return "TLS 1.3"
	default:
		return ""
	}
}

func (a *Attempt) snapshot() AttemptSnapshot {
	a.mu.Lock()
	defer a.mu.Unlock()

	snap := AttemptSnapshot{
		Index:         a.index,
		ChannelId:     a.channelId,
		ChannelType:   a.channelType,
		DNSCount:      a.dnsCount,
		ConnectCount:  a.connectCount,
		Protocol:      a.protocol,
		ErrorStage:    a.errStage,
		FirstDataKind: a.firstDataKind,
		TLSVersion:    tlsVersionString(a.tlsVersion),
	}

	if a.requestBodySize >= 0 {
		size := a.requestBodySize
		snap.RequestBodySize = &size
	}
	if a.connReusedSet {
		snap.ConnReused = ptrBool(a.connReused)
	}

	// 获取连接耗时：GetConn -> GotConn。
	if !a.getConnTime.IsZero() && !a.gotConnTime.IsZero() {
		snap.GetConnMs = ptrMs(a.gotConnTime.Sub(a.getConnTime))
	}

	// DNS：仅在确实发生过且首尾时刻齐备时记录。
	if a.dnsOccurred && !a.dnsStartTime.IsZero() && !a.dnsDoneTime.IsZero() {
		snap.DNSMs = ptrMs(a.dnsDoneTime.Sub(a.dnsStartTime))
	}
	// TCP：同理。
	if a.tcpOccurred && !a.connectStartTime.IsZero() && !a.connectDoneTime.IsZero() {
		snap.TCPMs = ptrMs(a.connectDoneTime.Sub(a.connectStartTime))
	}
	// TLS：同理。连接复用时不发生 TLS，snap.TLSMs 保持 nil（前端显示"无记录"）。
	if a.tlsOccurred && !a.tlsStartTime.IsZero() && !a.tlsDoneTime.IsZero() {
		snap.TLSMs = ptrMs(a.tlsDoneTime.Sub(a.tlsStartTime))
	}

	// 写出耗时：拿到连接 -> WroteRequest。
	if !a.gotConnTime.IsZero() && !a.wroteRequestTime.IsZero() {
		snap.WriteMs = ptrMs(a.wroteRequestTime.Sub(a.gotConnTime))
	}

	// 写出完成 -> 响应头首字节。
	if !a.wroteRequestTime.IsZero() && !a.gotFirstByteTime.IsZero() {
		snap.WaitRespMs = ptrMs(a.gotFirstByteTime.Sub(a.wroteRequestTime))
	}

	// 请求开始 -> client.Do 返回响应头。
	if a.hasStart && !a.responseReturnedTime.IsZero() {
		snap.RespHeaderMs = ptrMs(a.responseReturnedTime.Sub(a.requestStart))
	}

	// 第一条有效 data 相对"请求写出完成"的耗时。
	// 若 WroteRequest 未采集到（如非标准路径），回退到请求开始作为基准，
	// 但 FirstDataKind 仍如实命名，不声称它是首个文字 Token。
	if !a.firstDataTime.IsZero() {
		base := a.wroteRequestTime
		if base.IsZero() && a.hasStart {
			base = a.requestStart
		}
		if !base.IsZero() {
			snap.FirstDataMs = ptrMs(a.firstDataTime.Sub(base))
		}
	}

	return snap
}
