package accountpool

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// RuntimeProbe 由上层（App）实现，供账号池在选号时排除“绑定实例当前已运行”的账号。
// 这保证 GUI 手动启动的实例不会被 worker 通过租约抢占（Manual/GUI mutex）。
// accountpool 作为纯数据/选号层，不直接依赖 browser/launchcode，仅通过该接口读取运行态。
type RuntimeProbe interface {
	IsRunning(profileId string) bool
}

// ProfileFactory 由上层（App）实现，供批量导入为每行创建一个绑定实例，返回实例 ID。
// 保持 accountpool 不依赖 browser/launchcode。
type ProfileFactory interface {
	CreateProfileForRow(row AccountBatchRow) (profileID string, err error)
}

// ProxyResolver 由上层（App）实现，供账号池解析代理名称/实例到代理 ID（用于批量导入与代理失败冷却）。
type ProxyResolver interface {
	// ProxyIDForName 按名称解析代理 ID；未找到或名称不唯一时返回空串。
	ProxyIDForName(name string) string
	// ProxyIDForProfile 解析绑定实例当前使用的代理 ID；无则返回空串。
	ProxyIDForProfile(profileID string) string
}

// AccountPoolService 账号池业务服务
type AccountPoolService struct {
	dao          AccountDAO
	leaseDAO     LeaseDAO
	db           *sql.DB
	runtime      RuntimeProbe
	profileFac   ProfileFactory
	proxyResolver ProxyResolver
}

// NewAccountPoolService 创建 AccountPoolService
func NewAccountPoolService(dao AccountDAO) *AccountPoolService {
	return &AccountPoolService{dao: dao}
}

// SetLeaseDAO 注入租约 DAO（与底层 *sql.DB 同源）
func (s *AccountPoolService) SetLeaseDAO(dao LeaseDAO) {
	s.leaseDAO = dao
}

// SetDB 注入底层 *sql.DB，供租约事务使用
func (s *AccountPoolService) SetDB(db *sql.DB) {
	s.db = db
}

// SetRuntimeProbe 注入运行态探针，用于选号时排除已运行实例
func (s *AccountPoolService) SetRuntimeProbe(p RuntimeProbe) {
	s.runtime = p
}

// SetProfileFactory 注入批量导入用的实例工厂
func (s *AccountPoolService) SetProfileFactory(f ProfileFactory) {
	s.profileFac = f
}

// SetProxyResolver 注入代理解析器（批量导入按名绑代理 + 代理失败冷却）
func (s *AccountPoolService) SetProxyResolver(r ProxyResolver) {
	s.proxyResolver = r
}

var (
	// ErrNoAvailableAccount 表示无可用账号（HTTP 409）
	ErrNoAvailableAccount = errors.New("no available account")
	// ErrLeaseNotFound 表示租约不存在（HTTP 404）
	ErrLeaseNotFound = errors.New("lease not found")
	// ErrLeaseNotHeld 表示租约非 held 状态（HTTP 409）
	ErrLeaseNotHeld = errors.New("lease is not held")
	// ErrLeaseStoreUnavailable 表示租约存储未注入（HTTP 503）
	ErrLeaseStoreUnavailable = errors.New("lease store is not available")
)

// Create 创建账号；若 input.BoundProfileID 非空则绑定到指定实例
func (s *AccountPoolService) Create(input AccountInput) (*Account, error) {
	if strings.TrimSpace(input.AccountName) == "" {
		return nil, fmt.Errorf("accountName is required")
	}
	account := buildAccountFromInput(uuid.NewString(), input)
	account.CreatedAt = time.Now().Format(time.RFC3339)
	account.UpdatedAt = account.CreatedAt
	if err := s.dao.Upsert(account); err != nil {
		return nil, err
	}
	return s.dao.GetByID(account.AccountID)
}

// Get 查询单个账号
func (s *AccountPoolService) Get(accountID string) (*Account, error) {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return nil, fmt.Errorf("accountId is required")
	}
	return s.dao.GetByID(accountID)
}

