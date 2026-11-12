package model

import (
	"errors"
	"math/rand"

	"github.com/QuantumNous/new-api/common"
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
	priority := candidates[0].GetPriority()
	for _, ch := range candidates {
		if ch.GetPriority() > priority {
			priority = ch.GetPriority()
		}
	}
	targets := make([]*Channel, 0, len(candidates))
	totalWeight := 0
	for _, ch := range candidates {
		if ch.GetPriority() == priority {
			targets = append(targets, ch)
			totalWeight += ch.GetWeight()
		}
	}
	if totalWeight == 0 {
		return targets[rand.Intn(len(targets))], nil
	}
	weight := rand.Intn(totalWeight)
	for _, ch := range targets {
		weight -= ch.GetWeight()
		if weight < 0 {
			return ch, nil
		}
	}
	return nil, ErrNoAvailableFailoverChannel
}
