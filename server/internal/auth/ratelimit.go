package auth

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// 固定窗口限速器（计划 §5.1：登录限速）。
// 语义（计划 §3.2）：
//   - Redis 已配置且可用 → 计数存 Redis（多副本亦生效）；
//   - 未配置 Redis → 内存计数（仅开发场景）；
//   - 已配置但访问失败 → 返回 err，调用方按“失败关闭”拒绝（确定性 503）。
type Limiter struct {
	client *redis.Client // 为空表示未配置 Redis
	memory *memoryLimiter
}

func NewLimiter(client *redis.Client) *Limiter {
	l := &Limiter{client: client}
	if client == nil {
		l.memory = newMemoryLimiter()
	}
	return l
}

// HasBackend reports whether Redis was configured（用于调用方选择失败策略的说明）。
func (l *Limiter) HasBackend() bool { return l.client != nil }

// Allow 记一次请求并返回是否放行；limit 次内放行，超出拒绝，窗口到期清零。
// backErr 仅在“配置了 Redis 但不可用”时非空，此时结果一律 false。
func (l *Limiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (allowed bool, backErr error) {
	if l.client == nil {
		return l.memory.allow(key, limit, window), nil
	}
	pipe := l.client.TxPipeline()
	incr := pipe.Incr(ctx, "rl:"+key)
	pipe.Expire(ctx, "rl:"+key, window)
	if _, err := pipe.Exec(ctx); err != nil {
		return false, fmt.Errorf("rate limit backend: %w", err)
	}
	return incr.Val() <= int64(limit), nil
}

// Reset 在成功登录等场景清空失败计数。
func (l *Limiter) Reset(ctx context.Context, key string) {
	if l.client == nil {
		l.memory.reset(key)
		return
	}
	_ = l.client.Del(ctx, "rl:"+key).Err()
}

// ---------- 内存版（开发退路） ----------

type memCounter struct{ start, hits int64 }

type memoryLimiter struct {
	mu sync.Mutex
	m  map[string]*memCounter
}

func newMemoryLimiter() *memoryLimiter { return &memoryLimiter{m: map[string]*memCounter{}} }

func (l *memoryLimiter) allow(key string, limit int, window time.Duration) bool {
	now := time.Now().UnixMilli()
	l.mu.Lock()
	defer l.mu.Unlock()
	c, ok := l.m[key]
	if !ok || now-c.start >= window.Milliseconds() {
		l.m[key] = &memCounter{start: now, hits: 1}
		return limit >= 1
	}
	c.hits++
	return c.hits <= int64(limit)
}

func (l *memoryLimiter) reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.m, key)
}
