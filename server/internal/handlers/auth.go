package handlers

import (
	"errors"
	"log/slog"
	"net"
	"net/http"

	"keepsake/internal/auth"
)

// ---------- 站点访问门禁（计划 §5.1 站点访问） ----------

func (h *Handlers) unlock(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.Password == "" {
		err(w, http.StatusBadRequest, "bad_request", "缺少 password")
		return
	}

	peer := clientIP(r)
	if uerr := h.svc.Unlock(r.Context(), peer, in.Password); uerr != nil {
		h.unlockFail(w, uerr)
		return
	}

	token, terr := h.svc.IssueGate(r.Context())
	if terr != nil {
		backendUnavailable(w)
		return
	}
	http.SetCookie(w, auth.SessionCookie(auth.GateCookieName, token, h.svc.GateTTL, h.svc.Secure))
	Data(w, http.StatusOK, map[string]any{"unlocked": true})
}

func (h *Handlers) unlockFail(w http.ResponseWriter, uerr error) {
	switch {
	case errors.Is(uerr, auth.ErrRateLimited):
		w.Header().Set("Retry-After", "120")
		err(w, http.StatusTooManyRequests, "rate_limited", "尝试次数过多，请稍后再试")
	case errors.Is(uerr, auth.ErrFailClosed):
		backendUnavailable(w)
	case errors.Is(uerr, auth.ErrGateNotSet):
		err(w, http.StatusNotFound, "gate_not_set", "站点访问口令尚未启用")
	case errors.Is(uerr, auth.ErrGateMismatch):
		err(w, http.StatusUnauthorized, "gate_mismatch", "访问口令不正确")
	default:
		slogInternalError(uerr)
		err(w, http.StatusInternalServerError, "internal_error", "服务内部错误")
	}
}

// ---------- 管理员认证（计划 §5.1 管理员认证） ----------

func (h *Handlers) login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.Username == "" || in.Password == "" {
		err(w, http.StatusBadRequest, "bad_request", "缺少 username/password")
		return
	}

	token, data, lerr := h.svc.Login(r.Context(), clientIP(r), in.Username, in.Password)
	switch {
	case lerr == nil:
		http.SetCookie(w, auth.SessionCookie(auth.AdminCookieName, token, h.svc.AdminTTL, h.svc.Secure))
		Data(w, http.StatusOK, map[string]any{"username": data.Username, "role": data.Role})
	case errors.Is(lerr, auth.ErrRateLimited):
		w.Header().Set("Retry-After", "300")
		err(w, http.StatusTooManyRequests, "rate_limited", "登录失败次数过多，请 15 分钟后再试")
	case errors.Is(lerr, auth.ErrFailClosed):
		backendUnavailable(w)
	case errors.Is(lerr, auth.ErrCredentials):
		err(w, http.StatusUnauthorized, "invalid_credentials", "用户名或密码不正确")
	default:
		slogInternalError(lerr)
		err(w, http.StatusInternalServerError, "internal_error", "服务内部错误")
	}
}

func (h *Handlers) logout(w http.ResponseWriter, r *http.Request) {
	gateToken, adminToken := "", ""
	if c, cerr := r.Cookie(auth.GateCookieName); cerr == nil {
		gateToken = c.Value
	}
	if c, cerr := r.Cookie(auth.AdminCookieName); cerr == nil {
		adminToken = c.Value
	}
	h.svc.Logout(r.Context(), gateToken, adminToken)
	secure := h.svc.Secure
	http.SetCookie(w, auth.ClearedCookie(auth.GateCookieName, secure))
	http.SetCookie(w, auth.ClearedCookie(auth.AdminCookieName, secure))
	Data(w, http.StatusOK, map[string]any{"logged_out": true})
}

func (h *Handlers) me(w http.ResponseWriter, r *http.Request) {
	admin, ok := adminFromCtx(r)
	if !ok {
		authRequired(w)
		return
	}
	Data(w, http.StatusOK, map[string]any{"username": admin.Username, "role": admin.Role})
}

// ---------- 工具 ----------

func clientIP(r *http.Request) string {
	host, _, perr := net.SplitHostPort(r.RemoteAddr)
	if perr != nil {
		host = r.RemoteAddr
	}
	return host
}

func slogInternalError(e error) { slog.Error("auth error", "err", e) }
