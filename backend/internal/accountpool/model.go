package accountpool

// Account 账号模型
//
// 列与 accounts 表一一对应；JSON tag 供 Wails 前端与 HTTP API 共用。
// 约定（与既有 browser_profiles 一致）：
//   - 时间字段使用 RFC3339 字符串
//   - tags / credential_json / metadata_json 以 JSON 文本持久化
//   - status：active | disabled | cooldown（cooldown 由代理失败/账号健康驱动）
//   - 软删除通过 deleted_at 标记
//
// 身份锚点（均为可选，仅 account_name 必填）：
//   - AccountRef 对应 account_ref 列，即「用户名/uid」（Wails/JSON 字段仍为 accountRef）
//   - Email / Phone 为一等列（v20 起），配合部分唯一索引保证未删除账号间唯一
//
// 平台归属不再使用独立 platform 列（v21 起物理删除）：xhs/x 等平台已归一入 tags（服务即标签）。
type Account struct {
	AccountID      string           `json:"accountId"`
	AccountName    string           `json:"accountName"`
	AccountRef     string           `json:"accountRef"` // 用户名/uid
	Email          string           `json:"email"`      // 邮箱（小写归一，可选）
	Phone          string           `json:"phone"`      // 手机号（归一，可选）
	BoundProfileID string           `json:"boundProfileId"`
	ProxyID        string           `json:"proxyId"`
	Status         string           `json:"status"` // active | disabled | cooldown
	CooldownUntil  string           `json:"cooldownUntil"`
	Notes          string           `json:"notes"`
	Tags           []string         `json:"tags"`
	GroupID        string           `json:"groupId"`
	Credential     map[string]any   `json:"credential"`
	Metadata       map[string]any   `json:"metadata"`
	LastUsedAt     string           `json:"lastUsedAt"`
	CreatedAt      string           `json:"createdAt"`
	UpdatedAt      string           `json:"updatedAt"`
	DeletedAt      string           `json:"deletedAt"`
}

// AccountInput 创建/更新账号的输入
//
// 仅 accountName 必填；email/phone/accountRef 均可选（历史仅命名的账号仍可编辑）。
// 不再接收/写入 platform：平台归属请使用 tags。
type AccountInput struct {
	AccountName    string           `json:"accountName"`
	AccountRef     string           `json:"accountRef"`
	Email          string           `json:"email"`
	Phone          string           `json:"phone"`
	BoundProfileID string           `json:"boundProfileId"`
	ProxyID        string           `json:"proxyId"`
	Status         string           `json:"status"`
	CooldownUntil  string           `json:"cooldownUntil"`
	Notes          string           `json:"notes"`
	Tags           []string         `json:"tags"`
	GroupID        string           `json:"groupId"`
	Credential     map[string]any   `json:"credential"`
	Metadata       map[string]any   `json:"metadata"`
}

// AccountFilter 列表过滤条件（平台已并入 tags，故不再按 platform 过滤）
type AccountFilter struct {
	Status  string
	GroupID string
}

// AccountStatus 账号状态
const (
	AccountStatusActive   = "active"
	AccountStatusDisabled = "disabled"
	AccountStatusCooldown = "cooldown"
)
