package launchcode

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

const DefaultAPIKeyHeader = "X-Ant-Api-Key"

// APIAuthConfig 定义 LaunchServer 对 /api/* 请求的可选认证配置。
type APIAuthConfig struct {
	Enabled bool
	APIKey  string
	Header  string
}

func normalizeAPIAuthConfig(cfg APIAuthConfig) APIAuthConfig {
	cfg.APIKey = strings.TrimSpace(cfg.APIKey)
	cfg.Header = strings.TrimSpace(cfg.Header)
	if cfg.Header == "" {
		cfg.Header = DefaultAPIKeyHeader
	}
	return cfg
}

func (cfg APIAuthConfig) Requested() bool {
	return cfg.Enabled
}

func (cfg APIAuthConfig) Configured() bool {
	return cfg.APIKey != ""
}

func (cfg APIAuthConfig) Active() bool {
	return cfg.Requested() && cfg.Configured()
}

func (s *LaunchServer) SetAPIAuthConfig(cfg APIAuthConfig) {
	s.authMu.Lock()
	s.apiAuth = normalizeAPIAuthConfig(cfg)
	s.authMu.Unlock()
}

func (s *LaunchServer) apiAuthConfig() APIAuthConfig {
	s.authMu.RLock()
	cfg := s.apiAuth
	s.authMu.RUnlock()
	return cfg
}

func (s *LaunchServer) APIAuthHeader() string {
	return s.apiAuthConfig().Header
}

func (s *LaunchServer) APIAuthRequested() bool {
	return s.apiAuthConfig().Requested()
}

func (s *LaunchServer) APIAuthConfigured() bool {
	return s.apiAuthConfig().Configured()
}

func (s *LaunchServer) APIAuthEnabled() bool {
	return s.apiAuthConfig().Active()
}

func (s *LaunchServer) apiAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}

		cfg := s.apiAuthConfig()
		stateChanging := isStateChangingMethod(r.Method)

		// 状态变更请求统一要求 Content-Type: application/json（有 body 时），阻断表单类 CSRF。
		if stateChanging && hasBody(r) && !isJSONContentType(r) {
			writeJSON(w, http.StatusUnsupportedMediaType, map[string]interface{}{
				"ok":         false,
				"error":      "unsupported media type: require application/json",
				"authHeader": cfg.Header,
			})
			return
		}

		keyValid := false
		if cfg.Requested() && cfg.Configured() {
			providedKey := strings.TrimSpace(r.Header.Get(cfg.Header))
			keyValid = subtle.ConstantTimeCompare([]byte(providedKey), []byte(cfg.APIKey)) == 1
		}
		if keyValid {
			next.ServeHTTP(w, r)
			return
		}

		// 认证开启但 key 为空：FAIL CLOSED，拒绝所有 /api/* 请求，避免静默放行。
		if cfg.Requested() && !cfg.Configured() {
			writeJSON(w, http.StatusUnauthorized, map[string]interface{}{
				"ok":         false,
				"error":      "unauthorized: api auth enabled but no key configured",
				"authHeader": cfg.Header,
			})
			return
		}

		// 状态变更请求（无有效 key）：要求 same-origin，拒绝 cross-site / no-site，作为 CSRF 防护。
		if stateChanging {
			if isSameOriginFetch(r) {
				next.ServeHTTP(w, r)
				return
			}
			writeJSON(w, http.StatusForbidden, map[string]interface{}{
				"ok":         false,
				"error":      "forbidden: cross-site state-changing request rejected (require api key or same-origin)",
				"authHeader": cfg.Header,
			})
			return
		}

		// 非状态变更（GET 等）：auth 开启但 key 不匹配 -> 401；auth 未开启（opt-out）-> 放行。
		if cfg.Requested() {
			writeJSON(w, http.StatusUnauthorized, map[string]interface{}{
				"ok":         false,
				"error":      "unauthorized: invalid api key",
				"authHeader": cfg.Header,
			})
			return
		}

		next.ServeHTTP(w, r)
	})
}
