package channel_latency

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

const (
	redisProbeSet = redisKeyPrefix + ":probe"
	redisLastTTL  = 48 * time.Hour
)

// LastResult is the latest success/fail outcome for a (model, channel).
type LastResult struct {
	Model            string `json:"model"`
	ChannelID        int    `json:"channel_id"`
	Success          bool   `json:"success"`
	StatusCode       int    `json:"status_code,omitempty"`
	ConsecutiveFails int    `json:"consecutive_fails,omitempty"`
	UpdatedAt        int64  `json:"updated_at"`
	Source           string `json:"source,omitempty"` // traffic | probe
}

// ProbeFunc runs a max_tokens=1 connectivity check for one (channel, model).
// Registered from controller to avoid service → controller import cycles.
var ProbeFunc func(channelID int, modelName string) (success bool, statusCode int, err error)

func lastKey(model string, channelID int) string {
	return fmt.Sprintf("%s:last:%s:%d", redisKeyPrefix, model, channelID)
}

// IsInProbePool reports whether (model, channel) is quarantined from routing.
func IsInProbePool(model string, channelID int) bool {
	if model == "" || channelID <= 0 {
		return false
	}
	if in, hit := getCachedProbe(model, channelID); hit {
		return in
	}
	if !common.RedisEnabled || common.RDB == nil {
		in := memIsInProbe(model, channelID)
		putCachedProbe(model, channelID, in)
		return in
	}
	ctx := context.Background()
	ok, err := common.RDB.SIsMember(ctx, redisProbeSet, targetMember(model, channelID)).Result()
	if err != nil {
		in := memIsInProbe(model, channelID)
		putCachedProbe(model, channelID, in)
		return in
	}
	putCachedProbe(model, channelID, ok)
	return ok
}

// ListProbePool returns quarantined (model, channel) pairs.
func ListProbePool() []LastResult {
	if !common.RedisEnabled || common.RDB == nil {
		return memListProbe()
	}
	ctx := context.Background()
	members, err := common.RDB.SMembers(ctx, redisProbeSet).Result()
	if err != nil {
		return memListProbe()
	}
	out := make([]LastResult, 0, len(members))
	for _, m := range members {
		modelName, channelID, ok := parseTargetMember(m)
		if !ok {
			continue
		}
		lr, found := loadLastResult(modelName, channelID)
		if !found {
			lr = LastResult{Model: modelName, ChannelID: channelID, Success: false}
		}
		out = append(out, lr)
	}
	return out
}

func probeFailThreshold() int {
	n := 2
	if s := operation_setting.GetChannelLatencySetting(); s != nil && s.ProbeFailThreshold > 0 {
		n = s.ProbeFailThreshold
	}
	return n
}

func markTrafficResult(model string, channelID int, success bool, statusCode int) {
	if model == "" || channelID <= 0 {
		return
	}
	prev, _ := loadLastResult(model, channelID)
	fails := 0
	if !success {
		fails = prev.ConsecutiveFails + 1
	}
	lr := LastResult{
		Model:            model,
		ChannelID:        channelID,
		Success:          success,
		StatusCode:       statusCode,
		ConsecutiveFails: fails,
		UpdatedAt:        time.Now().Unix(),
		Source:           "traffic",
	}
	saveLastResult(lr)
	if success {
		removeFromProbePool(model, channelID)
		return
	}
	threshold := probeFailThreshold()
	if fails < threshold {
		common.SysLog(fmt.Sprintf(
			"latency-probe fail streak model=%s channel=#%d status=%d streak=%d/%d (not quarantined yet)",
			model, channelID, statusCode, fails, threshold,
		))
		return
	}
	addToProbePool(model, channelID)
	common.SysLog(fmt.Sprintf(
		"latency-probe quarantine add model=%s channel=#%d status=%d streak=%d",
		model, channelID, statusCode, fails,
	))
}

