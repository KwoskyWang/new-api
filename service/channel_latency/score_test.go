package channel_latency

import (
	"testing"

	"github.com/QuantumNous/new-api/setting/operation_setting"
)

func TestComputeScoreSuccessDominates(t *testing.T) {
	good := make([]Sample, 0, 10)
	for i := 0; i < 10; i++ {
		good = append(good, Sample{Success: true, TWaitMs: 200, TTotalMs: 800})
	}
	bad := make([]Sample, 0, 10)
	for i := 0; i < 10; i++ {
		bad = append(bad, Sample{Success: false, TWaitMs: 200, TTotalMs: 800})
	}
	sg := computeScore("m", 1, good)
	sb := computeScore("m", 2, bad)
	if sg.Score <= sb.Score {
		t.Fatalf("expected good score > bad score, got %v vs %v", sg.Score, sb.Score)
	}
}

func TestApplyEffectiveWeightsScoreOnlyIgnoresAdminWeight(t *testing.T) {
	setting := operation_setting.GetChannelLatencySetting()
	prevEnabled := setting.Enabled
	prevMin := setting.MinSamples
	prevThr := setting.SoftExcludeThreshold
	setting.Enabled = true
	setting.MinSamples = 1
	setting.SoftExcludeThreshold = 0.5
	defer func() {
		setting.Enabled = prevEnabled
		setting.MinSamples = prevMin
		setting.SoftExcludeThreshold = prevThr
	}()

	clearRoutingCache()
	removeFromProbePool("m", 1)
	removeFromProbePool("m", 2)
	removeFromProbePool("m", 3)

	saveMemoryScore(Score{Model: "m", ChannelID: 1, Samples: 10, Score: 0.9})
	saveMemoryScore(Score{Model: "m", ChannelID: 2, Samples: 10, Score: 0.2})
	saveMemoryScore(Score{Model: "m", ChannelID: 3, Samples: 10, Score: 0.1})

	// Admin weights would have preferred channel 3; score-only must ignore them.
	out := ApplyEffectiveWeights("g", "m", map[int]int{1: 1, 2: 100, 3: 1000})
	if _, ok := out[3]; ok {
		t.Fatalf("channel 3 should be soft-excluded, got %v", out)
	}
	if out[1] <= out[2] {
		t.Fatalf("higher score should get higher weight: %v", out)
	}
	if out[1] != scoreToWeight(0.9) {
		t.Fatalf("expected score-derived weight, got %v want %v", out[1], scoreToWeight(0.9))
	}
}

func TestApplyEffectiveWeightsExcludesProbePool(t *testing.T) {
	setting := operation_setting.GetChannelLatencySetting()
	prevEnabled := setting.Enabled
	prevMin := setting.MinSamples
	prevThr := setting.SoftExcludeThreshold
	prevFail := setting.ProbeFailThreshold
	setting.Enabled = true
	setting.MinSamples = 1
	setting.SoftExcludeThreshold = 0
	setting.ProbeFailThreshold = 2
	defer func() {
		setting.Enabled = prevEnabled
		setting.MinSamples = prevMin
		setting.SoftExcludeThreshold = prevThr
		setting.ProbeFailThreshold = prevFail
		removeFromProbePool("m", 1)
		removeFromProbePool("m", 2)
	}()

	clearRoutingCache()
	saveMemoryScore(Score{Model: "m", ChannelID: 1, Samples: 5, Score: 0.9})
	saveMemoryScore(Score{Model: "m", ChannelID: 2, Samples: 5, Score: 0.8})

	markTrafficResult("m", 2, false, 403)
	out1 := ApplyEffectiveWeights("g", "m", map[int]int{1: 1, 2: 1})
	if _, ok := out1[2]; !ok {
		t.Fatalf("single failure should not quarantine yet, got %v", out1)
	}

	markTrafficResult("m", 2, false, 403)
	out := ApplyEffectiveWeights("g", "m", map[int]int{1: 1, 2: 1})
	if _, ok := out[2]; ok {
		t.Fatalf("channel 2 should be probe-excluded after 2 fails, got %v", out)
	}
	if out[1] != scoreToWeight(0.9) {
		t.Fatalf("expected channel 1 kept, got %v", out)
	}

	markProbeResult("m", 2, true, 200)
	out2 := ApplyEffectiveWeights("g", "m", map[int]int{1: 1, 2: 1})
	if _, ok := out2[2]; !ok {
		t.Fatalf("channel 2 should recover after probe success, got %v", out2)
	}
}

func TestApplyEffectiveWeightsFallbackWhenAllExcluded(t *testing.T) {
	setting := operation_setting.GetChannelLatencySetting()
	prevEnabled := setting.Enabled
	prevMin := setting.MinSamples
	prevThr := setting.SoftExcludeThreshold
	setting.Enabled = true
	setting.MinSamples = 1
	setting.SoftExcludeThreshold = 0.5
	defer func() {
		setting.Enabled = prevEnabled
		setting.MinSamples = prevMin
		setting.SoftExcludeThreshold = prevThr
	}()

	clearRoutingCache()
	removeFromProbePool("m", 1)
	removeFromProbePool("m", 2)
	saveMemoryScore(Score{Model: "m", ChannelID: 1, Samples: 10, Score: 0.1})
	saveMemoryScore(Score{Model: "m", ChannelID: 2, Samples: 10, Score: 0.05})

	orig := map[int]int{1: 10, 2: 20}
	out := ApplyEffectiveWeights("g", "m", orig)
	if len(out) != 2 || out[1] != scoreWeightScale || out[2] != scoreWeightScale {
		t.Fatalf("expected equal fallback weights, got %v", out)
	}
}
