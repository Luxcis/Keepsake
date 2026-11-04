// Package cachex 对外提供 Redis 探活语义。
// 计划 §3：Redis 用于会话、短期缓存和限流；不保存唯一业务事实，缓存丢失后可从 SQLite 重建。
// 连接供 auth 包共享（会话/限流），本包只负责健康状态展示。
package cachex

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	StatusOK          = "ok"
	StatusDisabled    = "disabled"
	StatusUnreachable = "unreachable"
)

// ErrDisabled 表示未配置 Redis（REDIS_ADDR 为空），调用方据此走降级路径。
var ErrDisabled = errors.New("redis disabled: REDIS_ADDR not configured")

type Cache struct {
	client *redis.Client // nil = 未配置
}

// New client 为 nil 表示未配置 Redis（开发态允许）。
func New(client *redis.Client) *Cache { return &Cache{client: client} }

// Ping 返回 nil（可用）/ ErrDisabled（未配置）/ 连接错误。
func (c *Cache) Ping(ctx context.Context) error {
	if c.client == nil {
		return ErrDisabled
	}
	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return c.client.Ping(pingCtx).Err()
}

// Status 返回探活状态字符串，用于 /healthz 展示。
func (c *Cache) Status(ctx context.Context) string {
	switch err := c.Ping(ctx); {
	case err == nil:
		return StatusOK
	case errors.Is(err, ErrDisabled):
		return StatusDisabled
	default:
		return StatusUnreachable
	}
}

// Close 关闭底层连接（如有）。
func (c *Cache) Close() error {
	if c.client != nil {
		return c.client.Close()
	}
	return nil
}
