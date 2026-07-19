package launchcode

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"ant-chrome/backend/internal/accountpool"
)

// accountPoolService 由 accountpool.AccountPoolService 实现，避免直接依赖具体类型
type accountPoolService interface {
	Create(input accountpool.AccountInput) (*accountpool.Account, error)
	Get(accountID string) (*accountpool.Account, error)
	List(filter accountpool.AccountFilter) ([]*accountpool.Account, error)
	Update(accountID string, input accountpool.AccountInput) (*accountpool.Account, error)
	Delete(accountID string) error

	// 租约（Phase 3）
	Lease(input accountpool.LeaseInput) (*accountpool.Account, *accountpool.Lease, error)
	GetLease(leaseID string) (*accountpool.Lease, error)
	Heartbeat(leaseID string, ttlSec int) (*accountpool.Lease, error)
	Release(leaseID, result string, cooldownSec int) (*accountpool.Lease, *accountpool.Account, error)
	MarkLeaseStarted(leaseID, cdpEndpoint string) error
	ReclaimExpired() ([]*accountpool.Lease, error)
}

// SetAccountPoolService 注入账号池服务，供 HTTP API 使用
func (s *LaunchServer) SetAccountPoolService(svc accountPoolService) {
	s.mu.Lock()
	s.accountPool = svc
	s.mu.Unlock()
}

// handleAccounts /api/v1/pool/accounts
func (s *LaunchServer) handleAccounts(w http.ResponseWriter, r *http.Request) {
	if s.accountPool == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{
			"ok":    false,
			"error": "account pool is not available",
		})
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.handleListAccounts(w, r)
	case http.MethodPost:
		s.handleCreateAccount(w, r)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{
			"ok":    false,
			"error": "method not allowed",
		})
	}
}

// handleAccountByID /api/v1/pool/accounts/{id}
func (s *LaunchServer) handleAccountByID(w http.ResponseWriter, r *http.Request) {
	if s.accountPool == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{
			"ok":    false,
			"error": "account pool is not available",
		})
		return
	}

	accountID, ok := parseAccountPathID(r.URL.Path)
	if !ok {
		// 支持 /api/v1/pool/accounts/{id}/{start,stop} 子路径
		accountID, action, actionOK := parseAccountActionPath(r.URL.Path)
		if !actionOK {
			writeJSON(w, http.StatusNotFound, map[string]interface{}{
				"ok":    false,
				"error": "account not found",
			})
			return
		}
		switch action {
		case "start":
			s.handleAccountStart(w, r, accountID)
		case "stop":
			s.handleAccountStop(w, r, accountID)
		default:
			writeJSON(w, http.StatusNotFound, map[string]interface{}{"ok": false, "error": "account not found"})
		}
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.handleGetAccount(w, r, accountID)
	case http.MethodPut:
		s.handleUpdateAccount(w, r, accountID)
	case http.MethodDelete:
		s.handleDeleteAccount(w, r, accountID)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{
			"ok":    false,
			"error": "method not allowed",
		})
	}
}

// parseAccountActionPath 解析 /api/v1/pool/accounts/{id}/{start|stop}
func parseAccountActionPath(path string) (accountID, action string, ok bool) {
	rest := strings.TrimPrefix(path, "/api/v1/pool/accounts/")
	rest = strings.Trim(rest, "/")
	parts := strings.Split(rest, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func (s *LaunchServer) handleListAccounts(w http.ResponseWriter, r *http.Request) {
	filter := accountpool.AccountFilter{
		Platform: strings.TrimSpace(r.URL.Query().Get("platform")),
		Status:   strings.TrimSpace(r.URL.Query().Get("status")),
		GroupID:  strings.TrimSpace(r.URL.Query().Get("group_id")),
	}
	items, err := s.accountPool.List(filter)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}
	out := make([]accountpool.Account, 0, len(items))
	for _, item := range items {
		out = append(out, *item)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":       true,
		"accounts": out,
	})
}

func (s *LaunchServer) handleCreateAccount(w http.ResponseWriter, r *http.Request) {
	var input accountpool.AccountInput
	if err := decodeAccountBody(r, &input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	account, err := s.accountPool.Create(input)
	if err != nil {
		writeJSON(w, mapAccountWriteErrorStatus(err), map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"ok": true, "account": account})
}

func (s *LaunchServer) handleGetAccount(w http.ResponseWriter, _ *http.Request, accountID string) {
	account, err := s.accountPool.Get(accountID)
	if err != nil {
		writeJSON(w, mapAccountWriteErrorStatus(err), map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "account": account})
}

func (s *LaunchServer) handleUpdateAccount(w http.ResponseWriter, r *http.Request, accountID string) {
	var input accountpool.AccountInput
	if err := decodeAccountBody(r, &input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	account, err := s.accountPool.Update(accountID, input)
	if err != nil {
		writeJSON(w, mapAccountWriteErrorStatus(err), map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "account": account})
}

func (s *LaunchServer) handleDeleteAccount(w http.ResponseWriter, _ *http.Request, accountID string) {
	if err := s.accountPool.Delete(accountID); err != nil {
		writeJSON(w, mapAccountWriteErrorStatus(err), map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
}

// decodeAccountBody 解析 JSON 请求体；CSRF 守卫已要求 Content-Type 为 application/json
func decodeAccountBody(r *http.Request, out *accountpool.AccountInput) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	if err := dec.Decode(out); err != nil {
		if errors.Is(err, io.EOF) {
			return errors.New("request body is empty")
		}
		return errors.New("invalid request body")
	}
	return nil
}

// parseAccountPathID 从 /api/v1/pool/accounts/{id} 中解析账号 ID
func parseAccountPathID(path string) (string, bool) {
	path = strings.TrimPrefix(path, "/api/v1/pool/accounts/")
	path = strings.Trim(path, "/")
	id := strings.TrimSpace(path)
	if id == "" || strings.Contains(id, "/") {
		return "", false
	}
	return id, true
}

// mapAccountWriteErrorStatus 将账号服务的错误映射为 HTTP 状态码
func mapAccountWriteErrorStatus(err error) int {
	if err == nil {
		return http.StatusOK
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "not found"), strings.Contains(msg, "不存在"):
		return http.StatusNotFound
	case strings.Contains(msg, "is required"), strings.Contains(msg, "invalid"):
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}