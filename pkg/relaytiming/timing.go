// Package relaytiming 提供"管理员专属的转发链路分段耗时诊断"所需的
// 低开销、线程安全的分段计时能力。
//
// 设计目标：
//   - 不改变现有 StartTime / frt / use_time 的含义与起点；本包是一套独立的诊断计时。
//   - 所有时长都使用 time.Now()/time.Since()（保留单调时钟）计算。
//   - httptrace 回调可能在不同 goroutine 并发执行，因此 Tracer 内部用锁保护。
//   - 每次渠道尝试（attempt）独立记录，不同尝试的数据绝不混合。
//   - 容量有上限，避免诊断数据无限增长。
//
// 字段语义（计时边界）详见各字段注释与 snapshot.go 中的导出结构。
package relaytiming

import (
	"sync"
	"time"
)

// maxAttempts 为单个请求记录的最大渠道尝试次数。
// 超过该上限后不再追加新的 attempt，防止异常重试风暴导致诊断数据膨胀。
const maxAttempts = 8

// maxLocalPhases 为本地处理阶段分组的最大数量，防止调用方无限写入。
const maxLocalPhases = 32

// RequestTiming 是单个转发请求的诊断计时容器。
//
// 它覆盖两部分：
//  1. 本地处理（local processing）：从"应用进入时间"(EnterTime) 到真正开始第一次
//     上游 HTTP 请求之前，在本进程内消耗的时间，按阶段分组。
//  2. 若干次上游尝试（Attempts）：每次渠道尝试一条，含 httptrace 采集到的连接/写出/
//     首字节等细分耗时。
//
// 并发说明：RequestTiming 的方法由 relay 主流程串行调用（本地阶段、attempt 的开始/结束
// 都在主 goroutine），而每个 Attempt 内部的 httptrace 回调由 Tracer 用锁保护。
// 为稳妥起见，RequestTiming 自身也用互斥锁保护切片的追加与读取快照。
type RequestTiming struct {
	mu sync.Mutex

	// enterTime 是请求进入应用（最早中间件）的时间点，作为本地处理计时的基准。
	enterTime time.Time
	enterSet  bool

	// localPhases 记录本地处理的各阶段耗时（毫秒），按插入顺序保存。
	localPhases []LocalPhase

	// attempts 记录每一次上游渠道尝试。
	attempts []*Attempt

	// localToUpstreamMs 记录"应用进入到首次发起上游 HTTP 请求"的总耗时（毫秒）。
	// -1 表示尚未记录或应用进入时间不可用。仅在第一次 attempt 开始时记录一次。
	localToUpstreamMs int64
}

// LocalPhase 表示本地处理的一个阶段分组。
// 各阶段边界互不重叠，调用方负责保证不会把重叠区间重复累加。
type LocalPhase struct {
	Name       string `json:"name"`
	DurationMs int64  `json:"ms"`
}

// New 创建一个 RequestTiming。enterTime 为应用进入时间；若为零值则本地处理基准不可用，
// 后续 StartLocalPhase 之类的相对计时仍可工作（以调用时刻为基准）。
func New(enterTime time.Time) *RequestTiming {
	rt := &RequestTiming{}
	rt.localToUpstreamMs = -1
	if !enterTime.IsZero() {
		rt.enterTime = enterTime
		rt.enterSet = true
	}
	return rt
}

// MarkUpstreamStart 在首次发起上游 HTTP 请求时调用，记录"应用进入 -> 首次上游请求"的
// 本地处理总耗时。仅记录第一次（后续重试不再覆盖），以反映初始请求的本地准备开销。
func (rt *RequestTiming) MarkUpstreamStart() {
	if rt == nil {
		return
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.localToUpstreamMs >= 0 || !rt.enterSet {
		return
	}
	ms := time.Since(rt.enterTime).Milliseconds()
	if ms < 0 {
		ms = 0
	}
	rt.localToUpstreamMs = ms
}

// EnterTime 返回应用进入时间（可能为零值）。
func (rt *RequestTiming) EnterTime() time.Time {
	if rt == nil {
		return time.Time{}
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return rt.enterTime
}

// AddLocalPhaseSince 记录一个本地处理阶段：名称 name，耗时为 since 到现在的间隔。
// 使用单调时钟计算。该方法不假设各阶段连续，调用方自行界定边界。
func (rt *RequestTiming) AddLocalPhaseSince(name string, since time.Time) {
	if rt == nil || name == "" || since.IsZero() {
		return
	}
	rt.AddLocalPhase(name, time.Since(since))
}

// AddLocalPhase 直接记录一个本地处理阶段及其耗时。
func (rt *RequestTiming) AddLocalPhase(name string, d time.Duration) {
	if rt == nil || name == "" {
		return
	}
	ms := d.Milliseconds()
	if ms < 0 {
		// 不产生误导性的负耗时。
		ms = 0
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if len(rt.localPhases) >= maxLocalPhases {
		return
	}
	rt.localPhases = append(rt.localPhases, LocalPhase{Name: name, DurationMs: ms})
}

// LocalTotalMs 返回从应用进入到现在的本地处理总耗时（毫秒）。
// 若应用进入时间不可用则返回 -1，表示"无记录"。
func (rt *RequestTiming) localTotalMsLocked() int64 {
	if !rt.enterSet {
		return -1
	}
	ms := time.Since(rt.enterTime).Milliseconds()
	if ms < 0 {
		ms = 0
	}
	return ms
}

// NewAttempt 创建并登记一次新的上游尝试。
// index 为该尝试在本请求内的序号（从 0 开始，对应 relayInfo.RetryIndex）。
// 返回的 *Attempt 用于后续挂载 httptrace 与写入结果；若超过上限返回一个游离的
// Attempt（不被收集），保证调用方不必判空。
func (rt *RequestTiming) NewAttempt(index int, channelId int, channelType int) *Attempt {
	at := newAttempt(index, channelId, channelType)
	if rt == nil {
		return at
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if len(rt.attempts) >= maxAttempts {
		// 达到上限：返回游离 attempt，不收集，避免无限增长。
		return at
	}
	rt.attempts = append(rt.attempts, at)
	return at
}

// Snapshot 生成当前计时的只读快照，供日志序列化使用。
// 该方法会复制数据，调用后原始结构的后续变更不影响已返回的快照。
func (rt *RequestTiming) Snapshot() Snapshot {
	if rt == nil {
		return Snapshot{}
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()

	snap := Snapshot{
		LocalTotalMs:      rt.localTotalMsLocked(),
		LocalToUpstreamMs: rt.localToUpstreamMs,
	}
	if len(rt.localPhases) > 0 {
		snap.LocalPhases = make([]LocalPhase, len(rt.localPhases))
		copy(snap.LocalPhases, rt.localPhases)
	}
	if len(rt.attempts) > 0 {
		snap.Attempts = make([]AttemptSnapshot, 0, len(rt.attempts))
		for _, at := range rt.attempts {
			snap.Attempts = append(snap.Attempts, at.snapshot())
		}
	}
	return snap
}

// HasData 返回是否有任何可供展示的诊断数据。
func (rt *RequestTiming) HasData() bool {
	if rt == nil {
		return false
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return rt.enterSet || len(rt.localPhases) > 0 || len(rt.attempts) > 0
}
