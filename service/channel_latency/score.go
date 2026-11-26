package channel_latency

import (
	"math"
	"sort"
	"time"

	"github.com/QuantumNous/new-api/setting/operation_setting"
)

// scoreWeightScale converts Score∈(0,1] to an integer SWRR weight.
const scoreWeightScale = 1000

func computeScore(model string, channelID int, samples []Sample) Score {
	sc := Score{
		Model:     model,
		ChannelID: channelID,
		Samples:   len(samples),
		Score:     1,
		UpdatedAt: time.Now().Unix(),
	}
	if len(samples) == 0 {
		return sc
	}
	success := 0
	waits := make([]int64, 0, len(samples))
	totals := make([]int64, 0, len(samples))
	for _, s := range samples {
		if s.Success {
			success++
		}
		waits = append(waits, s.TWaitMs)
		totals = append(totals, s.TTotalMs)
	}
	sc.SuccessRate = float64(success) / float64(len(samples))
	sc.P50WaitMs = percentile(waits, 0.50)
	sc.P95WaitMs = percentile(waits, 0.95)
	sc.P95TotalMs = percentile(totals, 0.95)

	latFactor := 1.0 / (1.0 + float64(sc.P95WaitMs)/2000.0)
	totalFactor := 1.0 / (1.0 + float64(sc.P95TotalMs)/8000.0)
	sc.Score = clamp01(sc.SuccessRate*0.65 + latFactor*0.25 + totalFactor*0.10)
	if sc.Score <= 0 {
		sc.Score = 0.01
	}
	return sc
}

func percentile(vals []int64, p float64) int64 {
	if len(vals) == 0 {
		return 0
	}
	cp := append([]int64(nil), vals...)
	sort.Slice(cp, func(i, j int) bool { return cp[i] < cp[j] })
	if p <= 0 {
		return cp[0]
	}
	if p >= 1 {
		return cp[len(cp)-1]
	}
	idx := int(math.Ceil(p*float64(len(cp)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(cp) {
		idx = len(cp) - 1
	}
	return cp[idx]
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func scoreToWeight(score float64) int {
	w := int(math.Round(score * scoreWeightScale))
	if w < 1 {
		w = 1
	}
	return w
}

// RecomputeAll refreshes hot scores for all known (model, channel) windows.
func RecomputeAll() {
	if !operation_setting.IsChannelLatencyScoringEnabled() {
		return
	}
	targets := listScoreTargets()
	for _, t := range targets {
		samples, err := loadWindow(t.Model, t.ChannelID)
		if err != nil || len(samples) == 0 {
			continue
		}
		sc := computeScore(t.Model, t.ChannelID, samples)
		saveScore(sc)
	}
}

// routingScore returns the score used for routing a channel.
// Score is computed from whatever samples are in the window (need not reach WindowSize).
// Missing score / below MinSamples → 1.0 (equal share among unknowns).
func routingScore(model string, channelID int) (score float64, exclude bool) {
	setting := operation_setting.GetChannelLatencySetting()
	minSamples := setting.MinSamples
	if minSamples < 1 {
		minSamples = 1
	}
	sc, ok := loadScore(model, channelID)
	if !ok || sc.Samples < minSamples {
		return 1, false
	}
	if setting.SoftExcludeThreshold > 0 && sc.Score < setting.SoftExcludeThreshold {
		return sc.Score, true
	}
	if sc.Score <= 0 {
		return 0.01, false
	}
	return sc.Score, false
}

// GetWeightMultiplier kept for compatibility; returns score as "multiplier".
// Admin weight is ignored by ApplyEffectiveWeights when scoring is on.
func GetWeightMultiplier(group, model string, channelID int) (multiplier float64, exclude bool) {
	_ = group
	if !operation_setting.IsChannelLatencyScoringEnabled() {
		return 1, false
	}
	return routingScore(model, channelID)
}

// ApplyEffectiveWeights builds SWRR weights from latency scores only.
// Incoming map keys are candidate channel IDs; admin weight values are ignored.
// Channels whose last result failed (probe pool) are hard-excluded until probe recovers.
func ApplyEffectiveWeights(group, model string, weights map[int]int) map[int]int {
	_ = group
	if !operation_setting.IsChannelLatencyScoringEnabled() || len(weights) == 0 {
		return weights
	}
	out := make(map[int]int, len(weights))
	for id := range weights {
		if IsInProbePool(model, id) {
			continue
		}
		sc, exclude := routingScore(model, id)
		if exclude {
			continue
		}
		out[id] = scoreToWeight(sc)
	}
	if len(out) == 0 {
		// All quarantined / soft-excluded: equal share so traffic does not stall.
		for id := range weights {
			out[id] = scoreWeightScale
		}
	}
	return out
}

// ListScores returns hot scores, optionally filtered by model.
func ListScores(group, model string) []Score {
	_ = group
	targets := listScoreTargets()
	out := make([]Score, 0, len(targets))
	for _, t := range targets {
		if model != "" && t.Model != model {
			continue
		}
		sc, ok := loadScore(t.Model, t.ChannelID)
		if !ok {
			samples, err := loadWindow(t.Model, t.ChannelID)
			if err != nil || len(samples) == 0 {
				continue
			}
			sc = computeScore(t.Model, t.ChannelID, samples)
		}
		out = append(out, sc)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Model != out[j].Model {
			return out[i].Model < out[j].Model
		}
		return out[i].ChannelID < out[j].ChannelID
	})
	return out
}
