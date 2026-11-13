package perf_metrics_setting

import "github.com/QuantumNous/new-api/setting/config"

type PerfMetricsSetting struct {
	Enabled       bool   `json:"enabled"`
	FlushInterval int    `json:"flush_interval"`
	BucketTime    string `json:"bucket_time"`
	RetentionDays int    `json:"retention_days"`
}

var perfMetricsSetting = PerfMetricsSetting{
	Enabled:       true,
	FlushInterval: 5,
	BucketTime:    "hour",
	RetentionDays: 0,
}

func init() {
	config.GlobalConfig.Register("perf_metrics_setting", &perfMetricsSetting)
}

func GetSetting() PerfMetricsSetting {
	return perfMetricsSetting
}

// SetEnabled toggles performance-metrics sampling at runtime. It is primarily
// used to disable asynchronous sampling in tests so that fire-and-forget sample
// goroutines do not outlive a test and race with global configuration, but it is
// also a valid runtime switch for operators.
// SetEnabled 在运行时开关性能指标采样。主要用于在测试中禁用异步采样，避免
// fire-and-forget 的采样 goroutine 超出测试生命周期、与全局配置产生竞争；对运维
// 而言它也是一个有效的运行时开关。
func SetEnabled(enabled bool) {
	perfMetricsSetting.Enabled = enabled
}

func GetBucketSeconds() int64 {
	switch perfMetricsSetting.BucketTime {
	case "minute":
		return 60
	case "5min":
		return 300
	case "hour":
		return 3600
	default:
		return 3600
	}
}

func GetFlushIntervalMinutes() int {
	if perfMetricsSetting.FlushInterval < 1 {
		return 1
	}
	return perfMetricsSetting.FlushInterval
}
