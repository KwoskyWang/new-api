package channel_latency

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/setting/operation_setting"
)

func TestRoutingCacheHitAvoidsMissSemantics(t *testing.T) {
	setting := operation_setting.GetChannelLatencySetting()
	prev := setting.RoutingCacheSeconds
	setting.RoutingCacheSeconds = 2
	defer func() { setting.RoutingCacheSeconds = prev }()

	putCachedScore("m", 7, Score{Model: "m", ChannelID: 7, Score: 0.77, Samples: 3}, true)
	sc, ok, hit := getCachedScore("m", 7)
	if !hit || !ok || sc.Score != 0.77 {
		t.Fatalf("expected cache hit, got hit=%v ok=%v sc=%v", hit, ok, sc)
	}

	putCachedProbe("m", 7, true)
	in, hit := getCachedProbe("m", 7)
	if !hit || !in {
		t.Fatalf("expected probe cache true, got hit=%v in=%v", hit, in)
	}

	// Expire by rewriting with past expiry via short TTL wait when TTL=0 path disabled.
	setting.RoutingCacheSeconds = 0
	if _, _, hit := getCachedScore("m", 7); hit {
		t.Fatalf("cache should be disabled when RoutingCacheSeconds=0")
	}
	_ = time.Second // keep time import used if build tags change
}
