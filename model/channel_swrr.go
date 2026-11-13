package model

import (
	"fmt"
	"sort"
	"sync"
)

// Smooth Weighted Round-Robin (SWRR) channel selection.
// 平滑加权轮询（SWRR）渠道选择。
//
// Unlike the random weighted selection, SWRR guarantees that over N requests
// the number of hits per channel matches its weight ratio exactly (as long as
// the candidate set stays stable), while distributing the choices smoothly
// instead of in bursts. This is the same algorithm used by Nginx upstream
// weighted load balancing.
// 与随机加权不同，SWRR 保证在 N 次请求内（候选集稳定时）各渠道命中次数严格符合
// 权重比例，并且分布平滑而非突发。该算法与 Nginx upstream 加权负载均衡一致。
//
// When a channel is disabled, it simply disappears from the candidate set and
// its counter is dropped; the remaining channels then keep selecting according
// to their relative weights. This naturally yields "redistribute by remaining
// weights" behavior.
// 当某渠道被关闭时，它会从候选集中消失且计数器被清理；剩余渠道继续按相对权重选择，
// 天然实现"按存活渠道剩余权重重新分配"的效果。

// swrrSelectorKey uniquely identifies a candidate pool. Candidates for a given
// (group, model, priority) are stable, so one selector is maintained per key.
// swrrSelectorKey 唯一标识一个候选池。给定 (group, model, priority) 的候选渠道集是
// 稳定的，因此每个 key 维护一个选择器。
type swrrSelectorKey struct {
	group    string
	model    string
	priority int64
}

// swrrSelector holds the running "current weight" state for each channel in a
// candidate pool. It is not safe for concurrent use on its own; access is
// guarded by swrrManager.
// swrrSelector 保存候选池中每个渠道的运行态"当前权重"。其本身不保证并发安全，
// 由 swrrManager 加锁保护。
type swrrSelector struct {
	current map[int]int // channelId -> current weight
}

// swrrManager is a global, concurrency-safe registry of SWRR selectors.
// swrrManager 是全局的、并发安全的 SWRR 选择器注册表。
type swrrManager struct {
	mu        sync.Mutex
	selectors map[swrrSelectorKey]*swrrSelector
}

var globalSWRR = &swrrManager{
	selectors: make(map[swrrSelectorKey]*swrrSelector),
}

