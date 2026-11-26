package channel_latency

import (
	"sync"
	"time"

	"github.com/QuantumNous/new-api/setting/operation_setting"
)

// Short-lived local cache so select-path score/probe checks avoid Redis RTT every request.
type scoreCacheEntry struct {
	score   Score
	ok      bool
	expires time.Time
}

type probeCacheEntry struct {
	in      bool
	expires time.Time
}

var routingCache = struct {
	mu     sync.RWMutex
	scores map[string]scoreCacheEntry
	probe  map[string]probeCacheEntry
}{
	scores: make(map[string]scoreCacheEntry),
	probe:  make(map[string]probeCacheEntry),
}

func routingCacheTTL() time.Duration {
	sec := 2
	if s := operation_setting.GetChannelLatencySetting(); s != nil {
		if s.RoutingCacheSeconds < 0 {
			return 0
		}
		sec = s.RoutingCacheSeconds
	}
	if sec <= 0 {
		return 0
	}
	return time.Duration(sec) * time.Second
}

func getCachedScore(model string, channelID int) (Score, bool, bool) {
	ttl := routingCacheTTL()
	if ttl <= 0 {
		return Score{}, false, false
	}
	key := scoreKey(model, channelID)
	now := time.Now()
	routingCache.mu.RLock()
	e, ok := routingCache.scores[key]
	routingCache.mu.RUnlock()
	if !ok || now.After(e.expires) {
		return Score{}, false, false
	}
	return e.score, e.ok, true
}

func putCachedScore(model string, channelID int, sc Score, ok bool) {
	ttl := routingCacheTTL()
	if ttl <= 0 {
		return
	}
	key := scoreKey(model, channelID)
	routingCache.mu.Lock()
	routingCache.scores[key] = scoreCacheEntry{score: sc, ok: ok, expires: time.Now().Add(ttl)}
	routingCache.mu.Unlock()
}

func invalidateCachedScore(model string, channelID int) {
	routingCache.mu.Lock()
	delete(routingCache.scores, scoreKey(model, channelID))
	routingCache.mu.Unlock()
}

func clearRoutingCache() {
	routingCache.mu.Lock()
	routingCache.scores = make(map[string]scoreCacheEntry)
	routingCache.probe = make(map[string]probeCacheEntry)
	routingCache.mu.Unlock()
}

func getCachedProbe(model string, channelID int) (in bool, hit bool) {
	ttl := routingCacheTTL()
	if ttl <= 0 {
		return false, false
	}
	key := targetMember(model, channelID)
	now := time.Now()
	routingCache.mu.RLock()
	e, ok := routingCache.probe[key]
	routingCache.mu.RUnlock()
	if !ok || now.After(e.expires) {
		return false, false
	}
	return e.in, true
}

func putCachedProbe(model string, channelID int, in bool) {
	ttl := routingCacheTTL()
	if ttl <= 0 {
		return
	}
	key := targetMember(model, channelID)
	routingCache.mu.Lock()
	routingCache.probe[key] = probeCacheEntry{in: in, expires: time.Now().Add(ttl)}
	routingCache.mu.Unlock()
}
