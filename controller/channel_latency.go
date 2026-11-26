package controller

import (
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	channellatency "github.com/QuantumNous/new-api/service/channel_latency"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
)

// GetChannelLatencyScores returns hot latency scores used for soft weight tuning.
// GET /api/channel/latency/scores?group=&model=&recompute=1
func GetChannelLatencyScores(c *gin.Context) {
	setting := operation_setting.GetChannelLatencySetting()
	if c.Query("recompute") == "1" || c.Query("recompute") == "true" {
		channellatency.RecomputeAll()
	}
	scores := channellatency.ListScores(c.Query("group"), c.Query("model"))
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"enabled":                setting.Enabled,
			"detail_db_enabled":      setting.DetailDBEnabled,
			"detail_retention_days":  setting.DetailRetentionDays,
			"window_size":            setting.WindowSize,
			"score_interval_seconds": setting.ScoreIntervalSeconds,
			"min_samples":             setting.MinSamples,
			"soft_exclude_threshold":  setting.SoftExcludeThreshold,
			"probe_interval_seconds":  setting.ProbeIntervalSeconds,
			"probe_fail_threshold":    setting.ProbeFailThreshold,
			"routing_cache_seconds":   setting.RoutingCacheSeconds,
			"trace_sample_percent":    setting.TraceSamplePercent,
			"scores":                  scores,
			"count":                   len(scores),
			"probe_pool":              channellatency.ListProbePool(),
		},
	})
}

// GetChannelLatencyWindow returns the sliding-window samples for one (model, channel).
// GET /api/channel/latency/window?model=&channel_id=
func GetChannelLatencyWindow(c *gin.Context) {
	modelName := c.Query("model")
	channelID, _ := strconv.Atoi(c.Query("channel_id"))
	if modelName == "" || channelID <= 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "model and channel_id are required",
		})
		return
	}
	samples, err := channellatency.LoadWindow(modelName, channelID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"model":      modelName,
			"channel_id": channelID,
			"samples":    samples,
			"count":      len(samples),
		},
	})
}

// GetChannelLatencyDetails returns persisted detail rows from LOG_DB.
// GET /api/channel/latency/details?group=&model=&channel_id=&limit=
func GetChannelLatencyDetails(c *gin.Context) {
	channelID, _ := strconv.Atoi(c.Query("channel_id"))
	limit, _ := strconv.Atoi(c.Query("limit"))
	rows, err := model.QueryChannelLatencySamples(c.Query("group"), c.Query("model"), channelID, limit)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"rows":  rows,
			"count": len(rows),
		},
	})
}

// ForceChannelLatencyRecompute triggers an immediate score recompute (admin).
func ForceChannelLatencyRecompute(c *gin.Context) {
	if !operation_setting.IsChannelLatencyScoringEnabled() {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "channel latency scoring is disabled",
		})
		return
	}
	channellatency.RecomputeAll()
	common.SysLog("channel latency scores recomputed by admin")
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "recomputed",
		"data":    channellatency.ListScores(c.Query("group"), c.Query("model")),
	})
}