// pick runs one step of smooth weighted round-robin over the given candidates
// and returns the id of the selected channel.
// pick 对给定候选渠道执行一步平滑加权轮询，返回被选中渠道的 id。
//
// weights maps channelId -> effective weight (must be > 0 for every candidate).
// weights 为 channelId -> 有效权重（每个候选必须 > 0）。
//
// The candidate set may change between calls (e.g. a channel is disabled). Stale
// counters for channels no longer present are pruned, and newly appeared
// channels start from current weight 0.
// 候选集可能在多次调用间变化（例如某渠道被关闭）。不再存在的渠道计数器会被清理，
// 新出现的渠道从当前权重 0 开始。
func (m *swrrManager) pick(key swrrSelectorKey, weights map[int]int) (int, bool) {
	if len(weights) == 0 {
		return 0, false
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	selector, ok := m.selectors[key]
	if !ok {
		selector = &swrrSelector{current: make(map[int]int, len(weights))}
		m.selectors[key] = selector
	}

	// Prune counters for channels that are no longer candidates, so a channel
	// that gets re-enabled later does not resume with a stale (possibly very
	// negative) current weight.
	// 清理不再是候选的渠道计数器，避免某渠道重新启用后带着陈旧（可能为很负）的
	// 当前权重继续参与。
	if len(selector.current) > 0 {
		for id := range selector.current {
			if _, stillCandidate := weights[id]; !stillCandidate {
				delete(selector.current, id)
			}
		}
	}

	totalWeight := 0
	for _, w := range weights {
		totalWeight += w
	}
	if totalWeight <= 0 {
		return 0, false
	}

	// Deterministic iteration order for tie-breaking so behavior is reproducible
	// regardless of Go map iteration randomness.
	// 使用确定性的遍历顺序进行平局决胜，保证行为与 Go map 随机遍历无关、可复现。
	ids := make([]int, 0, len(weights))
	for id := range weights {
		ids = append(ids, id)
	}
	sort.Ints(ids)

	bestID := -1
	bestCurrent := 0
	for _, id := range ids {
		cur := selector.current[id] + weights[id]
		selector.current[id] = cur
		if bestID == -1 || cur > bestCurrent {
			bestID = id
			bestCurrent = cur
		}
	}

	if bestID == -1 {
		return 0, false
	}

	selector.current[bestID] = bestCurrent - totalWeight
	return bestID, true
}

// resetSWRRSelectors drops all SWRR state. Called when the channel cache is
// rebuilt so that counters do not carry stale channel ids across reloads.
// resetSWRRSelectors 清空全部 SWRR 状态。在渠道缓存重建时调用，避免计数器跨重载
// 携带陈旧的渠道 id。
func resetSWRRSelectors() {
	globalSWRR.mu.Lock()
	globalSWRR.selectors = make(map[swrrSelectorKey]*swrrSelector)
	globalSWRR.mu.Unlock()
}

// selectChannelBySWRR selects one channel from targetChannels using SWRR.
// All channels in targetChannels must share the same priority.
// selectChannelBySWRR 使用 SWRR 从 targetChannels 中选择一个渠道。
// targetChannels 中所有渠道必须具有相同的优先级。
func selectChannelBySWRR(group, model string, priority int64, targetChannels []*Channel) (*Channel, error) {
	if len(targetChannels) == 0 {
		return nil, fmt.Errorf("no channel found, group: %s, model: %s, priority: %d", group, model, priority)
	}
	if len(targetChannels) == 1 {
		return targetChannels[0], nil
	}

	// Build effective weights.
	// 构造有效权重。
	//
	// Semantics: weight 0 means "do not participate" — such channels are excluded
	// from selection entirely, as long as at least one channel in this priority
	// layer has a positive weight. This lets operators keep extra channels
	// configured (weight 0) without receiving any traffic.
	// 语义：权重 0 表示"不参与分流"——只要该优先级层内至少有一个渠道权重 > 0，
	// 这些 0 权重渠道会被完全排除、不接收任何流量。这样可以把多余渠道以权重 0 的
	// 形式保留在配置里而不被调用。
	//
	// Fallback: if every channel in this layer has weight 0, treat them all as
	// equal (weight 1) so the layer does not end up with zero available traffic.
	// 兜底：如果该层所有渠道权重都为 0，则全部按相等(权重 1)处理，避免整层零流量
	// 导致无渠道可用。
	hasPositiveWeight := false
	for _, ch := range targetChannels {
		if ch.GetWeight() > 0 {
			hasPositiveWeight = true
			break
		}
	}

	weights := make(map[int]int, len(targetChannels))
	byID := make(map[int]*Channel, len(targetChannels))
	for _, ch := range targetChannels {
		w := ch.GetWeight()
		if hasPositiveWeight {
			// Exclude zero-weight channels from the candidate pool.
			// 将 0 权重渠道从候选池中排除。
			if w <= 0 {
				continue
			}
		} else {
			// All weights are 0 in this layer: distribute evenly.
			// 该层全部为 0 权重：均匀分配。
			w = 1
		}
		weights[ch.Id] = w
		byID[ch.Id] = ch
	}

	if len(byID) == 1 {
		for _, ch := range byID {
			return ch, nil
		}
	}

	key := swrrSelectorKey{group: group, model: model, priority: priority}
	id, ok := globalSWRR.pick(key, weights)
	if !ok {
		return nil, fmt.Errorf("channel not found, group: %s, model: %s, priority: %d", group, model, priority)
	}
	ch, ok := byID[id]
	if !ok {
		return nil, fmt.Errorf("channel not found, group: %s, model: %s, priority: %d", group, model, priority)
	}
	return ch, nil
}
