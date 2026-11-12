package controller

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestModelMetaFailoverConfiguration(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	model.InvalidateModelFailoverCache()
	t.Cleanup(model.InvalidateModelFailoverCache)
	router := gin.New()
	router.POST("/models", CreateModelMeta)
	router.PUT("/models", UpdateModelMeta)
	router.DELETE("/models/:id", DeleteModelMeta)
	call := func(method, path, body string) bool {
		t.Helper()
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(recorder, req)
		var response struct {
			Success bool `json:"success"`
		}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
		return response.Success
	}
	require.True(t, call(http.MethodPost, "/models", `{"model_name":"failover-test","status":1,"failover_enabled":true,"failover_max_retries":2}`))
	var m model.Model
	require.NoError(t, db.Where("model_name = ?", "failover-test").First(&m).Error)
	require.Equal(t, model.ModelFailoverConfig{Enabled: true, MaxRetries: 2}, model.GetModelFailoverConfig(m.ModelName))
	// Old clients omit these fields; existing settings must survive.
	require.True(t, call(http.MethodPut, "/models", fmt.Sprintf(`{"id":%d,"model_name":"failover-test","status":1}`, m.Id)))
	require.Equal(t, 2, model.GetModelFailoverConfig(m.ModelName).MaxRetries)
	require.True(t, call(http.MethodPut, "/models?status_only=true", fmt.Sprintf(`{"id":%d,"status":0}`, m.Id)))
	require.Equal(t, model.ModelFailoverConfig{}, model.GetModelFailoverConfig(m.ModelName))
	require.True(t, call(http.MethodPut, "/models?status_only=true", fmt.Sprintf(`{"id":%d,"status":1}`, m.Id)))
	require.Equal(t, 2, model.GetModelFailoverConfig(m.ModelName).MaxRetries)
	// Explicit false and zero must persist, not be dropped as empty values.
	require.True(t, call(http.MethodPut, "/models", fmt.Sprintf(`{"id":%d,"model_name":"failover-test","status":1,"failover_enabled":false,"failover_max_retries":0}`, m.Id)))
	require.Equal(t, model.ModelFailoverConfig{}, model.GetModelFailoverConfig(m.ModelName))
	require.NoError(t, db.First(&m, m.Id).Error)
	require.False(t, m.FailoverEnabled)
	require.Zero(t, m.FailoverMaxRetries)
	for _, count := range []int{-1, 0, 11} {
		require.False(t, call(http.MethodPost, "/models", fmt.Sprintf(`{"model_name":"invalid","failover_enabled":true,"failover_max_retries":%d}`, count)))
	}
	require.False(t, call(http.MethodPut, "/models", fmt.Sprintf(`{"id":%d,"model_name":"failover-test","failover_enabled":true,"failover_max_retries":11}`, m.Id)))
	require.True(t, call(http.MethodDelete, fmt.Sprintf("/models/%d", m.Id), ""))
	require.Equal(t, model.ModelFailoverConfig{}, model.GetModelFailoverConfig(m.ModelName))
}

func TestRelayFailoverReplaysRequestAcrossUpstreams(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	originalCache, originalRetries := common.MemoryCacheEnabled, common.RetryTimes
	originalCountToken, originalErrorLog := constant.CountToken, constant.ErrorLogEnabled
	originalPrices := ratio_setting.ModelPrice2JSONString()
	originalFreePreConsume := operation_setting.GetQuotaSetting().EnableFreeModelPreConsume
	operation_setting.GetQuotaSetting().EnableFreeModelPreConsume = false
	common.MemoryCacheEnabled, common.RetryTimes = false, 0
	constant.CountToken, constant.ErrorLogEnabled = false, false
	model.InvalidateModelFailoverCache()
	t.Cleanup(func() {
		common.MemoryCacheEnabled, common.RetryTimes = originalCache, originalRetries
		constant.CountToken, constant.ErrorLogEnabled = originalCountToken, originalErrorLog
		operation_setting.GetQuotaSetting().EnableFreeModelPreConsume = originalFreePreConsume
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(originalPrices))
		model.InvalidateModelFailoverCache()
	})
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"failover-e2e":0}`))
	require.NoError(t, db.Create(&model.Model{ModelName: "failover-e2e", Status: 1, FailoverEnabled: true, FailoverMaxRetries: 2}).Error)
	service.InitHttpClient()

	var calls []int
	var bodies []string
	var callsLock sync.Mutex
	channels := make([]*model.Channel, 0, 2)
	for i := 1; i <= 2; i++ {
		id := i
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			callsLock.Lock()
			calls = append(calls, id)
			bodies = append(bodies, string(body))
			callsLock.Unlock()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = fmt.Fprintf(w, `{"error":{"message":"upstream-%d-down","type":"server_error","code":"unavailable"}}`, id)
		}))
		t.Cleanup(upstream.Close)
		priority := int64(10 - i)
		ch := &model.Channel{Id: id, Type: constant.ChannelTypeOpenAI, Key: "test-key", BaseURL: &upstream.URL,
			Status: common.ChannelStatusEnabled, Models: "failover-e2e", Group: "default", Priority: &priority,
			AutoBan: common.GetPointer(0)}
		require.NoError(t, db.Create(ch).Error)
		require.NoError(t, ch.AddAbilities(nil))
		channels = append(channels, ch)
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"failover-e2e","messages":[{"role":"user","content":"hello"}],"temperature":0}`))
	c.Request.Header.Set("Content-Type", "application/json")
	common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
	common.SetContextKey(c, constant.ContextKeyTokenGroup, "default")
	require.Nil(t, middleware.SetupContextForSelectedChannel(c, channels[0], "failover-e2e"))
	Relay(c, types.RelayFormatOpenAI)
	callsLock.Lock()
	defer callsLock.Unlock()
	require.Equal(t, []int{1, 2}, calls)
	require.Len(t, bodies, 2)
	require.JSONEq(t, bodies[0], bodies[1])
	require.Contains(t, bodies[1], `"temperature":0`)
	require.Equal(t, http.StatusServiceUnavailable, c.Writer.Status())
	require.Contains(t, recorder.Body.String(), "upstream-2-down")
	require.NotContains(t, recorder.Body.String(), "no untried channel")
	require.Equal(t, []string{"1", "2"}, c.GetStringSlice("use_channel"))
}

