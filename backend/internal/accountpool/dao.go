package accountpool

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// AccountDAO 账号持久化接口
type AccountDAO interface {
	List(filter AccountFilter) ([]*Account, error)
	GetByID(accountID string) (*Account, error)
	Upsert(account *Account) error
	SoftDelete(accountID string, deletedAt string) error
	// UpdateAccountStatus 仅更新账号状态、冷却到期时间与 last_used_at（供租约释放/回收在事务内调用）。
	UpdateAccountStatus(runner sqlRunner, accountID, status, cooldownUntil, lastUsedAt, updatedAt string) error
}

// SQLiteAccountDAO 基于 SQLite 的 AccountDAO 实现
type SQLiteAccountDAO struct {
	db *sql.DB
}

// NewSQLiteAccountDAO 创建 SQLiteAccountDAO
func NewSQLiteAccountDAO(db *sql.DB) *SQLiteAccountDAO {
	return &SQLiteAccountDAO{db: db}
}

// accountColumns 查询列，COALESCE 兼容从旧版本直接升级时的缺失列
const accountColumns = `
	account_id, account_name, platform, account_ref,
	COALESCE(bound_profile_id, ''), COALESCE(proxy_id, ''),
	COALESCE(status, 'active'), COALESCE(cooldown_until, ''),
	COALESCE(notes, ''), COALESCE(tags, '[]'), COALESCE(group_id, ''),
	COALESCE(credential_json, '{}'), COALESCE(metadata_json, '{}'),
	COALESCE(last_used_at, ''), created_at, updated_at, COALESCE(deleted_at, '')
`

