package launchcode

import (
	"net/http"
	"strings"

	"ant-chrome/backend/internal/accountpool"
)

// accountBatchImportRequest POST /api/v1/pool/accounts/batch-import
type accountBatchImportRequest struct {
	Rows []accountBatchImportRow `json:"rows"`
}

// accountBatchImportRow 与 accountpool.AccountBatchRow 对应，但 HTTP 使用 snake_case。
type accountBatchImportRow struct {
	Platform  string   `json:"platform"`
	Username  string   `json:"username"`
	ProxyName string   `json:"proxy_name"`
	Notes     string   `json:"notes"`
	Tags      []string `json:"tags"`
}

// handleAccountBatchImport POST /api/v1/pool/accounts/batch-import —— CSV/批量导入账号
func (s *LaunchServer) handleAccountBatchImport(w http.ResponseWriter, r *http.Request) {
	if s.accountPool == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"ok": false, "error": "account pool is not available"})
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
		return
	}
	var req accountBatchImportRequest
	if err := decodeLeaseBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	rows := make([]accountpool.AccountBatchRow, 0, len(req.Rows))
	for _, r := range req.Rows {
		rows = append(rows, accountpool.AccountBatchRow{
			Platform:  r.Platform,
			Username:  r.Username,
			ProxyName: r.ProxyName,
			Notes:     r.Notes,
			Tags:      r.Tags,
		})
	}
	results := s.accountPool.BatchImport(rows)
	created, failed := 0, 0
	for _, res := range results {
		if res.Error != "" {
			failed++
		} else {
			created++
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":      true,
		"results": results,
		"created": created,
		"failed":  failed,
	})
}

// poolProxyCooldownRequest POST /api/v1/pool/proxies/{id}/cooldown-accounts
type poolProxyCooldownRequest struct {
	CooldownSec int `json:"cooldown_sec"`
}

// handlePoolProxyByID /api/v1/pool/proxies/{id}/{cooldown-accounts}
func (s *LaunchServer) handlePoolProxyByID(w http.ResponseWriter, r *http.Request) {
	if s.accountPool == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"ok": false, "error": "account pool is not available"})
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/pool/proxies/")
	rest = strings.Trim(rest, "/")
	parts := strings.Split(rest, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		writeJSON(w, http.StatusNotFound, map[string]interface{}{"ok": false, "error": "proxy not found"})
		return
	}
	proxyID, action := parts[0], parts[1]
	switch action {
	case "cooldown-accounts":
		s.handlePoolProxyCooldownAccounts(w, r, proxyID)
	default:
		writeJSON(w, http.StatusNotFound, map[string]interface{}{"ok": false, "error": "proxy not found"})
	}
}

func (s *LaunchServer) handlePoolProxyCooldownAccounts(w http.ResponseWriter, r *http.Request, proxyID string) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method not allowed"})
		return
	}
	var req poolProxyCooldownRequest
	_ = decodeLeaseBody(r, &req) // body 可为空，使用默认冷却时长
	affected, err := s.accountPool.CooldownAccountsByProxy(proxyID, req.CooldownSec)
	if err != nil {
		writeJSON(w, mapAccountWriteErrorStatus(err), map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":          true,
		"proxyId":     proxyID,
		"affected":    affected,
		"affectedCount": len(affected),
	})
}