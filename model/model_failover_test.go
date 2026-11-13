package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type legacyFailoverModel struct {
	Id        int
	ModelName string
	Status    int
}

func (legacyFailoverModel) TableName() string { return "models" }

func TestModelFailoverMigrationPreservesExistingModels(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(&legacyFailoverModel{}))
	require.NoError(t, db.Create(&legacyFailoverModel{Id: 1, ModelName: "existing-model", Status: 1}).Error)
	require.False(t, db.Migrator().HasColumn(&Model{}, "failover_enabled"))
	require.NoError(t, db.AutoMigrate(&Model{}))
	require.True(t, db.Migrator().HasColumn(&Model{}, "failover_enabled"))
	require.True(t, db.Migrator().HasColumn(&Model{}, "failover_max_retries"))
	var existing Model
	require.NoError(t, db.First(&existing, 1).Error)
	require.Equal(t, "existing-model", existing.ModelName)
	require.Equal(t, 1, existing.Status)
	require.False(t, existing.FailoverEnabled)
	require.Zero(t, existing.FailoverMaxRetries)
}

func TestMatchModelFailoverConfig(t *testing.T) {
	models := []Model{
		{ModelName: "gpt-", NameRule: NameRulePrefix, Status: 1, FailoverEnabled: true, FailoverMaxRetries: 3},
		{ModelName: "gpt-exact", Status: 1, FailoverEnabled: true, FailoverMaxRetries: 2},
		{ModelName: "gpt-disabled", Status: 1},
		{ModelName: "gpt-hidden", Status: 0, FailoverEnabled: true, FailoverMaxRetries: 4},
		{ModelName: "-suffix", NameRule: NameRuleSuffix, Status: 1, FailoverEnabled: true, FailoverMaxRetries: 4},
		{ModelName: "contains", NameRule: NameRuleContains, Status: 1, FailoverEnabled: true, FailoverMaxRetries: 5},
		{ModelName: "gpt-invalid", Status: 1, FailoverEnabled: true, FailoverMaxRetries: 100},
	}
	for _, tc := range []struct {
		name string
		want int
	}{
		{"gpt-exact", 2}, {"gpt-other", 3}, {"gpt-disabled", 0},
		{"gpt-hidden", 0}, {"gpt-invalid", 0}, {"other-suffix", 4},
		{"x-contains-y", 5}, {"unknown", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := matchModelFailoverConfig(models, tc.name)
			require.Equal(t, tc.want > 0, got.Enabled)
			require.Equal(t, tc.want, got.MaxRetries)
		})
	}
	require.Equal(t, ModelFailoverConfig{Enabled: true, MaxRetries: 6}, matchModelFailoverConfig([]Model{
		{ModelName: "gpt-4-gizmo-*", Status: 1, FailoverEnabled: true, FailoverMaxRetries: 6},
	}, "gpt-4-gizmo-custom"))
}

func TestValidateModelFailover(t *testing.T) {
	for _, count := range []int{-1, 0, 11} {
		require.Error(t, ValidateModelFailover(&Model{FailoverEnabled: true, FailoverMaxRetries: count}))
	}
	for _, count := range []int{1, 2, 10} {
		require.NoError(t, ValidateModelFailover(&Model{FailoverEnabled: true, FailoverMaxRetries: count}))
	}
	require.NoError(t, ValidateModelFailover(&Model{}))
}

func TestModelFailoverPersistenceAndCache(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&Model{}))
	require.NoError(t, DB.Where("1 = 1").Unscoped().Delete(&Model{}).Error)
	InvalidateModelFailoverCache()
	t.Cleanup(func() {
		DB.Where("1 = 1").Unscoped().Delete(&Model{})
		InvalidateModelFailoverCache()
	})
	m := Model{ModelName: "failover-test", Status: 1, FailoverEnabled: true, FailoverMaxRetries: 2}
	require.NoError(t, m.Insert())
	require.Equal(t, ModelFailoverConfig{Enabled: true, MaxRetries: 2}, GetModelFailoverConfig(m.ModelName))
	m.FailoverEnabled = false
	m.FailoverMaxRetries = 0
	require.NoError(t, m.Update())
	InvalidateModelFailoverCache()
	require.Equal(t, ModelFailoverConfig{}, GetModelFailoverConfig(m.ModelName))
	var saved Model
	require.NoError(t, DB.First(&saved, m.Id).Error)
	require.False(t, saved.FailoverEnabled)
	require.Zero(t, saved.FailoverMaxRetries)
	m.FailoverEnabled = true
	m.FailoverMaxRetries = 3
	require.NoError(t, m.Update())
	InvalidateModelFailoverCache()
	require.Equal(t, 3, GetModelFailoverConfig(m.ModelName).MaxRetries)
	require.NoError(t, m.Delete())
	InvalidateModelFailoverCache()
	require.Equal(t, ModelFailoverConfig{}, GetModelFailoverConfig(m.ModelName))
}

