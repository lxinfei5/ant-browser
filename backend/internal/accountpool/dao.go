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