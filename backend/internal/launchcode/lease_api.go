package launchcode

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"ant-chrome/backend/internal/accountpool"
)

// leaseCreateRequest POST /api/v1/pool/lease 的请求体
type leaseCreateRequest struct {
	Platform  string   `json:"platform"`
	WorkerID  string   `json:"worker_id"`
	TTLSec    int      `json:"ttl_sec"`
	AutoStart bool     `json:"auto_start"`
	TagsAny   []string `json:"tags_any"`
	Purpose   string   `json:"purpose"`
}

// leaseHeartbeatRequest POST /api/v1/pool/lease/{id}/heartbeat
type leaseHeartbeatRequest struct {
	TTLSec int `json:"ttl_sec"`
}

// leaseReleaseRequest POST /api/v1/pool/lease/{id}/release
type leaseReleaseRequest struct {
	Result      string `json:"result"`
	CooldownSec int    `json:"cooldown_sec"`
}

// AcquireSession 是租约 handler 专用的轻量“启动并接管”入口（map.start_path）。
//
// 它组装当前 HTTP 层散落的步骤为一个可直接调用的 Go 方法：启动绑定实例 ->
// 等待调试端口就绪并 SetActiveProfile -> 返回 (debugPort, cdpUrl, error)。
// 不经过 HTTP，直接调用注入的 *LaunchServer / *App。
func (s *LaunchServer) AcquireSession(profileID string, timeout time.Duration) (debugPort int, cdpUrl string, err error) {
	profileID = strings.TrimSpace(profileID)
	if profileID == "" {
		return 0, "", errors.New("profileId is required")
	}
	if timeout <= 0 {
		timeout = defaultRuntimeSessionTimeout
	}

	profile, err := s.launchProfile(profileID, LaunchRequestParams{})
	if err != nil {
		return 0, "", err
	}
	profile, _, err = s.prepareRuntimeSession(profile, timeout)
	if err != nil {
		return 0, "", err
	}
	if profile == nil {
		return 0, "", errors.New("runtime session is not available")
	}

	s.SetActiveProfile(profile)
	cdp := s.CDPURL()
	if cdp == "" && profile.DebugReady && profile.DebugPort > 0 {
		cdp = fmt.Sprintf("http://127.0.0.1:%d", profile.DebugPort)
	}
	if cdp == "" {
		return 0, "", errors.New("cdp url is not available")
	}
	return profile.DebugPort, cdp, nil
}

// handleLease POST /api/v1/pool/lease —— 选号 + 建租约 + 可选 auto_start
func (s *LaunchServer) handleLease(w http.ResponseWriter, r *http.Request) {
	if s.accountPool == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"ok": false, "error": "account pool is not available"})
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
		return
	}

	var req leaseCreateRequest
	if err := decodeLeaseBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}

	account, lease, err := s.accountPool.Lease(accountpool.LeaseInput{
		Platform: req.Platform,
		WorkerID: req.WorkerID,
		TTLSec:   req.TTLSec,
		Purpose:  req.Purpose,
		TagsAny:  req.TagsAny,
	})
	if err != nil {
		writeJSON(w, mapLeaseErrorStatus(err), map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}

	// instance_code：绑定实例的 launchCode
	instanceCode := s.resolveProfileLaunchCode(account.BoundProfileID, "")

	var cdpURL string
	if req.AutoStart {
		_, url, startErr := s.AcquireSession(account.BoundProfileID, defaultRuntimeSessionTimeout)
		if startErr != nil {
			// auto_start 失败：回滚租约（释放回 active），返回 502
			_, _, _ = s.accountPool.Release(lease.LeaseID, accountpool.ReleaseResultOK, 0)
			writeJSON(w, http.StatusBadGateway, map[string]interface{}{
				"ok":      false,
				"error":   "auto_start failed: " + startErr.Error(),
				"leaseId": lease.LeaseID,
			})
			return
		}
		cdpURL = url
		_ = s.accountPool.MarkLeaseStarted(lease.LeaseID, cdpURL)
		lease.AutoStarted = 1
		lease.CDPEndpoint = cdpURL
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":           true,
		"lease_id":     lease.LeaseID,
		"accountId":    account.AccountID,
		"account_id":   account.AccountID,
		"instance_code": instanceCode,
		"cdp_url":      cdpURL,
		"proxy_summary": s.proxySummaryForAccount(account),
		"expires_at":   lease.ExpiresAt,
	})
}

// handleLeaseByID POST /api/v1/pool/lease/{id}/{heartbeat|release}
func (s *LaunchServer) handleLeaseByID(w http.ResponseWriter, r *http.Request) {
	if s.accountPool == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"ok": false, "error": "account pool is not available"})
		return
	}

	leaseID, action, ok := parseLeasePath(r.URL.Path)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]interface{}{"ok": false, "error": "lease not found"})
		return
	}

	switch action {
	case "heartbeat":
		s.handleLeaseHeartbeat(w, r, leaseID)
	case "release":
		s.handleLeaseRelease(w, r, leaseID)
	default:
		writeJSON(w, http.StatusNotFound, map[string]interface{}{"ok": false, "error": "lease not found"})
	}
}

