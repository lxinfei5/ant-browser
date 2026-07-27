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
	IconKind       string           `json:"iconKind"`  // "" | color | text | image（空=不定制 Dock 图标）
	IconColor      string           `json:"iconColor"` // 底色（color/text 用）
	IconText       string           `json:"iconText"`  // 首字母/短名（text 用）
	IconImage      string           `json:"iconImage"` // 主图 PNG 引用（image 用）
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
	IconKind       string         `json:"iconKind"`
	IconColor      string         `json:"iconColor"`
	IconText       string         `json:"iconText"`
	IconImage      string         `json:"iconImage"`
}

// AccountFilter 列表过滤条件
type AccountFilter struct {
	Platform string
	Status   string
	GroupID  string
}

// Lease 账号租约模型（Phase 3）
//
// 列与 account_leases 表一一对应。租约表示某个 worker 在一段时间内独占某个账号及其绑定实例。
// 约定：
//   - expires_at / heartbeat_at / released_at 使用 RFC3339 字符串；expires_at 为空表示无过期
//   - status：held | released | expired | stolen
//   - purpose：manual | scrape | warmup
//   - release_result：ok | risk | ban | need_login
//   - auto_started：1 表示本次租约启动了实例，release/expire 时需由上层停止实例
//   - metadata_json 以 JSON 文本持久化
//
// 防重复租约由数据库唯一偏索引 idx_leases_one_held 保证：同一 account_id 至多一条 status='held'。
type Lease struct {
	LeaseID       string         `json:"leaseId"`
	AccountID     string         `json:"accountId"`
	ProfileID     string         `json:"profileId"`
	WorkerID      string         `json:"workerId"`
	Purpose       string         `json:"purpose"`
	Status        string         `json:"status"`
	CDPEndpoint   string         `json:"cdpEndpoint"`
	LeasedAt      string         `json:"leasedAt"`
	ExpiresAt     string         `json:"expiresAt"`
	HeartbeatAt   string         `json:"heartbeatAt"`
	ReleasedAt    string         `json:"releasedAt"`
	ReleaseResult string         `json:"releaseResult"`
	AutoStarted   int            `json:"autoStarted"`
	Metadata      map[string]any `json:"metadata"`
	CreatedAt     string         `json:"createdAt"`
	UpdatedAt     string         `json:"updatedAt"`
}

// LeaseInput 创建租约的输入
type LeaseInput struct {
	Platform string   `json:"platform"`
	WorkerID string   `json:"workerId"`
	TTLSec   int      `json:"ttlSec"`
	Purpose  string   `json:"purpose"`
	TagsAny  []string `json:"tagsAny"`
}

// LeaseReleaseResult 释放租约时对账号状态的处置结果
const (
	ReleaseResultOK        = "ok"
	ReleaseResultRisk      = "risk"
	ReleaseResultBan       = "ban"
	ReleaseResultNeedLogin = "need_login"
)

// LeaseStatus 租约状态
const (
	LeaseStatusHeld     = "held"
	LeaseStatusReleased = "released"
	LeaseStatusExpired  = "expired"
	LeaseStatusStolen   = "stolen"
)

// AccountStatus 账号状态（Phase 3 扩展）
const (
	AccountStatusActive    = "active"
	AccountStatusDisabled  = "disabled"
	AccountStatusCooldown = "cooldown"
	AccountStatusBanned    = "banned"
	AccountStatusNeedLogin = "need_login"
)

// AccountBatchRow CSV 批量导入的单行输入（Phase 5）。
type AccountBatchRow struct {
	Platform  string   `json:"platform"`
	Username  string   `json:"username"`
	ProxyName string   `json:"proxyName"`
	Notes     string   `json:"notes"`
	Tags      []string `json:"tags"`
}

// BatchImportResult 单行导入结果：成功时 Account 非空，失败时 Error 非空。
type BatchImportResult struct {
	Row     AccountBatchRow `json:"row"`
	Account *Account        `json:"account"`
	Error   string          `json:"error"`
}