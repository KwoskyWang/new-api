package model

import (
	"sync"
	"testing"
)

// TestSWRRExactQuota verifies that over a full weight cycle the hit count of
// each channel matches its weight exactly, and that the distribution is smooth
// (no single channel is picked in one big burst).
// 验证在一个完整权重周期内各渠道命中次数严格等于其权重，且分布平滑。
func TestSWRRExactQuota(t *testing.T) {
	resetSWRRSelectors()
	key := swrrSelectorKey{group: "g", model: "m", priority: 0}
	weights := map[int]int{1: 70, 2: 20, 3: 10} // A=70% B=20% C=10%

	counts := map[int]int{}
	for i := 0; i < 100; i++ {
		id, ok := globalSWRR.pick(key, weights)
		if !ok {
			t.Fatalf("pick failed at iteration %d", i)
		}
		counts[id]++
	}

	if counts[1] != 70 || counts[2] != 20 || counts[3] != 10 {
		t.Fatalf("expected 70/20/10 over 100 picks, got A=%d B=%d C=%d", counts[1], counts[2], counts[3])
	}

	// Over the first 10 picks, with 70/20/10 the expected exact split is 7/2/1.
	// 前 10 次，70/20/10 的精确配额应为 7/2/1。
	resetSWRRSelectors()
	ten := map[int]int{}
	for i := 0; i < 10; i++ {
		id, _ := globalSWRR.pick(key, weights)
		ten[id]++
	}
	if ten[1] != 7 || ten[2] != 2 || ten[3] != 1 {
		t.Fatalf("expected 7/2/1 over first 10 picks, got A=%d B=%d C=%d", ten[1], ten[2], ten[3])
	}
}

// TestSWRRRedistributeOnDisable verifies that once a channel disappears from the
// candidate set, traffic is redistributed to the survivors by their remaining
// weights (B:C = 20:10 => 2:1).
// 验证某渠道从候选集中消失后，流量按存活渠道剩余权重重分配（B:C = 20:10 => 2:1）。
func TestSWRRRedistributeOnDisable(t *testing.T) {
	resetSWRRSelectors()
	key := swrrSelectorKey{group: "g", model: "m", priority: 0}

	full := map[int]int{1: 70, 2: 20, 3: 10}
	for i := 0; i < 20; i++ {
		globalSWRR.pick(key, full)
	}

	// Channel A disabled: candidate set becomes B=20, C=10.
	// 渠道 A 关闭：候选集变为 B=20, C=10。
	survivors := map[int]int{2: 20, 3: 10}
	counts := map[int]int{}
	for i := 0; i < 30; i++ {
		id, ok := globalSWRR.pick(key, survivors)
		if !ok {
			t.Fatalf("pick failed at iteration %d", i)
		}
		if id == 1 {
			t.Fatalf("disabled channel A was selected")
		}
		counts[id]++
	}
	if counts[2] != 20 || counts[3] != 10 {
		t.Fatalf("expected B/C = 20/10 over 30 picks after disabling A, got B=%d C=%d", counts[2], counts[3])
	}
}

// TestSWRRSingleAndEmpty covers the trivial candidate sets.
// 覆盖单渠道与空候选集的边界情况。
func TestSWRRSingleAndEmpty(t *testing.T) {
	resetSWRRSelectors()
	key := swrrSelectorKey{group: "g", model: "m", priority: 0}

	if _, ok := globalSWRR.pick(key, map[int]int{}); ok {
		t.Fatalf("empty candidate set should return ok=false")
	}

	single := map[int]int{5: 100}
	for i := 0; i < 5; i++ {
		id, ok := globalSWRR.pick(key, single)
		if !ok || id != 5 {
			t.Fatalf("single candidate should always be selected, got id=%d ok=%v", id, ok)
		}
	}
}

func chanWithWeight(id int, weight uint, priority int64) *Channel {
	w := weight
	p := priority
	return &Channel{Id: id, Weight: &w, Priority: &p}
}

