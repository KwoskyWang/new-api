package controller

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/perf_metrics_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// Exercise real HTTP requests through token authentication, channel distribution,
// the relay controller, and local OpenAI-compatible upstream servers. No real
// provider credentials, external services, or persistent databases are used.
func TestRelayFailoverRecoveryHTTP(t *testing.T) {
	originalCache, originalRetries := common.MemoryCacheEnabled, common.RetryTimes
	originalCountToken, originalErrorLog := constant.CountToken, constant.ErrorLogEnabled
	originalStreamingTimeout := constant.StreamingTimeout
	originalLogConsume, originalBatch := common.LogConsumeEnabled, common.BatchUpdateEnabled
	originalPrices := ratio_setting.ModelPrice2JSONString()
	originalFreePreConsume := operation_setting.GetQuotaSetting().EnableFreeModelPreConsume
	originalPerfMetricsEnabled := perf_metrics_setting.GetSetting().Enabled
	common.RetryTimes = 0
	constant.CountToken, constant.ErrorLogEnabled = false, false
	constant.StreamingTimeout = 5
	common.LogConsumeEnabled, common.BatchUpdateEnabled = false, false
	operation_setting.GetQuotaSetting().EnableFreeModelPreConsume = false
	// Disable async performance-metrics sampling. Otherwise the fire-and-forget
	// sampling goroutine spawned after a successful relay can outlive this test and
	// read global config (e.g. common.RedisEnabled) concurrently with the next
	// test's setup that writes it, producing a data race.
	// 禁用异步性能指标采样。否则 relay 成功后派生的 fire-and-forget 采样 goroutine
	// 可能超出本测试生命周期，在下一个测试写入全局配置（如 common.RedisEnabled）时
	// 并发读取它，产生数据竞争。
	perf_metrics_setting.SetEnabled(false)
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"failover-http":0}`))
	service.InitHttpClient()
	t.Cleanup(func() {
		common.MemoryCacheEnabled, common.RetryTimes = originalCache, originalRetries
		constant.CountToken, constant.ErrorLogEnabled = originalCountToken, originalErrorLog
		constant.StreamingTimeout = originalStreamingTimeout
		common.LogConsumeEnabled, common.BatchUpdateEnabled = originalLogConsume, originalBatch
		operation_setting.GetQuotaSetting().EnableFreeModelPreConsume = originalFreePreConsume
		perf_metrics_setting.SetEnabled(originalPerfMetricsEnabled)
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(originalPrices))
		model.InvalidateModelFailoverCache()
	})

	for _, tc := range []struct {
		name           string
		cached         bool
		disabled       bool
		connectionDown bool
		stream         bool
		firstStatus    int
		channels       int
		successID      int
		retries        int
		wantStatus     int
		wantAttempts   []string
		wantCalls      []int
	}{
		{name: "503 switches to healthy backup", firstStatus: 503, channels: 2, successID: 2, retries: 2, wantStatus: 200, wantAttempts: []string{"1", "2"}, wantCalls: []int{1, 2}},
		{name: "connection refused switches to healthy backup", connectionDown: true, firstStatus: 503, channels: 2, successID: 2, retries: 2, wantStatus: 200, wantAttempts: []string{"1", "2"}, wantCalls: []int{2}},
		{name: "memory cache switches to healthy backup", cached: true, firstStatus: 503, channels: 2, successID: 2, retries: 2, wantStatus: 200, wantAttempts: []string{"1", "2"}, wantCalls: []int{1, 2}},
		{name: "disabled does not switch with zero global retries", disabled: true, firstStatus: 503, channels: 2, successID: 2, retries: 2, wantStatus: 503, wantAttempts: []string{"1"}, wantCalls: []int{1}},
		{name: "one switch limit does not reach third channel", firstStatus: 503, channels: 3, successID: 3, retries: 1, wantStatus: 503, wantAttempts: []string{"1", "2"}, wantCalls: []int{1, 2}},
		{name: "bad request does not switch", firstStatus: 400, channels: 2, successID: 2, retries: 2, wantStatus: 400, wantAttempts: []string{"1"}, wantCalls: []int{1}},
		{name: "streaming request switches before response starts", stream: true, firstStatus: 503, channels: 2, successID: 2, retries: 2, wantStatus: 200, wantAttempts: []string{"1", "2"}, wantCalls: []int{1, 2}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupModelListControllerTestDB(t)
			require.NoError(t, db.AutoMigrate(&model.Token{}))
			common.MemoryCacheEnabled = tc.cached
			model.InvalidateModelFailoverCache()
			t.Cleanup(model.InvalidateModelFailoverCache)
			require.NoError(t, db.Create(&model.Model{ModelName: "failover-http", Status: 1, FailoverEnabled: !tc.disabled, FailoverMaxRetries: tc.retries}).Error)
			user := model.User{Username: "failover-test", Password: "test-only", Status: common.UserStatusEnabled, Group: "default", Quota: 100000}
			require.NoError(t, db.Create(&user).Error)
			token := model.Token{UserId: user.Id, Key: "failoverhttptoken", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true}
			require.NoError(t, db.Create(&token).Error)

			var mu sync.Mutex
			var calls []int
			var attempts []string
			bodies := make(map[int]string)
			upstreams := make([]*httptest.Server, 0, tc.channels)
			for id := 1; id <= tc.channels; id++ {
				id := id
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					body, _ := io.ReadAll(r.Body)
					mu.Lock()
					calls = append(calls, id)
					bodies[id] = string(body)
					mu.Unlock()
					if id == tc.successID {
						if tc.stream {
							w.Header().Set("Content-Type", "text/event-stream")
							_, _ = io.WriteString(w, "data: {\"id\":\"chatcmpl-test\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"failover-http\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"OK\"},\"finish_reason\":null}]}\n\n")
							_, _ = io.WriteString(w, "data: {\"id\":\"chatcmpl-test\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"failover-http\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":1,\"total_tokens\":2}}\n\ndata: [DONE]\n\n")
							return
						}
						w.Header().Set("Content-Type", "application/json")
						_, _ = io.WriteString(w, `{"id":"chatcmpl-test","object":"chat.completion","created":1,"model":"failover-http","choices":[{"index":0,"message":{"role":"assistant","content":"OK"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
						return
					}
					status := http.StatusServiceUnavailable
					if id == 1 {
						status = tc.firstStatus
					}
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(status)
					_, _ = fmt.Fprintf(w, `{"error":{"message":"upstream-%d-down","type":"server_error","code":"unavailable"}}`, id)
				}))
				upstreams = append(upstreams, upstream)
				t.Cleanup(upstream.Close)
				priority := int64(10 - id)
				ch := model.Channel{Id: id, Type: constant.ChannelTypeOpenAI, Key: "local-test-key", BaseURL: &upstream.URL,
					Status: common.ChannelStatusEnabled, Models: "failover-http", Group: "default", Priority: &priority, AutoBan: common.GetPointer(0)}
				require.NoError(t, db.Create(&ch).Error)
				require.NoError(t, ch.AddAbilities(nil))
			}
			model.InitChannelCache()
			router := gin.New()
			completed := make(chan struct{})
			router.Use(gin.Recovery(), func(c *gin.Context) {
				defer close(completed)
				c.Next()
				mu.Lock()
				attempts = append([]string(nil), c.GetStringSlice("use_channel")...)
				mu.Unlock()
			})
			router.POST("/v1/chat/completions", middleware.TokenAuth(), middleware.Distribute(), func(c *gin.Context) { Relay(c, types.RelayFormatOpenAI) })
			gateway := httptest.NewServer(router)
			t.Cleanup(gateway.Close)
			if tc.connectionDown {
				upstreams[0].Close()
			}
			body := fmt.Sprintf(`{"model":"failover-http","messages":[{"role":"user","content":"hello"}],"temperature":0,"stream":%t}`, tc.stream)
			req, err := http.NewRequest(http.MethodPost, gateway.URL+"/v1/chat/completions", strings.NewReader(body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer sk-"+token.Key)
			client := gateway.Client()
			client.Timeout = 10 * time.Second
			resp, err := client.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()
			responseBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			// The upstream Content-Length can let the client finish reading before
			// the relay finishes its bookkeeping and middleware unwinds.
			select {
			case <-completed:
			case <-time.After(10 * time.Second):
				t.Fatal("gateway handler did not finish")
			}
			require.Equal(t, tc.wantStatus, resp.StatusCode, string(responseBody))
			mu.Lock()
			defer mu.Unlock()
			require.Equal(t, tc.wantAttempts, attempts)
			require.Equal(t, tc.wantCalls, calls)
			for _, sentBody := range bodies {
				require.Contains(t, sentBody, `"temperature":0`)
			}
			if tc.wantStatus == http.StatusOK {
				require.Contains(t, string(responseBody), `"content":"OK"`)
				if tc.stream {
					require.Contains(t, resp.Header.Get("Content-Type"), "text/event-stream")
					require.Contains(t, string(responseBody), "data: [DONE]")
				}
				if !tc.connectionDown {
					require.JSONEq(t, bodies[1], bodies[2])
				}
			}
			t.Logf("HTTP %d, channel attempts %v, upstream calls %v", resp.StatusCode, attempts, calls)
		})
	}
}
