package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// 会话有两类（计划 §5.1：站点访问口令 + 管理员登录，均为服务端会话）：
//
//	KindGate  站点访问门禁（访客），有效期长、滑动续期；
//	KindAdmin 管理员后台会话，默认 24h。
type SessionKind string

const (
	KindGate  SessionKind = "gate"
	KindAdmin SessionKind = "admin"
)

// Cookie 名：前端无需读取内容，HttpOnly。
const (
	GateCookieName  = "olj_gate"
	AdminCookieName = "olj_admin"
)

var ErrNoSession = errors.New("no session")

type SessionData struct {
	Username string `json:"username,omitempty"`
	Role     string `json:"role,omitempty"`
}

type SessionStore interface {
	Create(ctx context.Context, kind SessionKind, data SessionData, ttl time.Duration) (string, error)
	Get(ctx context.Context, kind SessionKind, token string) (SessionData, error)
	Delete(ctx context.Context, kind SessionKind, token string) error
	Close() error
}

func newToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// ---------- Redis 实现（生产） ----------

type RedisSessionStore struct {
	client *redis.Client
}

func NewRedisSessionStore(client *redis.Client) *RedisSessionStore {
	return &RedisSessionStore{client: client}
}

func keyOf(kind SessionKind, token string) string {
	return fmt.Sprintf("sess:%s:%s", kind, token)
}

func (s *RedisSessionStore) Create(ctx context.Context, kind SessionKind, data SessionData, ttl time.Duration) (string, error) {
	token, err := newToken()
	if err != nil {
		return "", err
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	if err := s.client.Set(ctx, keyOf(kind, token), raw, ttl).Err(); err != nil {
		return "", err
	}
	return token, nil
}

func (s *RedisSessionStore) Get(ctx context.Context, kind SessionKind, token string) (SessionData, error) {
	if token == "" {
		return SessionData{}, ErrNoSession
	}
	raw, err := s.client.Get(ctx, keyOf(kind, token)).Bytes()
	if err == redis.Nil {
		return SessionData{}, ErrNoSession
	}
	if err != nil {
		return SessionData{}, err
	}
	var data SessionData
	if err := json.Unmarshal(raw, &data); err != nil {
		return SessionData{}, fmt.Errorf("corrupt session: %w", err)
	}
	// 滑动续期：失败仅表示无法续期，不影响本请求。
	_ = s.client.Expire(ctx, keyOf(kind, token), 7*24*time.Hour).Err()
	return data, nil
}

func (s *RedisSessionStore) Delete(ctx context.Context, kind SessionKind, token string) error {
	if token == "" {
		return nil
	}
	return s.client.Del(ctx, keyOf(kind, token)).Err()
}

func (s *RedisSessionStore) Close() error { return s.client.Close() }

// ---------- 内存实现（仅开发：REDIS_ADDR 未配置时） ----------

type memoryEntry struct {
	data    SessionData
	expires time.Time
}

type MemorySessionStore struct {
	mu      sync.Mutex
	entries map[SessionKind]map[string]memoryEntry
	now     func() time.Time
}

func NewMemorySessionStore() *MemorySessionStore {
	return &MemorySessionStore{
		entries: map[SessionKind]map[string]memoryEntry{
			KindGate:  {},
			KindAdmin: {},
		},
		now: time.Now,
	}
}

func (s *MemorySessionStore) Create(ctx context.Context, kind SessionKind, data SessionData, ttl time.Duration) (string, error) {
	token, err := newToken()
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries[kind][token] = memoryEntry{data: data, expires: s.now().Add(ttl)}
	return token, nil
}

func (s *MemorySessionStore) Get(ctx context.Context, kind SessionKind, token string) (SessionData, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[kind][token]
	if !ok || s.now().After(e.expires) {
		delete(s.entries[kind], token)
		return SessionData{}, ErrNoSession
	}
	return e.data, nil
}

func (s *MemorySessionStore) Delete(ctx context.Context, kind SessionKind, token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.entries[kind], token)
	return nil
}

func (s *MemorySessionStore) Close() error { return nil }
