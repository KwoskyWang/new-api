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