// TestSelectChannelBySWRRZeroWeightExcluded verifies that zero-weight channels
// are excluded from selection when at least one channel in the layer has a
// positive weight (e.g. configure 7/2/1/0/0 => only the first three are used).
// 验证当该层至少有一个渠道权重 > 0 时，权重 0 的渠道被完全排除（例如配置
// 7/2/1/0/0 => 只有前三个会被使用）。
func TestSelectChannelBySWRRZeroWeightExcluded(t *testing.T) {
	resetSWRRSelectors()
	chans := []*Channel{
		chanWithWeight(1, 7, 0),
		chanWithWeight(2, 2, 0),
		chanWithWeight(3, 1, 0),
		chanWithWeight(4, 0, 0),
		chanWithWeight(5, 0, 0),
	}

	counts := map[int]int{}
	for i := 0; i < 100; i++ {
		ch, err := selectChannelBySWRR("g", "m", 0, chans)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		counts[ch.Id]++
	}

	if counts[4] != 0 || counts[5] != 0 {
		t.Fatalf("zero-weight channels must never be selected, got #4=%d #5=%d", counts[4], counts[5])
	}
	if counts[1] != 70 || counts[2] != 20 || counts[3] != 10 {
		t.Fatalf("expected 70/20/10 among positive-weight channels, got #1=%d #2=%d #3=%d", counts[1], counts[2], counts[3])
	}
}

// TestSelectChannelBySWRRAllZeroFallback verifies that if every channel in the
// layer has weight 0, selection falls back to equal distribution instead of
// returning no channel.
// 验证当该层所有渠道权重都为 0 时，退回均等分配而非返回无渠道。
func TestSelectChannelBySWRRAllZeroFallback(t *testing.T) {
	resetSWRRSelectors()
	chans := []*Channel{
		chanWithWeight(1, 0, 0),
		chanWithWeight(2, 0, 0),
		chanWithWeight(3, 0, 0),
	}

	counts := map[int]int{}
	for i := 0; i < 99; i++ {
		ch, err := selectChannelBySWRR("g", "m", 0, chans)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		counts[ch.Id]++
	}
	if counts[1] != 33 || counts[2] != 33 || counts[3] != 33 {
		t.Fatalf("all-zero layer should distribute evenly, got #1=%d #2=%d #3=%d", counts[1], counts[2], counts[3])
	}
}

// TestSWRRConcurrentDynamicPool hammers the SWRR picker from many goroutines
// while the candidate pool changes (channels appear/disappear). It verifies that
// the picker never panics, never deadlocks, and never returns an id outside the
// current candidate set.
// 在候选池动态变化（渠道增删）的同时，用大量 goroutine 并发压测 SWRR 选择器，验证
// 不会 panic、不会死锁，且绝不返回当前候选集之外的渠道 id。
func TestSWRRConcurrentDynamicPool(t *testing.T) {
	resetSWRRSelectors()
	key := swrrSelectorKey{group: "g", model: "m", priority: 0}

	// Three rotating candidate sets to simulate channels being disabled/enabled.
	// 三个轮换的候选集，模拟渠道被禁用/启用。
	pools := []map[int]int{
		{1: 70, 2: 20, 3: 10},
		{2: 20, 3: 10}, // #1 disabled
		{1: 70, 3: 10}, // #2 disabled
	}

	const workers = 32
	const iterations = 500
	var wg sync.WaitGroup
	errCh := make(chan error, workers)

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(seed int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				pool := pools[(seed+i)%len(pools)]
				id, ok := globalSWRR.pick(key, pool)
				if !ok {
					errCh <- &swrrTestError{msg: "pick returned ok=false for non-empty pool"}
					return
				}
				if _, inPool := pool[id]; !inPool {
					errCh <- &swrrTestError{msg: "pick returned id not in candidate set"}
					return
				}
			}
		}(w)
	}

	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatal(err)
	}
}

type swrrTestError struct{ msg string }

func (e *swrrTestError) Error() string { return e.msg }
