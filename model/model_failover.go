package model

import (
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

const MaxModelFailoverRetries = 10

type ModelFailoverConfig struct {
	Enabled    bool
	MaxRetries int
}

var modelFailoverCache struct {
	sync.RWMutex
	models []Model
	loaded time.Time
}

func ValidateModelFailover(m *Model) error {
	if m.FailoverMaxRetries < 0 || m.FailoverMaxRetries > MaxModelFailoverRetries || (m.FailoverEnabled && m.FailoverMaxRetries == 0) {
		return errors.New("故障切换次数必须为 1 到 10（关闭时可为 0）")
	}
	return nil
}

func InvalidateModelFailoverCache() {
	modelFailoverCache.Lock()
	defer modelFailoverCache.Unlock()
	modelFailoverCache.loaded = time.Time{}
}

// Cache metadata, not per-request model names, to keep the cache bounded. Other
// nodes pick up metadata changes within one minute, like the pricing cache.
func GetModelFailoverConfig(modelName string) ModelFailoverConfig {
	modelFailoverCache.RLock()
	if time.Since(modelFailoverCache.loaded) < time.Minute {
		defer modelFailoverCache.RUnlock()
		return matchModelFailoverConfig(modelFailoverCache.models, modelName)
	}
	modelFailoverCache.RUnlock()
	modelFailoverCache.Lock()
	defer modelFailoverCache.Unlock()
	if time.Since(modelFailoverCache.loaded) >= time.Minute {
		var models []Model
		err := DB.Select("id", "model_name", "name_rule", "status", "failover_enabled", "failover_max_retries").Order("id ASC").Find(&models).Error
		if err != nil {
			common.SysError("load model failover settings: " + err.Error())
		} else {
			modelFailoverCache.models = models
		}
		modelFailoverCache.loaded = time.Now()
	}
	return matchModelFailoverConfig(modelFailoverCache.models, modelName)
}

func matchModelFailoverConfig(models []Model, modelName string) ModelFailoverConfig {
	// Match metadata rules in the same order as pricing: exact, prefix, suffix,
	// contains. An explicit disabled exact rule must override broader rules.
	for _, name := range []string{modelName, ratio_setting.FormatMatchingModelName(modelName)} {
		for _, rule := range []int{NameRuleExact, NameRulePrefix, NameRuleSuffix, NameRuleContains} {
			for _, m := range models {
				if m.NameRule != rule {
					continue
				}
				matched := false
				switch rule {
				case NameRuleExact:
					matched = name == m.ModelName
				case NameRulePrefix:
					matched = strings.HasPrefix(name, m.ModelName)
				case NameRuleSuffix:
					matched = strings.HasSuffix(name, m.ModelName)
				case NameRuleContains:
					matched = strings.Contains(name, m.ModelName)
				}
				if matched {
					if m.Status != 1 || !m.FailoverEnabled || ValidateModelFailover(&m) != nil {
						return ModelFailoverConfig{}
					}
					return ModelFailoverConfig{Enabled: true, MaxRetries: m.FailoverMaxRetries}
				}
			}
		}
	}
	return ModelFailoverConfig{}
}