// List 查询账号列表，支持 platform / status / group_id 过滤
func (s *AccountPoolService) List(filter AccountFilter) ([]*Account, error) {
	return s.dao.List(filter)
}

// Update 更新账号；BoundProfileID 为空表示解除绑定
func (s *AccountPoolService) Update(accountID string, input AccountInput) (*Account, error) {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return nil, fmt.Errorf("accountId is required")
	}
	existing, err := s.dao.GetByID(accountID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(input.AccountName) == "" {
		return nil, fmt.Errorf("accountName is required")
	}

	account := buildAccountFromInput(accountID, input)
	account.CreatedAt = existing.CreatedAt
	account.UpdatedAt = time.Now().Format(time.RFC3339)
	account.LastUsedAt = existing.LastUsedAt
	account.DeletedAt = existing.DeletedAt
	if err := s.dao.Upsert(account); err != nil {
		return nil, err
	}
	return s.dao.GetByID(accountID)
}

// Delete 软删除账号
func (s *AccountPoolService) Delete(accountID string) error {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return fmt.Errorf("accountId is required")
	}
	return s.dao.SoftDelete(accountID, time.Now().Format(time.RFC3339))
}

func buildAccountFromInput(accountID string, input AccountInput) *Account {
	return &Account{
		AccountID:      accountID,
		AccountName:    strings.TrimSpace(input.AccountName),
		Platform:       strings.TrimSpace(input.Platform),
		AccountRef:     strings.TrimSpace(input.AccountRef),
		BoundProfileID: strings.TrimSpace(input.BoundProfileID),
		ProxyID:        strings.TrimSpace(input.ProxyID),
		Status:         normalizeStatus(input.Status),
		CooldownUntil:  strings.TrimSpace(input.CooldownUntil),
		Notes:          input.Notes,
		Tags:           normalizeTags(input.Tags),
		GroupID:        strings.TrimSpace(input.GroupID),
		Credential:     nonNilMap(input.Credential),
		Metadata:       nonNilMap(input.Metadata),
	}
}

func normalizeStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "disabled":
		return "disabled"
	case "":
		return "active"
	default:
		return strings.ToLower(strings.TrimSpace(status))
	}
}