func TestGetModelFailoverChannel(t *testing.T) {
	originalCacheEnabled := common.MemoryCacheEnabled
	t.Cleanup(func() { common.MemoryCacheEnabled = originalCacheEnabled })
	for _, cached := range []bool{false, true} {
		name := "database"
		if cached {
			name = "memory"
		}
		t.Run(name, func(t *testing.T) {
			clearPreferredOwnerTables(t)
			t.Cleanup(func() { clearPreferredOwnerTables(t) })
			common.MemoryCacheEnabled = cached
			// Same-priority alternatives must be tried before lower priorities.
			for _, tc := range []struct {
				id           int
				group, model string
				priority     int64
				status       int
			}{
				{1, "default", "test-model", 10, common.ChannelStatusEnabled},
				{2, "default", "test-model", 10, common.ChannelStatusEnabled},
				{3, "default", "test-model", 5, common.ChannelStatusEnabled},
				{4, "vip", "test-model", 100, common.ChannelStatusEnabled},
				{5, "default", "other-model", 100, common.ChannelStatusEnabled},
				{6, "default", "test-model", 100, common.ChannelStatusManuallyDisabled},
			} {
				insertPreferredOwnerCandidate(t, tc.id, tc.model, tc.group, 1, tc.priority, 0, tc.status, tc.status == common.ChannelStatusEnabled)
				require.NoError(t, DB.Model(&Channel{}).Where("id = ?", tc.id).Updates(map[string]interface{}{
					"group": tc.group, "models": tc.model, "priority": tc.priority,
				}).Error)
			}
			InitChannelCache()
			ch, err := GetModelFailoverChannel("default", "test-model", []int{1})
			require.NoError(t, err)
			require.Equal(t, 2, ch.Id)
			ch, err = GetModelFailoverChannel("default", "test-model", []int{1, 2})
			require.NoError(t, err)
			require.Equal(t, 3, ch.Id)
			ch, err = GetModelFailoverChannel("default", "test-model", []int{1, 2, 3})
			require.Nil(t, ch)
			require.ErrorIs(t, err, ErrNoAvailableFailoverChannel)
			_, err = GetModelFailoverChannel("unknown", "test-model", nil)
			require.ErrorIs(t, err, ErrNoAvailableFailoverChannel)
			// Only one enabled channel: never retry the failed channel itself.
			_, err = GetModelFailoverChannel("vip", "test-model", []int{4})
			require.ErrorIs(t, err, ErrNoAvailableFailoverChannel)
		})
	}
}

// TestGetModelFailoverChannelZeroWeightExcluded verifies that failover shares the
// same weight-0 semantics as the primary selection path: a weight-0 channel is
// excluded from selection as long as a positive-weight channel exists in the
// same priority layer, and is only used when it is the sole remaining candidate.
// 验证 failover 与主选择路径的权重 0 语义一致：只要同优先级层存在权重 > 0 的渠道，
// 权重 0 的渠道会被排除；仅当它是唯一剩余候选时才会被使用。
func TestGetModelFailoverChannelZeroWeightExcluded(t *testing.T) {
	originalCacheEnabled := common.MemoryCacheEnabled
	t.Cleanup(func() { common.MemoryCacheEnabled = originalCacheEnabled })
	for _, cached := range []bool{false, true} {
		name := "database"
		if cached {
			name = "memory"
		}
		t.Run(name, func(t *testing.T) {
			clearPreferredOwnerTables(t)
			t.Cleanup(func() { clearPreferredOwnerTables(t) })
			common.MemoryCacheEnabled = cached

			// Three same-priority channels: #1 weight 10, #2 weight 0, #3 weight 20.
			// 三个同优先级渠道：#1 权重 10，#2 权重 0，#3 权重 20。
			for _, tc := range []struct {
				id     int
				weight uint
			}{{1, 10}, {2, 0}, {3, 20}} {
				insertPreferredOwnerCandidate(t, tc.id, "zw-model", "default", 1, 10, tc.weight, common.ChannelStatusEnabled, true)
				// Keep Channel.weight in sync (cached path reads channel weight).
				// 同步 Channel.weight（缓存路径读取渠道权重）。
				require.NoError(t, DB.Model(&Channel{}).Where("id = ?", tc.id).Updates(map[string]interface{}{
					"group": "default", "models": "zw-model", "priority": 10, "weight": tc.weight,
				}).Error)
			}
			InitChannelCache()

			// With #1 and #3 positive-weight present, #2 (weight 0) must never be
			// chosen across many selections.
			// 当 #1、#3 为正权重时，#2（权重 0）在多次选择中绝不应被选中。
			for i := 0; i < 50; i++ {
				ch, err := GetModelFailoverChannel("default", "zw-model", nil)
				require.NoError(t, err)
				require.NotEqual(t, 2, ch.Id, "zero-weight channel must not be selected")
			}

			// Exclude #1 and #3; only the weight-0 channel #2 remains, so it must be
			// selected (sole candidate fallback).
			// 排除 #1 和 #3；只剩权重 0 的 #2，此时它应被选中（唯一候选兜底）。
			ch, err := GetModelFailoverChannel("default", "zw-model", []int{1, 3})
			require.NoError(t, err)
			require.Equal(t, 2, ch.Id)
		})
	}
}