func markProbeResult(model string, channelID int, success bool, statusCode int) {
	if model == "" || channelID <= 0 {
		return
	}
	fails := 0
	if !success {
		prev, _ := loadLastResult(model, channelID)
		fails = prev.ConsecutiveFails
		if fails < 1 {
			fails = 1
		}
	}
	lr := LastResult{
		Model:            model,
		ChannelID:        channelID,
		Success:          success,
		StatusCode:       statusCode,
		ConsecutiveFails: fails,
		UpdatedAt:        time.Now().Unix(),
		Source:           "probe",
	}
	saveLastResult(lr)
	if success {
		removeFromProbePool(model, channelID)
		common.SysLog(fmt.Sprintf(
			"latency-probe recovered model=%s channel=#%d status=%d",
			model, channelID, statusCode,
		))
	} else {
		addToProbePool(model, channelID)
		common.SysLog(fmt.Sprintf(
			"latency-probe still down model=%s channel=#%d status=%d",
			model, channelID, statusCode,
		))
	}
}

func saveLastResult(lr LastResult) {
	memSaveLast(lr)
	if !common.RedisEnabled || common.RDB == nil {
		return
	}
	payload, err := common.Marshal(lr)
	if err != nil {
		return
	}
	_ = common.RedisSet(lastKey(lr.Model, lr.ChannelID), string(payload), redisLastTTL)
}

func loadLastResult(model string, channelID int) (LastResult, bool) {
	if !common.RedisEnabled || common.RDB == nil {
		return memLoadLast(model, channelID)
	}
	val, err := common.RedisGet(lastKey(model, channelID))
	if err != nil || val == "" {
		return memLoadLast(model, channelID)
	}
	var lr LastResult
	if err := common.Unmarshal([]byte(val), &lr); err != nil {
		return LastResult{}, false
	}
	return lr, true
}

func addToProbePool(model string, channelID int) {
	memAddProbe(model, channelID)
	putCachedProbe(model, channelID, true)
	if !common.RedisEnabled || common.RDB == nil {
		return
	}
	ctx := context.Background()
	pipe := common.RDB.TxPipeline()
	pipe.SAdd(ctx, redisProbeSet, targetMember(model, channelID))
	pipe.Expire(ctx, redisProbeSet, redisLastTTL)
	_, _ = pipe.Exec(ctx)
}

func removeFromProbePool(model string, channelID int) {
	memRemoveProbe(model, channelID)
	putCachedProbe(model, channelID, false)
	if !common.RedisEnabled || common.RDB == nil {
		return
	}
	ctx := context.Background()
	_ = common.RDB.SRem(ctx, redisProbeSet, targetMember(model, channelID)).Err()
}

var probeMem = struct {
	mu    sync.RWMutex
	last  map[string]LastResult
	probe map[string]struct{}
}{
	last:  make(map[string]LastResult),
	probe: make(map[string]struct{}),
}

func memSaveLast(lr LastResult) {
	probeMem.mu.Lock()
	defer probeMem.mu.Unlock()
	probeMem.last[lastKey(lr.Model, lr.ChannelID)] = lr
}

func memLoadLast(model string, channelID int) (LastResult, bool) {
	probeMem.mu.RLock()
	defer probeMem.mu.RUnlock()
	lr, ok := probeMem.last[lastKey(model, channelID)]
	return lr, ok
}

func memAddProbe(model string, channelID int) {
	probeMem.mu.Lock()
	defer probeMem.mu.Unlock()
	probeMem.probe[targetMember(model, channelID)] = struct{}{}
}

func memRemoveProbe(model string, channelID int) {
	probeMem.mu.Lock()
	defer probeMem.mu.Unlock()
	delete(probeMem.probe, targetMember(model, channelID))
}

func memIsInProbe(model string, channelID int) bool {
	probeMem.mu.RLock()
	defer probeMem.mu.RUnlock()
	_, ok := probeMem.probe[targetMember(model, channelID)]
	return ok
}

func memListProbe() []LastResult {
	probeMem.mu.RLock()
	defer probeMem.mu.RUnlock()
	out := make([]LastResult, 0, len(probeMem.probe))
	for m := range probeMem.probe {
		modelName, channelID, ok := parseTargetMember(m)
		if !ok {
			continue
		}
		lr, found := probeMem.last[lastKey(modelName, channelID)]
		if !found {
			lr = LastResult{Model: modelName, ChannelID: channelID, Success: false}
		}
		out = append(out, lr)
	}
	return out
}