func (s *LaunchServer) handleLeaseHeartbeat(w http.ResponseWriter, r *http.Request, leaseID string) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
		return
	}
	var req leaseHeartbeatRequest
	_ = decodeLeaseBody(r, &req) // body 可为空

	lease, err := s.accountPool.Heartbeat(leaseID, req.TTLSec)
	if err != nil {
		writeJSON(w, mapLeaseErrorStatus(err), map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":        true,
		"lease_id":  lease.LeaseID,
		"leaseId":   lease.LeaseID,
		"expires_at": lease.ExpiresAt,
	})
}

func (s *LaunchServer) handleLeaseRelease(w http.ResponseWriter, r *http.Request, leaseID string) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
		return
	}
	var req leaseReleaseRequest
	if err := decodeLeaseBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}

	lease, account, err := s.accountPool.Release(leaseID, req.Result, req.CooldownSec)
	if err != nil {
		writeJSON(w, mapLeaseErrorStatus(err), map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}

	// 若租约自动启动了实例，则释放时停止绑定实例。
	if lease != nil && lease.AutoStarted == 1 {
		_, _, _ = s.stopProfile(lease.ProfileID)
	}

	accountStatus := ""
	if account != nil {
		accountStatus = account.Status
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":            true,
		"lease_id":      lease.LeaseID,
		"leaseId":       lease.LeaseID,
		"account_id":    lease.AccountID,
		"accountId":     lease.AccountID,
		"account_status": accountStatus,
	})
}

// handleAccountStart POST /api/v1/pool/accounts/{id}/start —— 启动账号绑定的实例
func (s *LaunchServer) handleAccountStart(w http.ResponseWriter, r *http.Request, accountID string) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
		return
	}
	account, err := s.accountPool.Get(accountID)
	if err != nil {
		writeJSON(w, mapAccountWriteErrorStatus(err), map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	if strings.TrimSpace(account.BoundProfileID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "account has no bound profile"})
		return
	}
	debugPort, cdpURL, startErr := s.AcquireSession(account.BoundProfileID, defaultRuntimeSessionTimeout)
	if startErr != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": startErr.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":         true,
		"accountId":  account.AccountID,
		"cdp_url":    cdpURL,
		"debug_port": debugPort,
	})
}

// handleAccountStop POST /api/v1/pool/accounts/{id}/stop —— 停止账号绑定的实例
func (s *LaunchServer) handleAccountStop(w http.ResponseWriter, r *http.Request, accountID string) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
		return
	}
	account, err := s.accountPool.Get(accountID)
	if err != nil {
		writeJSON(w, mapAccountWriteErrorStatus(err), map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	if strings.TrimSpace(account.BoundProfileID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "account has no bound profile"})
		return
	}
	profile, status, errMsg := s.stopProfile(account.BoundProfileID)
	if errMsg != "" {
		writeJSON(w, status, map[string]interface{}{"ok": false, "error": errMsg})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":        true,
		"accountId": account.AccountID,
		"stopped":   profile == nil || !profile.Running,
	})
}

// proxySummaryForAccount 返回账号绑定实例的代理摘要（优先 ProxyBindName，回退 ProxyId / account.ProxyID）。
func (s *LaunchServer) proxySummaryForAccount(account *accountpool.Account) string {
	if account == nil {
		return ""
	}
	if profile, _, _ := s.profileSnapshotByID(account.BoundProfileID); profile != nil {
		if name := strings.TrimSpace(profile.ProxyBindName); name != "" {
			return name
		}
		if id := strings.TrimSpace(profile.ProxyId); id != "" {
			return id
		}
	}
	return strings.TrimSpace(account.ProxyID)
}

// decodeLeaseBody 解析 JSON 请求体；CSRF 守卫已要求 Content-Type 为 application/json
func decodeLeaseBody(r *http.Request, out any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	if err := dec.Decode(out); err != nil {
		if errors.Is(err, io.EOF) {
			return nil // body 可为空（heartbeat）
		}
		return errors.New("invalid request body")
	}
	return nil
}

// parseLeasePath 从 /api/v1/pool/lease/{id}/{action} 解析租约 ID 与动作
func parseLeasePath(path string) (leaseID, action string, ok bool) {
	rest := strings.TrimPrefix(path, "/api/v1/pool/lease/")
	rest = strings.Trim(rest, "/")
	parts := strings.Split(rest, "/")
	if len(parts) != 2 || parts[0] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

// mapLeaseErrorStatus 将租约服务错误映射为 HTTP 状态码
func mapLeaseErrorStatus(err error) int {
	if err == nil {
		return http.StatusOK
	}
	switch {
	case errors.Is(err, accountpool.ErrNoAvailableAccount):
		return http.StatusConflict
	case errors.Is(err, accountpool.ErrLeaseNotFound):
		return http.StatusNotFound
	case errors.Is(err, accountpool.ErrLeaseNotHeld):
		return http.StatusConflict
	case errors.Is(err, accountpool.ErrLeaseStoreUnavailable):
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}