// List 查询未删除的账号，支持 platform / status / group_id 过滤
func (d *SQLiteAccountDAO) List(filter AccountFilter) ([]*Account, error) {
	var (
		conds []string
		args  []interface{}
	)
	conds = append(conds, "COALESCE(deleted_at, '') = ''")
	if v := strings.TrimSpace(filter.Platform); v != "" {
		conds = append(conds, "platform = ?")
		args = append(args, v)
	}
	if v := strings.TrimSpace(filter.Status); v != "" {
		conds = append(conds, "status = ?")
		args = append(args, v)
	}
	if v := strings.TrimSpace(filter.GroupID); v != "" {
		conds = append(conds, "group_id = ?")
		args = append(args, v)
	}

	query := fmt.Sprintf(`SELECT %s FROM accounts WHERE %s ORDER BY created_at ASC`,
		accountColumns, strings.Join(conds, " AND "))
	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("查询账号列表失败: %w", err)
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

// GetByID 根据 accountID 查询单个账号（含已删除）
func (d *SQLiteAccountDAO) GetByID(accountID string) (*Account, error) {
	row := d.db.QueryRow(
		fmt.Sprintf(`SELECT %s FROM accounts WHERE account_id = ?`, accountColumns),
		accountID,
	)
	a, err := scanAccount(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("账号不存在: %s", accountID)
	}
	return a, err
}

// Upsert 新增或更新账号（INSERT ... ON CONFLICT DO UPDATE）
func (d *SQLiteAccountDAO) Upsert(account *Account) error {
	tags, _ := json.Marshal(normalizeTags(account.Tags))
	credential, _ := json.Marshal(account.Credential)
	metadata, _ := json.Marshal(account.Metadata)

	now := time.Now().Format(time.RFC3339)
	if account.CreatedAt == "" {
		account.CreatedAt = now
	}
	if account.UpdatedAt == "" {
		account.UpdatedAt = now
	}
	if account.Status == "" {
		account.Status = "active"
	}

	_, err := d.db.Exec(`
		INSERT INTO accounts
		  (account_id, account_name, platform, account_ref, bound_profile_id, proxy_id,
		   status, cooldown_until, notes, tags, group_id, credential_json, metadata_json,
		   last_used_at, created_at, updated_at, deleted_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(account_id) DO UPDATE SET
		  account_name     = excluded.account_name,
		  platform          = excluded.platform,
		  account_ref       = excluded.account_ref,
		  bound_profile_id  = excluded.bound_profile_id,
		  proxy_id          = excluded.proxy_id,
		  status            = excluded.status,
		  cooldown_until    = excluded.cooldown_until,
		  notes             = excluded.notes,
		  tags              = excluded.tags,
		  group_id          = excluded.group_id,
		  credential_json   = excluded.credential_json,
		  metadata_json     = excluded.metadata_json,
		  last_used_at      = excluded.last_used_at,
		  deleted_at        = excluded.deleted_at,
		  updated_at        = excluded.updated_at`,
		account.AccountID, account.AccountName, account.Platform, account.AccountRef,
		account.BoundProfileID, account.ProxyID,
		account.Status, account.CooldownUntil, account.Notes, string(tags), account.GroupID,
		string(credential), string(metadata),
		account.LastUsedAt, account.CreatedAt, account.UpdatedAt, account.DeletedAt,
	)
	if err != nil {
		return fmt.Errorf("保存账号失败: %w", err)
	}
	return nil
}

// SoftDelete 通过 deleted_at 软删除账号
func (d *SQLiteAccountDAO) SoftDelete(accountID string, deletedAt string) error {
	result, err := d.db.Exec(`UPDATE accounts SET deleted_at = ?, updated_at = ? WHERE account_id = ?`,
		deletedAt, deletedAt, accountID)
	if err != nil {
		return fmt.Errorf("软删除账号失败: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return fmt.Errorf("账号不存在: %s", accountID)
	}
	return nil
}

// UpdateAccountStatus 在事务内更新账号状态、冷却到期时间与最近使用时间。
func (d *SQLiteAccountDAO) UpdateAccountStatus(runner sqlRunner, accountID, status, cooldownUntil, lastUsedAt, updatedAt string) error {
	result, err := runner.Exec(`
		UPDATE accounts
		   SET status = ?, cooldown_until = ?, last_used_at = ?, updated_at = ?
		 WHERE account_id = ?`,
		status, cooldownUntil, lastUsedAt, updatedAt, accountID)
	if err != nil {
		return fmt.Errorf("更新账号状态失败: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return fmt.Errorf("账号不存在: %s", accountID)
	}
	return nil
}

// scanner 统一扫描接口，兼容 *sql.Row 和 *sql.Rows
type scanner interface {
	Scan(dest ...any) error
}

func scanAccount(s scanner) (*Account, error) {
	var (
		tagsJSON, credentialJSON, metadataJSON string
		a                                      Account
	)
	err := s.Scan(
		&a.AccountID, &a.AccountName, &a.Platform, &a.AccountRef,
		&a.BoundProfileID, &a.ProxyID,
		&a.Status, &a.CooldownUntil, &a.Notes, &tagsJSON, &a.GroupID,
		&credentialJSON, &metadataJSON,
		&a.LastUsedAt, &a.CreatedAt, &a.UpdatedAt, &a.DeletedAt,
	)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(tagsJSON), &a.Tags)
	_ = json.Unmarshal([]byte(credentialJSON), &a.Credential)
	_ = json.Unmarshal([]byte(metadataJSON), &a.Metadata)
	if a.Tags == nil {
		a.Tags = []string{}
	}
	if a.Credential == nil {
		a.Credential = map[string]any{}
	}
	if a.Metadata == nil {
		a.Metadata = map[string]any{}
	}
	return &a, nil
}

func normalizeTags(tags []string) []string {
	if len(tags) == 0 {
		return []string{}
	}
	out := make([]string, 0, len(tags))
	for _, t := range tags {
		v := strings.TrimSpace(t)
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

// sqlRunner 抽象 *sql.DB 与 *sql.Tx 共同实现的执行接口，使租约 DAO 方法既可走事务也可走单连接。
type sqlRunner interface {
	Exec(query string, args ...any) (sql.Result, error)
	QueryRow(query string, args ...any) *sql.Row
	Query(query string, args ...any) (*sql.Rows, error)
}

// LeaseDAO 租约持久化接口
type LeaseDAO interface {
	UpsertLease(runner sqlRunner, lease *Lease) error
	GetLeaseByID(runner sqlRunner, leaseID string) (*Lease, error)
	GetHeldByAccount(runner sqlRunner, accountID string) (*Lease, error)
	ListHeld(runner sqlRunner) ([]*Lease, error)
	ListExpired(runner sqlRunner, now string) ([]*Lease, error)
	UpdateLeaseStatus(runner sqlRunner, leaseID, status, releasedAt, releaseResult string) error
	UpdateLeaseHeartbeat(runner sqlRunner, leaseID, expiresAt, heartbeatAt, nowRFC3339 string) error
	UpdateLeaseStarted(runner sqlRunner, leaseID, cdpEndpoint string, autoStarted int) error
}

// SQLiteLeaseDAO 基于 SQLite 的 LeaseDAO 实现
type SQLiteLeaseDAO struct {
	db *sql.DB
}

// NewSQLiteLeaseDAO 创建 SQLiteLeaseDAO
func NewSQLiteLeaseDAO(db *sql.DB) *SQLiteLeaseDAO {
	return &SQLiteLeaseDAO{db: db}
}

// leaseColumns 查询列，COALESCE 兼容从旧版本直接升级时的缺失列
const leaseColumns = `
	lease_id, account_id, profile_id, worker_id, purpose, status, cdp_endpoint,
	leased_at, expires_at, heartbeat_at, released_at, release_result, auto_started,
	metadata_json, created_at, updated_at
`

func (d *SQLiteLeaseDAO) UpsertLease(runner sqlRunner, lease *Lease) error {
	metadata, _ := json.Marshal(nonNilMap(lease.Metadata))
	if lease.Status == "" {
		lease.Status = LeaseStatusHeld
	}
	if lease.Purpose == "" {
		lease.Purpose = "scrape"
	}

	_, err := runner.Exec(`
		INSERT INTO account_leases
		  (lease_id, account_id, profile_id, worker_id, purpose, status, cdp_endpoint,
		   leased_at, expires_at, heartbeat_at, released_at, release_result, auto_started,
		   metadata_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(lease_id) DO UPDATE SET
		  account_id     = excluded.account_id,
		  profile_id      = excluded.profile_id,
		  worker_id       = excluded.worker_id,
		  purpose         = excluded.purpose,
		  status          = excluded.status,
		  cdp_endpoint    = excluded.cdp_endpoint,
		  expires_at      = excluded.expires_at,
		  heartbeat_at    = excluded.heartbeat_at,
		  released_at     = excluded.released_at,
		  release_result  = excluded.release_result,
		  auto_started    = excluded.auto_started,
		  metadata_json   = excluded.metadata_json,
		  updated_at      = excluded.updated_at`,
		lease.LeaseID, lease.AccountID, lease.ProfileID, lease.WorkerID, lease.Purpose, lease.Status, lease.CDPEndpoint,
		lease.LeasedAt, lease.ExpiresAt, lease.HeartbeatAt, lease.ReleasedAt, lease.ReleaseResult, lease.AutoStarted,
		string(metadata), lease.CreatedAt, lease.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("保存租约失败: %w", err)
	}
	return nil
}

func (d *SQLiteLeaseDAO) GetLeaseByID(runner sqlRunner, leaseID string) (*Lease, error) {
	row := runner.QueryRow(
		fmt.Sprintf(`SELECT %s FROM account_leases WHERE lease_id = ?`, leaseColumns),
		leaseID,
	)
	lease, err := scanLease(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("租约不存在: %s", leaseID)
	}
	return lease, err
}

// GetHeldByAccount 返回指定账号当前 status='held' 的租约（由 idx_leases_one_held 保证至多一条）。
// 无 held 租约时返回 (nil, nil)。
func (d *SQLiteLeaseDAO) GetHeldByAccount(runner sqlRunner, accountID string) (*Lease, error) {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return nil, nil
	}
	row := runner.QueryRow(
		fmt.Sprintf(`SELECT %s FROM account_leases WHERE account_id = ? AND status = 'held' LIMIT 1`, leaseColumns),
		accountID,
	)
	lease, err := scanLease(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return lease, err
}

// ListHeld 列出所有 status='held' 的租约
func (d *SQLiteLeaseDAO) ListHeld(runner sqlRunner) ([]*Lease, error) {
	rows, err := runner.Query(fmt.Sprintf(`SELECT %s FROM account_leases WHERE status = 'held' ORDER BY leased_at ASC`, leaseColumns))
	if err != nil {
		return nil, fmt.Errorf("查询 held 租约失败: %w", err)
	}
	defer rows.Close()
	return scanLeases(rows)
}

// ListExpired 列出 status='held' 且 expires_at 非空且早于 now 的租约
func (d *SQLiteLeaseDAO) ListExpired(runner sqlRunner, now string) ([]*Lease, error) {
	rows, err := runner.Query(fmt.Sprintf(`
		SELECT %s FROM account_leases
		WHERE status = 'held' AND expires_at != '' AND expires_at < ?
		ORDER BY expires_at ASC`, leaseColumns), now)
	if err != nil {
		return nil, fmt.Errorf("查询过期租约失败: %w", err)
	}
	defer rows.Close()
	return scanLeases(rows)
}

// UpdateLeaseStatus 更新租约状态（及可选的释放时间/结果）
// UpdateLeaseStatus 条件更新租约状态：仅当当前为 held 时才改为 released/expired/stolen，
// 0 rows 表示租约不存在或已非 held（并发 Release 的第二个调用会命中此分支，无 TOCTOU）。
func (d *SQLiteLeaseDAO) UpdateLeaseStatus(runner sqlRunner, leaseID, status, releasedAt, releaseResult string) error {
	result, err := runner.Exec(`
		UPDATE account_leases
		   SET status = ?, released_at = ?, release_result = ?, updated_at = ?
		 WHERE lease_id = ? AND status = 'held'`,
		status, releasedAt, releaseResult, time.Now().Format(time.RFC3339), leaseID)
	if err != nil {
		return fmt.Errorf("更新租约状态失败: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		// 区分“不存在”与“已非 held”：先查存在性
		var cnt int
		if e := runner.QueryRow(`SELECT COUNT(1) FROM account_leases WHERE lease_id = ?`, leaseID).Scan(&cnt); e == nil && cnt == 0 {
			return fmt.Errorf("租约不存在: %s", leaseID)
		}
		return fmt.Errorf("租约非 held: %s", leaseID)
	}
	return nil
}

// UpdateLeaseHeartbeat 更新心跳与过期时间：仅当当前为 held 且尚未过期（或无过期）时才续租，
// 已被回收/已过期/已释放的租约返回“非 held”错误。
func (d *SQLiteLeaseDAO) UpdateLeaseHeartbeat(runner sqlRunner, leaseID, expiresAt, heartbeatAt, nowRFC3339 string) error {
	result, err := runner.Exec(`
		UPDATE account_leases
		   SET expires_at = ?, heartbeat_at = ?, updated_at = ?
		 WHERE lease_id = ? AND status = 'held' AND (expires_at = '' OR expires_at >= ?)`,
		expiresAt, heartbeatAt, time.Now().Format(time.RFC3339), leaseID, nowRFC3339)
	if err != nil {
		return fmt.Errorf("更新租约心跳失败: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		var cnt int
		if e := runner.QueryRow(`SELECT COUNT(1) FROM account_leases WHERE lease_id = ?`, leaseID).Scan(&cnt); e == nil && cnt == 0 {
			return fmt.Errorf("租约不存在: %s", leaseID)
		}
		return fmt.Errorf("租约非 held 或已过期: %s", leaseID)
	}
	return nil
}

// UpdateLeaseStarted 标记租约已启动实例（写入 cdp_endpoint 与 auto_started）
func (d *SQLiteLeaseDAO) UpdateLeaseStarted(runner sqlRunner, leaseID, cdpEndpoint string, autoStarted int) error {
	result, err := runner.Exec(`
		UPDATE account_leases
		   SET cdp_endpoint = ?, auto_started = ?, updated_at = ?
		 WHERE lease_id = ?`,
		cdpEndpoint, autoStarted, time.Now().Format(time.RFC3339), leaseID)
	if err != nil {
		return fmt.Errorf("更新租约启动信息失败: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return fmt.Errorf("租约不存在: %s", leaseID)
	}
	return nil
}

func scanLease(s scanner) (*Lease, error) {
	var (
		metadataJSON string
		l            Lease
	)
	err := s.Scan(
		&l.LeaseID, &l.AccountID, &l.ProfileID, &l.WorkerID, &l.Purpose, &l.Status, &l.CDPEndpoint,
		&l.LeasedAt, &l.ExpiresAt, &l.HeartbeatAt, &l.ReleasedAt, &l.ReleaseResult, &l.AutoStarted,
		&metadataJSON, &l.CreatedAt, &l.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(metadataJSON), &l.Metadata)
	if l.Metadata == nil {
		l.Metadata = map[string]any{}
	}
	return &l, nil
}

func scanLeases(rows *sql.Rows) ([]*Lease, error) {
	var list []*Lease
	for rows.Next() {
		lease, err := scanLease(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, lease)
	}
	return list, rows.Err()
}