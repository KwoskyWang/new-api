package channel_latency

import (
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/bytedance/gopkg/util/gopool"
)

func startProbeLoop() {
	gopool.Go(func() {
		for {
			setting := operation_setting.GetChannelLatencySetting()
			sec := setting.ProbeIntervalSeconds
			if sec <= 0 {
				sec = 300
			}
			time.Sleep(time.Duration(sec) * time.Second)
			if !operation_setting.IsChannelLatencyScoringEnabled() {
				continue
			}
			runProbeRound()
		}
	})
}

func runProbeRound() {
	if ProbeFunc == nil {
		return
	}
	pool := ListProbePool()
	if len(pool) == 0 {
		return
	}
	common.SysLog(fmt.Sprintf("latency-probe round start count=%d", len(pool)))
	for _, item := range pool {
		modelName := item.Model
		channelID := item.ChannelID
		success, statusCode, err := ProbeFunc(channelID, modelName)
		if err != nil {
			common.SysLog(fmt.Sprintf(
				"latency-probe error model=%s channel=#%d err=%v",
				modelName, channelID, err,
			))
			markProbeResult(modelName, channelID, false, statusCode)
			continue
		}
		markProbeResult(modelName, channelID, success, statusCode)
	}
}