func TestRelayFailoverRetryGuards(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name      string
		code      int
		errorCode types.ErrorCode
		remaining int
		setup     func(*gin.Context)
		want      bool
	}{
		{name: "connection failure", code: 500, errorCode: types.ErrorCodeDoRequestFailed, remaining: 2, want: true},
		{name: "unavailable", code: 503, remaining: 2, want: true},
		{name: "bad client input", code: 400, remaining: 2},
		{name: "timeout safety", code: 504, remaining: 2},
		{name: "exhausted", code: 503},
		{name: "exhausted channel error", code: 500, errorCode: types.ErrorCodeChannelNoAvailableKey},
		{name: "pinned channel", code: 503, remaining: 2, setup: func(c *gin.Context) { c.Set("specific_channel_id", 1) }},
		{name: "pinned channel error", code: 500, errorCode: types.ErrorCodeChannelNoAvailableKey, remaining: 2, setup: func(c *gin.Context) { c.Set("specific_channel_id", 1) }},
		{name: "stream started", code: 503, remaining: 2, setup: func(c *gin.Context) { _, _ = c.Writer.Write([]byte("data: partial\n\n")) }},
		{name: "cancelled", code: 503, remaining: 2, setup: func(c *gin.Context) {
			ctx, cancel := context.WithCancel(c.Request.Context())
			cancel()
			c.Request = c.Request.WithContext(ctx)
		}},
		{name: "affinity forbids retry", code: 503, remaining: 2, setup: func(c *gin.Context) { c.Set("channel_affinity_skip_retry_on_failure", true) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			if tc.setup != nil {
				tc.setup(c)
			}
			err := types.NewErrorWithStatusCode(errors.New("upstream failed"), tc.errorCode, tc.code)
			require.Equal(t, tc.want, shouldRetry(c, err, tc.remaining))
		})
	}
}

func TestRelayFailoverSelectsDistinctChannelsInOriginalGroup(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	originalCache, originalRetries := common.MemoryCacheEnabled, common.RetryTimes
	common.MemoryCacheEnabled, common.RetryTimes = false, 0
	model.InvalidateModelFailoverCache()
	t.Cleanup(func() {
		common.MemoryCacheEnabled, common.RetryTimes = originalCache, originalRetries
		model.InvalidateModelFailoverCache()
	})
	require.NoError(t, db.Create(&model.Model{ModelName: "test-model", Status: 1, FailoverEnabled: true, FailoverMaxRetries: 2}).Error)
	for _, group := range []string{"default", "vip"} {
		for i := 0; i < 3; i++ {
			ch := model.Channel{Key: "test-key", Type: 1, Status: common.ChannelStatusEnabled, Models: "test-model", Group: group}
			require.NoError(t, db.Create(&ch).Error)
			require.NoError(t, ch.AddAbilities(nil))
		}
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	common.SetContextKey(c, constant.ContextKeyAutoGroup, "default")
	p := service.NewRelayRetryParam(c, "auto", "test-model")
	require.Equal(t, 2, p.GetRetryLimit()) // Independent of a zero global limit.
	// The retry policy snapshots the original actual group.
	common.SetContextKey(c, constant.ContextKeyAutoGroup, "vip")
	ids := make(map[int]bool)
	for i := 0; i <= p.GetRetryLimit(); i++ {
		ch, group, err := service.CacheGetRandomSatisfiedChannel(p)
		require.NoError(t, err)
		require.Equal(t, "default", group)
		require.Equal(t, "default", ch.Group)
		require.False(t, ids[ch.Id])
		ids[ch.Id] = true
		addUsedChannel(c, ch.Id)
		p.IncreaseRetry()
	}
	_, _, err := service.CacheGetRandomSatisfiedChannel(p)
	require.ErrorIs(t, err, model.ErrNoAvailableFailoverChannel)
	// Playground requests can select an authorized group other than TokenGroup.
	common.SetContextKey(c, constant.ContextKeyAutoGroup, "")
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "vip")
	playground := service.NewRelayRetryParam(c, "default", "test-model")
	ch, group, err := service.CacheGetRandomSatisfiedChannel(playground)
	require.NoError(t, err)
	require.Equal(t, "vip", group)
	require.Equal(t, "vip", ch.Group)
	require.False(t, shouldRetryTaskRelay(c, 1, &dto.TaskError{StatusCode: 503}, 0))
	// Disabling the feature restores the global retry limit.
	require.NoError(t, db.Model(&model.Model{}).Where("model_name = ?", "test-model").Update("failover_enabled", false).Error)
	model.InvalidateModelFailoverCache()
	require.Zero(t, service.NewRelayRetryParam(c, "default", "test-model").GetRetryLimit())
}
