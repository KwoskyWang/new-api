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

	// Collect candidates at the highest priority among the untried channels.
	// 在未尝试的渠道中收集最高优先级层的候选。
	layer := make([]*Channel, 0, len(candidates))
	for _, ch := range candidates {
		if ch.GetPriority() == priority {
			layer = append(layer, ch)
		}
	}

	// Weight-0 semantics, consistent with the primary selection path:
	// a weight of 0 means "do not participate". Zero-weight channels are excluded
	// as long as at least one channel in this layer has a positive weight. If every
	// channel in the layer is weight 0, fall back to an even random choice so the
	// failover does not stall. The random weighted algorithm itself is kept for
	// failover (not switched to SWRR), per configuration.
	// 权重 0 语义，与主选择路径保持一致：权重 0 表示"不参与"。只要该层至少有一个渠道
	// 权重 > 0，权重 0 的渠道会被排除；若该层全部为 0 权重，则兜底为均匀随机选择，避免
	// failover 卡住。failover 仍保留随机加权算法（未切换为 SWRR）。
	targets := make([]*Channel, 0, len(layer))
	totalWeight := 0
	for _, ch := range layer {
		if ch.GetWeight() > 0 {
			targets = append(targets, ch)
			totalWeight += ch.GetWeight()
		}
	}
	if totalWeight == 0 {
		// All channels in this layer have weight 0: distribute evenly.
		// 该层全部为 0 权重：均匀分配。
		return layer[rand.Intn(len(layer))], nil
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
