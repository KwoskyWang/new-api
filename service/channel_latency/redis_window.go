package channel_latency

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

const (
	redisKeyPrefix = "chlat"
	redisKeysSet   = redisKeyPrefix + ":keys"
	redisWindowTTL = 48 * time.Hour
	redisScoreTTL  = 48 * time.Hour
)

// Hot keys are (model, channel) only. Group is not part of routing score identity:
// select path already has candidate channel IDs for the request group.
func windowKey(model string, channelID int) string {
	return fmt.Sprintf("%s:win:%s:%d", redisKeyPrefix, model, channelID)
}

func scoreKey(model string, channelID int) string {
	return fmt.Sprintf("%s:score:%s:%d", redisKeyPrefix, model, channelID)
}

func targetMember(model string, channelID int) string {
	return fmt.Sprintf("%s|%d", model, channelID)
}

func pushSample(sample Sample) {
	if !common.RedisEnabled || common.RDB == nil {
		pushMemory(sample)
		return
	}
	setting := operation_setting.GetChannelLatencySetting()
	window := setting.WindowSize
	if window <= 0 {
		window = 100
	}
	key := windowKey(sample.Model, sample.ChannelID)
	payload, err := common.Marshal(sample)
	if err != nil {
		return
	}
	ctx := context.Background()
	pipe := common.RDB.TxPipeline()
	pipe.LPush(ctx, key, string(payload))
	pipe.LTrim(ctx, key, 0, int64(window-1))
	pipe.Expire(ctx, key, redisWindowTTL)
	pipe.SAdd(ctx, redisKeysSet, targetMember(sample.Model, sample.ChannelID))
	pipe.Expire(ctx, redisKeysSet, redisWindowTTL)
	_, _ = pipe.Exec(ctx)
}

// LoadWindow returns the sliding-window samples for observation APIs.
func LoadWindow(model string, channelID int) ([]Sample, error) {
	return loadWindow(model, channelID)
}

func loadWindow(model string, channelID int) ([]Sample, error) {
	if !common.RedisEnabled || common.RDB == nil {
		return loadMemory(model, channelID), nil
	}
	setting := operation_setting.GetChannelLatencySetting()
	window := setting.WindowSize
	if window <= 0 {
		window = 100
	}
	ctx := context.Background()
	vals, err := common.RDB.LRange(ctx, windowKey(model, channelID), 0, int64(window-1)).Result()
	if err != nil {
		return nil, err
	}
	out := make([]Sample, 0, len(vals))
	for _, v := range vals {
		var s Sample
		if err := common.Unmarshal([]byte(v), &s); err != nil {
			continue
		}
		out = append(out, s)
	}
	return out, nil
}

func listScoreTargets() []scoreTarget {
	if !common.RedisEnabled || common.RDB == nil {
		return listMemoryTargets()
	}
	ctx := context.Background()
	members, err := common.RDB.SMembers(ctx, redisKeysSet).Result()
	if err != nil {
		return nil
	}
	out := make([]scoreTarget, 0, len(members))
	for _, m := range members {
		modelName, channelID, ok := parseTargetMember(m)
		if !ok {
			continue
		}
		out = append(out, scoreTarget{Model: modelName, ChannelID: channelID})
	}
	return out
}

func parseTargetMember(s string) (model string, channelID int, ok bool) {
	// Current: model|channelID (model may contain '|' rarely — split from right).
	idx := strings.LastIndexByte(s, '|')
	if idx <= 0 || idx >= len(s)-1 {
		return "", 0, false
	}
	model = s[:idx]
	id, err := strconv.Atoi(s[idx+1:])
	if err != nil || model == "" || id <= 0 {
		return "", 0, false
	}
	return model, id, true
}

func saveScore(sc Score) {
	saveMemoryScore(sc)
	putCachedScore(sc.Model, sc.ChannelID, sc, true)
	if !common.RedisEnabled || common.RDB == nil {
		return
	}
	payload, err := common.Marshal(sc)
	if err != nil {
		return
	}
	_ = common.RedisSet(scoreKey(sc.Model, sc.ChannelID), string(payload), redisScoreTTL)
}

func loadScore(model string, channelID int) (Score, bool) {
	if sc, ok, hit := getCachedScore(model, channelID); hit {
		return sc, ok
	}
	if !common.RedisEnabled || common.RDB == nil {
		sc, ok := loadMemoryScore(model, channelID)
		putCachedScore(model, channelID, sc, ok)
		return sc, ok
	}
	val, err := common.RedisGet(scoreKey(model, channelID))
	if err != nil || val == "" {
		putCachedScore(model, channelID, Score{}, false)
		return Score{}, false
	}
	var sc Score
	if err := common.Unmarshal([]byte(val), &sc); err != nil {
		putCachedScore(model, channelID, Score{}, false)
		return Score{}, false
	}
	putCachedScore(model, channelID, sc, true)
	return sc, true
}

type scoreTarget struct {
	Model     string
	ChannelID int
}
