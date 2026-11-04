package handlers

import (
	"context"
	"net/http"

	"keepsake/internal/auth"
)

// 会话中间件（handlers 包内实现，直接复用响应语义与错误结构）。

type ctxKey int

const ctxKeyAdmin ctxKey = iota

func adminFromCtx(r *http.Request) (auth.SessionData, bool) {
	v, ok := r.Context().Value(ctxKeyAdmin).(auth.SessionData)
	return v, ok
}

// withGate：私人内容读接口的门禁校验（计划 §6.1：未过门禁无法读取私人内容）。
// 管理员会话视同已解锁；站点口令未配置时视为开放模式（引导期/更新口令的过渡行为）。
func (h *Handlers) withGate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if pw, gerr := h.st.GetSetting(r.Context(), "site_password"); gerr == nil && pw == "" {
			next.ServeHTTP(w, r)
			return
		}

		if hc, cerr := r.Cookie(auth.AdminCookieName); cerr == nil && hc.Value != "" {
			data, aerr := h.svc.LoadAdmin(r.Context(), hc.Value)
			if aerr != nil {
				sessionErrToHTTP(w, aerr, authRequired)
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKeyAdmin, data)))
			return
		}

		gc, cerr := r.Cookie(auth.GateCookieName)
		if cerr != nil || gc.Value == "" {
			gateRequired(w)
			return
		}
		if _, gerr := h.svc.LoadGate(r.Context(), gc.Value); gerr != nil {
			sessionErrToHTTP(w, gerr, gateRequired)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// withAdmin：管理端接口的管理员会话校验（计划 §5.1）。
func (h *Handlers) withAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, cerr := r.Cookie(auth.AdminCookieName)
		if cerr != nil || c.Value == "" {
			authRequired(w)
			return
		}
		data, aerr := h.svc.LoadAdmin(r.Context(), c.Value)
		if aerr != nil {
			sessionErrToHTTP(w, aerr, authRequired)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKeyAdmin, data)))
	})
}
