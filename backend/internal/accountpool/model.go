package accountpool

// Account 账号模型（Phase 2）
//
// 列与 accounts 表一一对应；JSON tag 供 Wails 前端与 HTTP API 共用。
// 约定（与既有 browser_profiles 一致）：
//   - 时间字段使用 RFC3339 字符串
//   - tags / credential_json / metadata_json 以 JSON 文本持久化
//   - status：Phase 2 仅 active|disabled，Phase 3 将扩展 cooldown|banned|need_login
//   - 软删除通过 deleted_at 标记
type Account struct {
	AccountID      string            `json:"accountId"`
	AccountName    string            `json:"accountName"`
	Platform       string            `json:"platform"`     // xhs | x | other
	AccountRef     string            `json:"accountRef"`   // 用户名/uid
	BoundProfileID string            `json:"boundProfileId"`
	ProxyID        string            `json:"proxyId"`
	Status         string            `json:"status"` // active | disabled
	CooldownUntil  string            `json:"cooldownUntil"`
	Notes          string            `json:"notes"`
	Tags           []string         `json:"tags"`
	GroupID        string           `json:"groupId"`
	Credential     map[string]any   `json:"credential"`
	Metadata       map[string]any   `json:"metadata"`
	LastUsedAt     string           `json:"lastUsedAt"`
	CreatedAt      string            `json:"createdAt"`
	UpdatedAt      string            `json:"updatedAt"`
	DeletedAt      string            `json:"deletedAt"`
}

// AccountInput 创建/更新账号的输入
type AccountInput struct {
	AccountName    string            `json:"accountName"`
	Platform       string            `json:"platform"`
	AccountRef     string            `json:"accountRef"`
	BoundProfileID string            `json:"boundProfileId"`
	ProxyID        string            `json:"proxyId"`
	Status         string            `json:"status"`
	CooldownUntil  string            `json:"cooldownUntil"`
	Notes          string            `json:"notes"`
	Tags           []string          `json:"tags"`
	GroupID        string         `json:"groupId"`
	Credential     map[string]any `json:"credential"`
	Metadata       map[string]any `json:"metadata"`
}

// AccountFilter 列表过滤条件
type AccountFilter struct {
	Platform string
	Status   string
	GroupID  string
}