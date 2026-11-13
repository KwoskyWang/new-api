package model

import (
	"sort"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func clearChannelAndAbilityTables(t *testing.T) {
	t.Helper()
	require.NoError(t, DB.Exec("DELETE FROM abilities").Error)
	require.NoError(t, DB.Exec("DELETE FROM channels").Error)
}

func createChannelWithModels(t *testing.T, id int, group, models string) {
	t.Helper()
	ch := &Channel{
		Id:     id,
		Type:   1,
		Key:    "k",
		Status: common.ChannelStatusEnabled,
		Name:   "ch",
		Group:  group,
		Models: models,
	}
	require.NoError(t, DB.Create(ch).Error)
	require.NoError(t, ch.AddAbilities(nil))
}

func channelModels(t *testing.T, id int) string {
	t.Helper()
	var ch Channel
	require.NoError(t, DB.First(&ch, id).Error)
	return ch.Models
}

func abilityModels(t *testing.T, channelID int) []string {
	t.Helper()
	var models []string
	require.NoError(t, DB.Model(&Ability{}).Where("channel_id = ?", channelID).Pluck("model", &models).Error)
	sort.Strings(models)
	return models
}

func TestRemoveModelFromCSV(t *testing.T) {
	for _, tc := range []struct {
		name    string
		input   string
		model   string
		want    string
		removed bool
	}{
		{"middle", "a,b,c", "b", "a,c", true},
		{"first", "a,b,c", "a", "b,c", true},
		{"last", "a,b,c", "c", "a,b", true},
		{"only", "a", "a", "", true},
		{"absent", "a,b", "x", "a,b", false},
		{"substring not removed", "gpt-4,gpt-4o", "gpt-4", "gpt-4o", true},
		{"substring target", "gpt-4,gpt-4o", "gpt-4o", "gpt-4", true},
		{"with spaces", "a, b ,c", "b", "a,c", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, removed := removeModelFromCSV(tc.input, tc.model)
			require.Equal(t, tc.want, got)
			require.Equal(t, tc.removed, removed)
		})
	}
}

// TestRemoveModelFromAllChannels verifies that deleting a model name removes it
// from every channel's models list and keeps the abilities table in sync, while
// never touching substring-similar model names.
// 验证删除某模型名会从所有渠道的 models 列表中移除并保持 abilities 同步，同时绝不
// 触碰子串相似的模型名。
func TestRemoveModelFromAllChannels(t *testing.T) {
	clearChannelAndAbilityTables(t)
	t.Cleanup(func() { clearChannelAndAbilityTables(t) })

	// #1: has the target model alongside a substring-similar one.
	// #2: target model in the middle.
	// #3: target model is the only model (will become empty).
	// #4: does not reference the target model at all.
	createChannelWithModels(t, 1, "default", "gpt-4,gpt-4o")
	createChannelWithModels(t, 2, "default", "a,gpt-4,b")
	createChannelWithModels(t, 3, "default", "gpt-4")
	createChannelWithModels(t, 4, "default", "x,y")

	require.NoError(t, RemoveModelFromAllChannels("gpt-4"))

	require.Equal(t, "gpt-4o", channelModels(t, 1))
	require.Equal(t, "a,b", channelModels(t, 2))
	require.Equal(t, "", channelModels(t, 3))
	require.Equal(t, "x,y", channelModels(t, 4))

	// Abilities must be in sync and must not contain the removed model or an empty
	// model entry.
	// abilities 必须同步，且不得包含被移除的模型或空模型条目。
	require.Equal(t, []string{"gpt-4o"}, abilityModels(t, 1))
	require.Equal(t, []string{"a", "b"}, abilityModels(t, 2))
	require.Empty(t, abilityModels(t, 3))
	require.Equal(t, []string{"x", "y"}, abilityModels(t, 4))
}

func TestRemoveModelFromAllChannelsNoop(t *testing.T) {
	clearChannelAndAbilityTables(t)
	t.Cleanup(func() { clearChannelAndAbilityTables(t) })

	createChannelWithModels(t, 1, "default", "a,b")

	// Empty model name and absent model name must both be no-ops.
	// 空模型名与不存在的模型名都应为无操作。
	require.NoError(t, RemoveModelFromAllChannels("  "))
	require.NoError(t, RemoveModelFromAllChannels("not-present"))

	require.Equal(t, "a,b", channelModels(t, 1))
	require.Equal(t, []string{"a", "b"}, abilityModels(t, 1))
}
