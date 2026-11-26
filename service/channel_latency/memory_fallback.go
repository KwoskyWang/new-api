package channel_latency

import (
	"sync"

	"github.com/QuantumNous/new-api/setting/operation_setting"
)

// In-process fallback when Redis is disabled (single-node / local).
type memStore struct {
	mu      sync.RWMutex
	windows map[string][]Sample
	scores  map[string]Score
}

var mem = &memStore{
	windows: make(map[string][]Sample),
	scores:  make(map[string]Score),
}

func memKey(model string, channelID int) string {
	return windowKey(model, channelID)
}

func pushMemory(sample Sample) {
	settingWindow := 100
	if s := getWindowSize(); s > 0 {
		settingWindow = s
	}
	key := memKey(sample.Model, sample.ChannelID)
	mem.mu.Lock()
	defer mem.mu.Unlock()
	list := append([]Sample{sample}, mem.windows[key]...)
	if len(list) > settingWindow {
		list = list[:settingWindow]
	}
	mem.windows[key] = list
}

func loadMemory(model string, channelID int) []Sample {
	key := memKey(model, channelID)
	mem.mu.RLock()
	defer mem.mu.RUnlock()
	src := mem.windows[key]
	out := make([]Sample, len(src))
	copy(out, src)
	return out
}

func listMemoryTargets() []scoreTarget {
	mem.mu.RLock()
	defer mem.mu.RUnlock()
	seen := make(map[string]scoreTarget)
	for _, list := range mem.windows {
		if len(list) == 0 {
			continue
		}
		s := list[0]
		id := memKey(s.Model, s.ChannelID)
		seen[id] = scoreTarget{Model: s.Model, ChannelID: s.ChannelID}
	}
	out := make([]scoreTarget, 0, len(seen))
	for _, t := range seen {
		out = append(out, t)
	}
	return out
}

func saveMemoryScore(sc Score) {
	mem.mu.Lock()
	mem.scores[scoreKey(sc.Model, sc.ChannelID)] = sc
	mem.mu.Unlock()
	putCachedScore(sc.Model, sc.ChannelID, sc, true)
}

func loadMemoryScore(model string, channelID int) (Score, bool) {
	mem.mu.RLock()
	defer mem.mu.RUnlock()
	sc, ok := mem.scores[scoreKey(model, channelID)]
	return sc, ok
}

func getWindowSize() int {
	if s := operation_setting.GetChannelLatencySetting(); s != nil && s.WindowSize > 0 {
		return s.WindowSize
	}
	return 100
}
