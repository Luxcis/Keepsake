package auth

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"keepsake/internal/store"
)

var (
	// ErrCredentials：用户名或密码错误（对客户端统一为“凭据错误”，不区分细分原因）。
	ErrCredentials = errors.New("invalid credentials")
	// ErrRateLimited：触发限速，客户端应稍后再试。
	ErrRateLimited = errors.New("too many attempts")
	// ErrFailClosed：限速后端（Redis）配置了但不可用 —— 按计划 §3.2 拒绝而非放行。
	ErrFailClosed = errors.New("auth backend unavailable")
	// ErrGateNotSet：站点访问口令尚未初始化（引导完成后启用）。
	ErrGateNotSet = errors.New("site access password not configured")
	// ErrGateMismatch：站点口令错误。
	ErrGateMismatch = errors.New("site password mismatch")
)

const (
	loginLimitPerWindow  = 5 // 5 次失败
	loginLimitWindow     = 15 * time.Minute
	unlockLimitPerWindow = 10
	unlockLimitWindow    = 15 * time.Minute
)

// 默认会话时长：门禁 30 天滑动续期；管理员 24 小时（计划 §5.1 会话过期）。
const (
	DefaultGateTTL  = 30 * 24 * time.Hour
	DefaultAdminTTL = 24 * time.Hour
)

type Service struct {
	st       *store.Store
	sessions SessionStore
	limiter  *Limiter
	log      *slog.Logger

	GateTTL  time.Duration
	AdminTTL time.Duration
	Secure   bool // Cookie Secure 位；本地 HTTP 联调时应显式关闭
}

func NewService(st *store.Store, sessions SessionStore, limiter *Limiter, log *slog.Logger, secure bool) *Service {
	return &Service{
		st: st, sessions: sessions, limiter: limiter, log: log,
		GateTTL: DefaultGateTTL, AdminTTL: DefaultAdminTTL, Secure: secure,
	}
}

// Kit 供中间件/处理器共享的只读会话资源。
func (s *Service) Sessions() SessionStore { return s.sessions }

// ---------- 引导（createadmin CLI 用） ----------

func (s *Service) CreateAdminUser(ctx context.Context, username, password string) error {
	if len(username) < 2 || len(username) > 64 {
		return errors.New("username length must be 2..64")
	}
	if len(password) < 8 {
		return errors.New("password too short (min 8)")
	}
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	return s.st.CreateAdmin(ctx, username, hash)
}

// ---------- 站点访问门禁 ----------

// Unlock 校验访问口令并发放门禁会话；口令未配置返回 ErrGateNotSet。
func (s *Service) Unlock(ctx context.Context, ip, password string) error {
	allowed, berr := s.limiter.Allow(ctx, "unlock:"+ip, unlockLimitPerWindow, unlockLimitWindow)
	if berr != nil {
		return ErrFailClosed
	}
	if !allowed {
		return ErrRateLimited
	}

	hash, err := s.st.GetSetting(ctx, "site_password")
	if err != nil {
		return err
	}
	if hash == "" {
		return ErrGateNotSet
	}
	if err := VerifyPassword(password, hash); err != nil {
		return ErrGateMismatch
	}
	return nil
}

// IssueGate 发放门禁会话（校验通过后调用）。
func (s *Service) IssueGate(ctx context.Context) (token string, err error) {
	return s.sessions.Create(ctx, KindGate, SessionData{}, s.GateTTL)
}

// ---------- 管理员登录 ----------

// decoyHash：用户不存在时仍执行一次 Argon2id，抹平账号枚举的时间差异。
var decoyHashOnce = func() string {
	h, err := HashPassword("timing-equalizer")
	if err != nil {
		return ""
	}
	return h
}()

func (s *Service) Login(ctx context.Context, ip, username, password string) (string, SessionData, error) {
	allowed, berr := s.limiter.Allow(ctx, "login:"+ip, loginLimitPerWindow, loginLimitWindow)
	if berr != nil {
		return "", SessionData{}, ErrFailClosed
	}
	if !allowed {
		return "", SessionData{}, ErrRateLimited
	}

	hash, err := s.st.AdminPasswordHash(ctx, username)
	switch {
	case err == store.ErrNoUser:
		_ = VerifyPassword(password, decoyHashOnce) // 等时
		return "", SessionData{}, ErrCredentials
	case err != nil:
		return "", SessionData{}, ErrFailClosed
	}

	if verr := VerifyPassword(password, hash); verr != nil {
		return "", SessionData{}, ErrCredentials
	}
	s.limiter.Reset(ctx, "login:"+ip)

	role, err := s.st.AdminRole(ctx, username)
	if err != nil {
		return "", SessionData{}, ErrFailClosed
	}
	data := SessionData{Username: username, Role: role}
	token, err := s.sessions.Create(ctx, KindAdmin, data, s.AdminTTL)
	if err != nil {
		return "", SessionData{}, ErrFailClosed
	}
	return token, data, nil
}

// ---------- 会话读取 / 注销 ----------

func (s *Service) LoadGate(ctx context.Context, token string) (SessionData, error) {
	return s.sessions.Get(ctx, KindGate, token)
}

func (s *Service) LoadAdmin(ctx context.Context, token string) (SessionData, error) {
	return s.sessions.Get(ctx, KindAdmin, token)
}

func (s *Service) Logout(ctx context.Context, gateToken, adminToken string) {
	if gateToken != "" {
		_ = s.sessions.Delete(ctx, KindGate, gateToken)
	}
	if adminToken != "" {
		_ = s.sessions.Delete(ctx, KindAdmin, adminToken)
	}
}

// ---------- Cookie 工具 ----------

// SessionCookie：HttpOnly + SameSite=Lax + Path=/。
// Lax 足以阻挡常见的跨站 GET-cookie 泄露；配合 Origin 校验覆盖不安全方法。
func SessionCookie(name, token string, ttl time.Duration, secure bool) *http.Cookie {
	return &http.Cookie{
		Name:     name,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
		MaxAge:   int(ttl / time.Second),
	}
}

func ClearedCookie(name string, secure bool) *http.Cookie {
	c := SessionCookie(name, "", -time.Hour, secure)
	c.Value = ""
	return c
}