func nonNilMap(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

// ──────────────────────────────────────────────────────────────────────────
// 租约（Lease）—— accountpool 拥有租约 DB 状态与可用性选择；启动/停止由上层 handler 完成。
// ──────────────────────────────────────────────────────────────────────────

func (s *AccountPoolService) leaseStoreReady() bool {
	return s.db != nil && s.leaseDAO != nil
}

// Lease 在数据库事务内选号并建立一条 held 租约。
//
// 选择条件（必须全部满足）：
//   - status = 'active' 且未软删除
//   - 未处于冷却：cooldown_until 为空或早于当前时间
//   - 已绑定实例（bound_profile_id 非空）
//   - platform 匹配
//   - tags_any：若提供，账号 tags 需至少包含其中之一
//   - 绑定实例当前未运行（通过 RuntimeProbe 排除 GUI 手动启动的实例）
//
// 防重复租约依赖唯一偏索引 idx_leases_one_held：同一 account_id 至多一条 status='held'。
// 若 INSERT 因该唯一约束冲突失败，说明被其他 worker 抢先租用，自动重试下一个候选账号；
// 若全部候选均不可用，返回 ErrNoAvailableAccount（HTTP 409）。
//
// 注意：本方法不启动实例；上层 handler 在租约成立后按 auto_start 决定是否启动。
func (s *AccountPoolService) Lease(input LeaseInput) (*Account, *Lease, error) {
	platform := strings.TrimSpace(input.Platform)
	if platform == "" {
		return nil, nil, fmt.Errorf("platform is required")
	}
	if !s.leaseStoreReady() {
		return nil, nil, ErrLeaseStoreUnavailable
	}

	ttl := input.TTLSec
	if ttl <= 0 {
		ttl = 900
	}
	purpose := strings.TrimSpace(input.Purpose)
	if purpose == "" {
		purpose = "scrape"
	}
	workerID := strings.TrimSpace(input.WorkerID)
	tagsAny := normalizeTags(input.TagsAny)

	now := time.Now().UTC()
	expiresAt := now.Add(time.Duration(ttl) * time.Second).Format(time.RFC3339)

	tx, err := s.db.Begin()
	if err != nil {
		return nil, nil, fmt.Errorf("开启租约事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	candidates, err := selectLeaseCandidates(tx, platform)
	if err != nil {
		return nil, nil, err
	}
	if len(tagsAny) > 0 {
		candidates = filterByTagsAny(candidates, tagsAny)
	}

	leaseID := uuid.NewString()
	for _, acc := range candidates {
		if strings.TrimSpace(acc.BoundProfileID) == "" {
			continue
		}
		// Manual/GUI mutex：绑定实例当前正在运行的账号不可被租约抢占。
		if s.runtime != nil && s.runtime.IsRunning(acc.BoundProfileID) {
			continue
		}

		lease := &Lease{
			LeaseID:   leaseID,
			AccountID: acc.AccountID,
			ProfileID: acc.BoundProfileID,
			WorkerID:  workerID,
			Purpose:   purpose,
			Status:    LeaseStatusHeld,
			ExpiresAt: expiresAt,
			LeasedAt:  now.Format(time.RFC3339),
			Metadata:  map[string]any{},
		}
		if err := s.leaseDAO.UpsertLease(tx, lease); err != nil {
			if isUniqueHeldConflict(err) {
				// 被其他 worker 抢先租用，重试下一个候选
				continue
			}
			return nil, nil, err
		}
		if err := tx.Commit(); err != nil {
			return nil, nil, fmt.Errorf("提交租约事务失败: %w", err)
		}
		return acc, lease, nil
	}

	return nil, nil, ErrNoAvailableAccount
}

// selectLeaseCandidates 在事务内查询符合基础条件的候选账号（status=active、未冷却、已绑定实例、platform 匹配）。
// 按 last_used_at 升序（空值最优先），使较少使用的账号优先被租用。
func selectLeaseCandidates(tx *sql.Tx, platform string) ([]*Account, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	rows, err := tx.Query(fmt.Sprintf(`
		SELECT %s FROM accounts
		WHERE COALESCE(deleted_at, '') = ''
		  AND status = ?
		  AND COALESCE(bound_profile_id, '') != ''
		  AND (cooldown_until = '' OR cooldown_until <= ?)
		  AND platform = ?
		ORDER BY CASE WHEN last_used_at = '' THEN 0 ELSE 1 END ASC, last_used_at ASC, created_at ASC`,
		accountColumns), AccountStatusActive, now, platform)
	if err != nil {
		return nil, fmt.Errorf("查询候选账号失败: %w", err)
	}
	defer rows.Close()

	var list []*Account
	for rows.Next() {
		a, err := scanAccount(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, a)
	}
	return list, rows.Err()
}

// filterByTagsAny 过滤出 tags 至少包含 tagsAny 中任一项的账号
func filterByTagsAny(accounts []*Account, tagsAny []string) []*Account {
	wanted := make(map[string]struct{}, len(tagsAny))
	for _, t := range tagsAny {
		wanted[t] = struct{}{}
	}
	out := make([]*Account, 0, len(accounts))
	for _, a := range accounts {
		for _, t := range a.Tags {
			if _, ok := wanted[t]; ok {
				out = append(out, a)
				break
			}
		}
	}
	return out
}

// isUniqueHeldConflict 判断错误是否为 idx_leases_one_held 唯一约束冲突（被其他 worker 抢先租用）。
func isUniqueHeldConflict(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique constraint")
}

// GetLease 查询租约
func (s *AccountPoolService) GetLease(leaseID string) (*Lease, error) {
	if !s.leaseStoreReady() {
		return nil, ErrLeaseStoreUnavailable
	}
	leaseID = strings.TrimSpace(leaseID)
	if leaseID == "" {
		return nil, ErrLeaseNotFound
	}
	lease, err := s.leaseDAO.GetLeaseByID(s.db, leaseID)
	if err != nil {
		if strings.Contains(err.Error(), "不存在") {
			return nil, ErrLeaseNotFound
		}
		return nil, err
	}
	return lease, nil
}

// GetActiveLease 返回指定账号当前持有的 held 租约；无 held 租约时返回 (nil, nil)。
// 用于 GUI 展示“账号是否被占用”并提供强制释放入口。
func (s *AccountPoolService) GetActiveLease(accountID string) (*Lease, error) {
	if !s.leaseStoreReady() {
		return nil, ErrLeaseStoreUnavailable
	}
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return nil, fmt.Errorf("accountId is required")
	}
	return s.leaseDAO.GetHeldByAccount(s.db, accountID)
}

// MarkLeaseStarted 标记租约已启动实例（写入 cdp_endpoint 与 auto_started=1）。
// 由上层 handler 在成功启动绑定实例后调用。
func (s *AccountPoolService) MarkLeaseStarted(leaseID, cdpEndpoint string) error {
	if !s.leaseStoreReady() {
		return ErrLeaseStoreUnavailable
	}
	return s.leaseDAO.UpdateLeaseStarted(s.db, leaseID, cdpEndpoint, 1)
}

// Heartbeat 续租：更新 heartbeat_at 与 expires_at（=now+ttl）。仅 held 租约可续。
func (s *AccountPoolService) Heartbeat(leaseID string, ttlSec int) (*Lease, error) {
	if !s.leaseStoreReady() {
		return nil, ErrLeaseStoreUnavailable
	}
	leaseID = strings.TrimSpace(leaseID)
	if leaseID == "" {
		return nil, ErrLeaseNotFound
	}
	if ttlSec <= 0 {
		ttlSec = 900
	}
	now := time.Now().UTC()
	expiresAt := now.Add(time.Duration(ttlSec) * time.Second).Format(time.RFC3339)
	if err := s.leaseDAO.UpdateLeaseHeartbeat(s.db, leaseID, expiresAt, now.Format(time.RFC3339), now.Format(time.RFC3339)); err != nil {
		if strings.Contains(err.Error(), "非 held") || strings.Contains(err.Error(), "not held") || strings.Contains(err.Error(), "已过期") {
			return nil, ErrLeaseNotHeld
		}
		if strings.Contains(err.Error(), "不存在") {
			return nil, ErrLeaseNotFound
		}
		return nil, err
	}
	return s.leaseDAO.GetLeaseByID(s.db, leaseID)
}

// Release 释放租约并按结果驱动账号状态：
//   - ok        -> active（清除冷却）
//   - risk      -> cooldown（cooldown_until = now + cooldownSec，默认 60min）
//   - ban       -> banned
//   - need_login-> need_login
//
// 返回释放后的账号状态。是否停止绑定实例由上层 handler 依据 lease.auto_started 决定。
func (s *AccountPoolService) Release(leaseID, result string, cooldownSec int) (*Lease, *Account, error) {
	if !s.leaseStoreReady() {
		return nil, nil, ErrLeaseStoreUnavailable
	}
	leaseID = strings.TrimSpace(leaseID)
	if leaseID == "" {
		return nil, nil, ErrLeaseNotFound
	}
	result = normalizeReleaseResult(result)

	lease, err := s.leaseDAO.GetLeaseByID(s.db, leaseID)
	if err != nil {
		if strings.Contains(err.Error(), "不存在") {
			return nil, nil, ErrLeaseNotFound
		}
		return nil, nil, err
	}
	if lease.Status != LeaseStatusHeld {
		return nil, nil, ErrLeaseNotHeld
	}

	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)

	tx, err := s.db.Begin()
	if err != nil {
		return nil, nil, fmt.Errorf("开启释放事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := s.leaseDAO.UpdateLeaseStatus(tx, leaseID, LeaseStatusReleased, nowStr, result); err != nil {
		// 条件更新：并发 Release 的第二个会命中“非 held”，返回 409
		if strings.Contains(err.Error(), "非 held") || strings.Contains(err.Error(), "not held") {
			return nil, nil, ErrLeaseNotHeld
		}
		if strings.Contains(err.Error(), "不存在") {
			return nil, nil, ErrLeaseNotFound
		}
		return nil, nil, err
	}

	status, cooldownUntil := releaseAccountStatus(result, now, cooldownSec)
	if err := s.dao.UpdateAccountStatus(tx, lease.AccountID, status, cooldownUntil, nowStr, nowStr); err != nil {
		return nil, nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, nil, fmt.Errorf("提交释放事务失败: %w", err)
	}

	updated, err := s.leaseDAO.GetLeaseByID(s.db, leaseID)
	if err != nil {
		updated = lease
	}
	account, err := s.dao.GetByID(lease.AccountID)
	if err != nil {
		return updated, nil, nil
	}
	return updated, account, nil
}

// ReleaseAccountStatus 根据释放结果返回账号目标状态与冷却到期时间。
func ReleaseAccountStatus(result string, now time.Time, cooldownSec int) (status, cooldownUntil string) {
	return releaseAccountStatus(normalizeReleaseResult(result), now, cooldownSec)
}

func releaseAccountStatus(result string, now time.Time, cooldownSec int) (status, cooldownUntil string) {
	switch result {
	case ReleaseResultRisk:
		if cooldownSec <= 0 {
			cooldownSec = 3600 // 默认 60 分钟
		}
		return AccountStatusCooldown, now.Add(time.Duration(cooldownSec) * time.Second).Format(time.RFC3339)
	case ReleaseResultBan:
		return AccountStatusBanned, ""
	case ReleaseResultNeedLogin:
		return AccountStatusNeedLogin, ""
	default: // ok
		return AccountStatusActive, ""
	}
}

func normalizeReleaseResult(result string) string {
	switch strings.ToLower(strings.TrimSpace(result)) {
	case ReleaseResultRisk:
		return ReleaseResultRisk
	case ReleaseResultBan:
		return ReleaseResultBan
	case ReleaseResultNeedLogin:
		return ReleaseResultNeedLogin
	default:
		return ReleaseResultOK
	}
}

// ReclaimExpired 回收已过期的 held 租约：标记为 expired，将账号恢复为 active（除非已设置 release_result）。
// 返回本次回收的租约列表，供上层（ticker）按 auto_started 决定是否停止绑定实例。
func (s *AccountPoolService) ReclaimExpired() ([]*Lease, error) {
	if !s.leaseStoreReady() {
		return nil, ErrLeaseStoreUnavailable
	}
	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)

	expired, err := s.leaseDAO.ListExpired(s.db, nowStr)
	if err != nil {
		return nil, err
	}
	if len(expired) == 0 {
		return nil, nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("开启回收事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	reclaimed := make([]*Lease, 0, len(expired))
	for _, lease := range expired {
		if err := s.leaseDAO.UpdateLeaseStatus(tx, lease.LeaseID, LeaseStatusExpired, nowStr, lease.ReleaseResult); err != nil {
			// 单条失败不中断整批，记录后继续
			continue
		}
		// 过期回收：将账号恢复为 active 并清除冷却（除非已被显式释放为 ban/need_login 等）
		if strings.TrimSpace(lease.ReleaseResult) == "" {
			if err := s.dao.UpdateAccountStatus(tx, lease.AccountID, AccountStatusActive, "", nowStr, nowStr); err != nil {
				continue
			}
		}
		reclaimed = append(reclaimed, lease)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("提交回收事务失败: %w", err)
	}
	return reclaimed, nil
}

// ──────────────────────────────────────────────────────────────────────────
// 批量导入 + 代理失败冷却（Phase 5）
// ──────────────────────────────────────────────────────────────────────────

// BatchImport 批量导入账号：为每行创建一个绑定实例（经 ProfileFactory），按名解析并绑定代理
// （未找到/歧义时跳过代理，与上游导入行为一致），再创建账号。单行失败不中断整批。
// 返回每行的结果（含成功账号或失败原因）。
func (s *AccountPoolService) BatchImport(rows []AccountBatchRow) []BatchImportResult {
	results := make([]BatchImportResult, 0, len(rows))
	for _, row := range rows {
		res := BatchImportResult{Row: row}
		platform := strings.TrimSpace(row.Platform)
		if platform == "" {
			res.Error = "platform is required"
			results = append(results, res)
			continue
		}
		if strings.TrimSpace(row.Username) == "" {
			res.Error = "username is required"
			results = append(results, res)
			continue
		}

		// 1) 创建绑定实例
		var profileID string
		if s.profileFac != nil {
			pid, err := s.profileFac.CreateProfileForRow(row)
			if err != nil {
				res.Error = "create profile failed: " + err.Error()
				results = append(results, res)
				continue
			}
			profileID = strings.TrimSpace(pid)
		}

		// 2) 按名解析代理（未找到/歧义则不绑定代理）
		var proxyID string
		if s.proxyResolver != nil && strings.TrimSpace(row.ProxyName) != "" {
			proxyID = strings.TrimSpace(s.proxyResolver.ProxyIDForName(row.ProxyName))
		}

		// 3) 创建账号
		accountName := strings.TrimSpace(row.Username)
		account, err := s.Create(AccountInput{
			AccountName:    accountName,
			Platform:       platform,
			AccountRef:     strings.TrimSpace(row.Username),
			BoundProfileID: profileID,
			ProxyID:        proxyID,
			Notes:          row.Notes,
			Tags:           row.Tags,
		})
		if err != nil {
			res.Error = "create account failed: " + err.Error()
			results = append(results, res)
			continue
		}
		res.Account = account
		results = append(results, res)
	}
	return results
}

// CooldownAccountsByProxy 将所有绑定到指定代理的账号置为 cooldown，cooldown_until=now+cooldownSec。
// 绑定关系二选一：account.proxy_id == proxyID，或 account.bound_profile_id 经 ProxyResolver 解析的代理 == proxyID。
// cooldownSec<=0 时默认 3600。返回受影响的账号 ID 列表。
func (s *AccountPoolService) CooldownAccountsByProxy(proxyID string, cooldownSec int) ([]string, error) {
	proxyID = strings.TrimSpace(proxyID)
	if proxyID == "" {
		return nil, fmt.Errorf("proxyId is required")
	}
	if cooldownSec <= 0 {
		cooldownSec = 3600
	}
	accounts, err := s.dao.List(AccountFilter{})
	if err != nil {
		return nil, fmt.Errorf("查询账号列表失败: %w", err)
	}
	now := time.Now().UTC()
	cooldownUntil := now.Add(time.Duration(cooldownSec) * time.Second).Format(time.RFC3339)
	nowStr := now.Format(time.RFC3339)

	affected := make([]string, 0)
	for _, acc := range accounts {
		if !s.accountBoundToProxy(acc, proxyID) {
			continue
		}
		// 保留更严重的终态：banned / need_login 不应被 cooldown 覆盖。
		if acc.Status == AccountStatusBanned || acc.Status == AccountStatusNeedLogin {
			continue
		}
		// 已经在更长的冷却里则不缩短。
		if acc.Status == AccountStatusCooldown && acc.CooldownUntil != "" && acc.CooldownUntil > cooldownUntil {
			continue
		}
		if err := s.dao.UpdateAccountStatus(s.db, acc.AccountID, AccountStatusCooldown, cooldownUntil, acc.LastUsedAt, nowStr); err != nil {
			continue
		}
		affected = append(affected, acc.AccountID)
	}
	return affected, nil
}

// accountBoundToProxy 判断账号是否绑定到指定代理（直接 proxy_id 或经实例解析）。
func (s *AccountPoolService) accountBoundToProxy(acc *Account, proxyID string) bool {
	if acc == nil {
		return false
	}
	if strings.TrimSpace(acc.ProxyID) == proxyID {
		return true
	}
	if s.proxyResolver != nil && strings.TrimSpace(acc.BoundProfileID) != "" {
		return strings.TrimSpace(s.proxyResolver.ProxyIDForProfile(acc.BoundProfileID)) == proxyID
	}
	return false
}