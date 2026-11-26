package channel_latency

import (
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

var (
	dbQueue chan Sample
	dbOnce  sync.Once
)

func enqueueDetail(sample Sample) {
	if !operation_setting.IsChannelLatencyDetailDBEnabled() {
		return
	}
	dbOnce.Do(startDBWorker)
	select {
	case dbQueue <- sample:
	default:
		// drop on overload; never block relay / stream path
	}
}

func startDBWorker() {
	dbQueue = make(chan Sample, 4096)
	go func() {
		buf := make([]Sample, 0, 64)
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case s := <-dbQueue:
				buf = append(buf, s)
				if len(buf) >= 64 {
					flushDB(buf)
					buf = buf[:0]
				}
			case <-ticker.C:
				if len(buf) > 0 {
					flushDB(buf)
					buf = buf[:0]
				}
			}
		}
	}()
}

func flushDB(samples []Sample) {
	if len(samples) == 0 {
		return
	}
	rows := make([]model.ChannelLatencySample, 0, len(samples))
	for _, s := range samples {
		ts := s.Timestamp
		if ts.IsZero() {
			ts = time.Now()
		}
		rows = append(rows, model.ChannelLatencySample{
			CreatedAt:   ts,
			RequestId:   s.RequestID,
			Group:       s.Group,
			Model:       s.Model,
			ChannelId:   s.ChannelID,
			ChannelType: s.ChannelType,
			RetryIndex:  s.RetryIndex,
			Success:     s.Success,
			StatusCode:  s.StatusCode,
			ErrorStage:  s.ErrorStage,
			ReqBytes:    s.ReqBytes,
			RespBytes:   s.RespBytes,
			TSendMs:     s.TSendMs,
			TWaitMs:     s.TWaitMs,
			TRecvMs:     s.TRecvMs,
			TTotalMs:    s.TTotalMs,
		})
	}
	model.BatchInsertChannelLatencySamples(rows)
}

// startRetentionLoop periodically deletes old detail rows from LOG_DB.
func startRetentionLoop() {
	go func() {
		for {
			time.Sleep(30 * time.Minute)
			if !operation_setting.IsChannelLatencyDetailDBEnabled() {
				continue
			}
			days := operation_setting.GetChannelLatencySetting().DetailRetentionDays
			if days <= 0 {
				days = 7
			}
			before := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
			for {
				n, err := model.DeleteChannelLatencySamplesBefore(before, 5000)
				if err != nil {
					common.SysError("channel latency retention cleanup failed: " + err.Error())
					break
				}
				if n == 0 {
					break
				}
			}
		}
	}()
}
