package model

import (
	"errors"
	"math/rand"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

var ErrNoAvailableFailoverChannel = errors.New("no untried channel available for model failover")

// GetModelFailoverChannel chooses the highest-priority untried channel, using
// weights within that priority. It never changes the request's model or group.
func GetModelFailoverChannel(group, modelName string, excludedIDs []int) (*Channel, error) {
	excluded := make(map[int]bool, len(excludedIDs))
	for _, id := range excludedIDs {
		excluded[id] = true
	}
	var candidates []*Channel
	if common.MemoryCacheEnabled {
		channelSyncLock.RLock()
		defer channelSyncLock.RUnlock()
		ids := group2model2channels[group][modelName]
		if len(ids) == 0 {
			ids = group2model2channels[group][ratio_setting.FormatMatchingModelName(modelName)]
		}
		for _, id := range ids {
			if ch := channelsIDM[id]; ch != nil && ch.Status == common.ChannelStatusEnabled && !excluded[id] {
				candidates = append(candidates, ch)
			}
		}
	} else {
		// Resolve the model before excluding failed channels. Otherwise exhausting
		// an exact model could accidentally fall back to a different wildcard pool.
		var abilities []Ability
		query := DB.Where(commonGroupCol+" = ? AND model = ? AND enabled = ?", group, modelName, true)
		if err := query.Find(&abilities).Error; err != nil {
			return nil, err
		}
		if len(abilities) == 0 {
			if err := DB.Where(commonGroupCol+" = ? AND model = ? AND enabled = ?", group, ratio_setting.FormatMatchingModelName(modelName), true).Find(&abilities).Error; err != nil {
				return nil, err
			}
		}
		ids := make([]int, 0, len(abilities))
		for _, ability := range abilities {
			if !excluded[ability.ChannelId] {
				ids = append(ids, ability.ChannelId)
			}
		}
		if len(ids) > 0 {
			if err := DB.Where("id IN ? AND status = ?", ids, common.ChannelStatusEnabled).Find(&candidates).Error; err != nil {
				return nil, err
			}
		}
	}
	if len(candidates) == 0 {
		return nil, ErrNoAvailableFailoverChannel
	}

	layer := candidates
	if !operation_setting.IsChannelLatencyScoringEnabled() {
		priority := candidates[0].GetPriority()
		for _, ch := range candidates {
			if ch.GetPriority() > priority {
				priority = ch.GetPriority()
			}
		}
		// Collect candidates at the highest priority among the untried channels.
		layer = make([]*Channel, 0, len(candidates))
		for _, ch := range candidates {
			if ch.GetPriority() == priority {
				layer = append(layer, ch)
			}
		}
	}

	weightMap := make(map[int]int, len(layer))
	byID := make(map[int]*Channel, len(layer))
	if operation_setting.IsChannelLatencyScoringEnabled() {
		for _, ch := range layer {
			weightMap[ch.Id] = 1
			byID[ch.Id] = ch
		}
	} else {
		// Weight-0 semantics, consistent with the primary selection path.
		hasPositive := false
		for _, ch := range layer {
			w := ch.GetWeight()
			if w > 0 {
				hasPositive = true
			}
			weightMap[ch.Id] = w
			byID[ch.Id] = ch
		}
		if hasPositive {
			for id, w := range weightMap {
				if w <= 0 {
					delete(weightMap, id)
					delete(byID, id)
				}
			}
		} else {
			for id := range weightMap {
				weightMap[id] = 1
			}
		}
	}
	inCount := len(weightMap)
	if EffectiveWeightHook != nil && len(weightMap) > 0 {
		if tuned := EffectiveWeightHook(group, modelName, weightMap); len(tuned) > 0 {
			weightMap = tuned
			filtered := make(map[int]*Channel, len(tuned))
			for id := range tuned {
				if ch, ok := byID[id]; ok {
					filtered[id] = ch
				}
			}
			byID = filtered
		}
	}
	if len(byID) == 0 {
		logLatencyScoreSelection("failover", group, modelName, inCount, weightMap, 0)
		return nil, ErrNoAvailableFailoverChannel
	}
	if len(byID) == 1 {
		for _, ch := range byID {
			logLatencyScoreSelection("failover", group, modelName, inCount, weightMap, ch.Id)
			return ch, nil
		}
	}
	totalWeight := 0
	for _, w := range weightMap {
		totalWeight += w
	}
	if totalWeight <= 0 {
		logLatencyScoreSelection("failover", group, modelName, inCount, weightMap, 0)
		return nil, ErrNoAvailableFailoverChannel
	}
	weight := rand.Intn(totalWeight)
	for id, w := range weightMap {
		weight -= w
		if weight < 0 {
			logLatencyScoreSelection("failover", group, modelName, inCount, weightMap, id)
			return byID[id], nil
		}
	}
	logLatencyScoreSelection("failover", group, modelName, inCount, weightMap, 0)
	return nil, ErrNoAvailableFailoverChannel
}
