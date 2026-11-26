package model

import (
	"time"

	"github.com/QuantumNous/new-api/common"
)

// ChannelLatencySample stores one upstream attempt's sizes + timings (no body content).
// Written asynchronously to LOG_DB; never on the relay hot path.
type ChannelLatencySample struct {
	Id          int64     `json:"id" gorm:"primaryKey;autoIncrement"`
	CreatedAt   time.Time `json:"created_at" gorm:"index:idx_chlat_created,priority:1"`
	RequestId   string    `json:"request_id" gorm:"type:varchar(64);index:idx_chlat_req"`
	Group       string    `json:"group" gorm:"column:group;type:varchar(64);index:idx_chlat_gmc,priority:1"`
	Model       string    `json:"model" gorm:"type:varchar(128);index:idx_chlat_gmc,priority:2"`
	ChannelId   int       `json:"channel_id" gorm:"index:idx_chlat_gmc,priority:3"`
	ChannelType int       `json:"channel_type"`
	RetryIndex  int       `json:"retry_index"`
	Success     bool      `json:"success"`
	StatusCode  int       `json:"status_code"`
	ErrorStage  string    `json:"error_stage" gorm:"type:varchar(32)"`
	ReqBytes    int64     `json:"req_bytes"`
	RespBytes   int64     `json:"resp_bytes"`
	TSendMs     int64     `json:"t_send_ms"`
	TWaitMs     int64     `json:"t_wait_ms"`
	TRecvMs     int64     `json:"t_recv_ms"`
	TTotalMs    int64     `json:"t_total_ms"`
}

func (ChannelLatencySample) TableName() string {
	return "channel_latency_samples"
}

// BatchInsertChannelLatencySamples inserts detail rows; failures are logged and ignored.
func BatchInsertChannelLatencySamples(rows []ChannelLatencySample) {
	if len(rows) == 0 || LOG_DB == nil {
		return
	}
	if err := LOG_DB.CreateInBatches(rows, 100).Error; err != nil {
		common.SysError("channel latency DB insert failed: " + err.Error())
	}
}

// DeleteChannelLatencySamplesBefore removes old detail rows (retention).
func DeleteChannelLatencySamplesBefore(before time.Time, limit int) (int64, error) {
	if LOG_DB == nil {
		return 0, nil
	}
	if limit <= 0 {
		limit = 5000
	}
	res := LOG_DB.Where("created_at < ?", before).Limit(limit).Delete(&ChannelLatencySample{})
	return res.RowsAffected, res.Error
}

// QueryChannelLatencySamples returns recent detail rows for admin observation.
func QueryChannelLatencySamples(group, modelName string, channelID, limit int) ([]ChannelLatencySample, error) {
	if LOG_DB == nil {
		return nil, nil
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	q := LOG_DB.Model(&ChannelLatencySample{}).Order("id desc").Limit(limit)
	if group != "" {
		q = q.Where(commonGroupCol+" = ?", group)
	}
	if modelName != "" {
		q = q.Where("model = ?", modelName)
	}
	if channelID > 0 {
		q = q.Where("channel_id = ?", channelID)
	}
	var rows []ChannelLatencySample
	err := q.Find(&rows).Error
	return rows, err
}
