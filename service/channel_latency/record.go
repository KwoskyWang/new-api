package channel_latency

import (
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/relaytiming"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/bytedance/gopkg/util/gopool"
)

// IsCollectEnabled reports whether attempt timing should be collected for scoring.
func IsCollectEnabled() bool {
	return operation_setting.IsChannelLatencyScoringEnabled()
}

// RecordAttemptOutcome records only success/fail for probe quarantine (no timing window).
func RecordAttemptOutcome(info *relaycommon.RelayInfo, success bool, statusCode int) {
	if !IsCollectEnabled() || info == nil {
		return
	}
	modelName := info.OriginModelName
	if modelName == "" {
		return
	}
	channelID := 0
	if info.ChannelMeta != nil {
		channelID = info.ChannelId
	}
	if channelID <= 0 {
		return
	}
	gopool.Go(func() {
		markTrafficResult(modelName, channelID, success, statusCode)
	})
}

// RecordAttemptSample records one finished upstream attempt asynchronously.
func RecordAttemptSample(info *relaycommon.RelayInfo, attempt *relaytiming.Attempt, success bool, statusCode int) {
	if !IsCollectEnabled() || info == nil || attempt == nil {
		return
	}
	m := attempt.Metrics()
	group := info.UsingGroup
	if group == "" {
		group = info.TokenGroup
	}
	if group == "" {
		group = "default"
	}
	modelName := info.OriginModelName
	if modelName == "" {
		return
	}
	channelID := m.ChannelId
	if channelID <= 0 && info.ChannelMeta != nil {
		channelID = info.ChannelId
	}
	if channelID <= 0 {
		return
	}
	sample := Sample{
		Timestamp:   time.Now(),
		RequestID:   info.RequestId,
		Group:       group,
		Model:       modelName,
		ChannelID:   channelID,
		ChannelType: m.ChannelType,
		RetryIndex:  m.Index,
		Success:     success,
		StatusCode:  statusCode,
		ErrorStage:  m.ErrorStage,
		ReqBytes:    m.RequestBodySize,
		RespBytes:   m.ResponseBodySize,
		TSendMs:     m.TSendMs,
		TWaitMs:     m.TWaitMs,
		TRecvMs:     m.TRecvMs,
		TTotalMs:    m.TTotalMs,
	}
	if sample.ReqBytes < 0 {
		sample.ReqBytes = 0
	}
	if sample.RespBytes < 0 {
		sample.RespBytes = 0
	}
	gopool.Go(func() {
		pushSample(sample)
		enqueueDetail(sample)
		markTrafficResult(sample.Model, sample.ChannelID, sample.Success, sample.StatusCode)
	})
}

// Init registers the SWRR weight hook and starts score + probe + DB retention workers.
func Init() {
	model.EffectiveWeightHook = ApplyEffectiveWeights
	startRetentionLoop()
	startProbeLoop()
	gopool.Go(func() {
		for {
			setting := operation_setting.GetChannelLatencySetting()
			sec := setting.ScoreIntervalSeconds
			if sec <= 0 {
				sec = 60
			}
			time.Sleep(time.Duration(sec) * time.Second)
			if !operation_setting.IsChannelLatencyScoringEnabled() {
				continue
			}
			RecomputeAll()
		}
	})
	common.SysLog("channel latency scoring worker started")
}
