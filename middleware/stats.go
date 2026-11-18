package middleware

import (
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"

	"github.com/gin-gonic/gin"
)

// HTTPStats 存储HTTP统计信息
type HTTPStats struct {
	activeConnections int64
}

var globalStats = &HTTPStats{}

// StatsMiddleware 统计中间件
func StatsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 尽早记录请求进入应用的时间，作为"管理员专属转发链路分段耗时诊断"的
		// 本地处理计时基准。该时间仅用于诊断，不改变任何既有计时（StartTime 等）的语义。
		// 覆盖范围：从本中间件（relay 路由链上最早运行的中间件之一）开始，
		// 到真正发起第一次上游 HTTP 请求之前的进程内处理。
		common.SetContextKey(c, constant.ContextKeyAppEnterTime, time.Now())

		// 增加活跃连接数
		atomic.AddInt64(&globalStats.activeConnections, 1)

		// 确保在请求结束时减少连接数
		defer func() {
			atomic.AddInt64(&globalStats.activeConnections, -1)
		}()

		c.Next()
	}
}

// StatsInfo 统计信息结构
type StatsInfo struct {
	ActiveConnections int64 `json:"active_connections"`
}

// GetStats 获取统计信息
func GetStats() StatsInfo {
	return StatsInfo{
		ActiveConnections: atomic.LoadInt64(&globalStats.activeConnections),
	}
}
