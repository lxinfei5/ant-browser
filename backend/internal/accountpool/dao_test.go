package accountpool

import (
	"database/sql"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// applyAccountsSchema 重建 accounts 表（v21 形态：含 email/phone，无 platform），
// 并建立 email/phone 的 LOWER() 部分唯一索引（模拟 v20/v21 迁移后的最终结构）。
func applyAccountsSchema(t *testing.T, db *sql.DB) {
	t.Helper()
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS accounts (
			account_id        TEXT PRIMARY KEY,
			account_name      TEXT NOT NULL,
			account_ref       TEXT NOT NULL DEFAULT '',
			email             TEXT NOT NULL DEFAULT '',
			phone             TEXT NOT NULL DEFAULT '',
			bound_profile_id  TEXT NOT NULL DEFAULT '',
			proxy_id          TEXT NOT NULL DEFAULT '',
			status            TEXT NOT NULL DEFAULT 'active',
			cooldown_until    TEXT NOT NULL DEFAULT '',
			notes             TEXT NOT NULL DEFAULT '',
			tags              TEXT NOT NULL DEFAULT '[]',
			group_id          TEXT NOT NULL DEFAULT '',
			credential_json   TEXT NOT NULL DEFAULT '{}',
			metadata_json     TEXT NOT NULL DEFAULT '{}',
			last_used_at      TEXT NOT NULL DEFAULT '',
			created_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			deleted_at        TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_accounts_email ON accounts(LOWER(email)) WHERE email<>'' AND COALESCE(deleted_at,'')=''`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_accounts_phone ON accounts(LOWER(phone)) WHERE phone<>'' AND COALESCE(deleted_at,'')=''`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("建表失败: %v", err)
		}
	}
}

func newTestDAO(t *testing.T) *SQLiteAccountDAO {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("打开内存库失败: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	applyAccountsSchema(t, db)
	return NewSQLiteAccountDAO(db)
}

func TestSQLiteAccountDAO_UpsertAndList(t *testing.T) {
	dao := newTestDAO(t)

	a := &Account{
		AccountID:   "acc-1",
		AccountName: "测试账号",
		AccountRef:  "uid123",
		Email:       "user@example.com",
		Phone:       "+8613800138000",
		Tags:        []string{"vip"},
		Credential:  map[string]any{"cookie": "abc"},
		Metadata:    map[string]any{"region": "cn"},
	}
	if err := dao.Upsert(a); err != nil {
		t.Fatalf("Upsert 失败: %v", err)
	}

	got, err := dao.GetByID("acc-1")
	if err != nil {
		t.Fatalf("GetByID 失败: %v", err)
	}
	if got.AccountName != "测试账号" || got.AccountRef != "uid123" {
		t.Fatalf("字段回读不一致: %+v", got)
	}
	if got.Email != "user@example.com" || got.Phone != "+8613800138000" {
		t.Fatalf("email/phone 回读不一致: %+v", got)
	}
	if len(got.Tags) != 1 || got.Tags[0] != "vip" {
		t.Fatalf("tags 回读不一致: %v", got.Tags)
	}
	if got.Credential["cookie"] != "abc" {
		t.Fatalf("credential 回读不一致: %v", got.Credential)
	}

	// upsert 更新
	a.AccountName = "更新后"
	a.Status = "disabled"
	a.Email = "new@example.com"
	if err := dao.Upsert(a); err != nil {
		t.Fatalf("二次 Upsert 失败: %v", err)
	}
	got, _ = dao.GetByID("acc-1")
	if got.AccountName != "更新后" || got.Status != "disabled" || got.Email != "new@example.com" {
		t.Fatalf("更新未生效: %+v", got)
	}

	// 列表过滤
	a2 := &Account{AccountID: "acc-2", AccountName: "B", Status: "active", GroupID: "g1"}
	_ = dao.Upsert(a2)

	list, err := dao.List(AccountFilter{Status: "active"})
	if err != nil {
		t.Fatalf("List 失败: %v", err)
	}
	if len(list) != 1 || list[0].AccountID != "acc-2" {
		t.Fatalf("status 过滤失败: %v", list)
	}

	list, _ = dao.List(AccountFilter{GroupID: "g1"})
	if len(list) != 1 || list[0].AccountID != "acc-2" {
		t.Fatalf("group_id 过滤失败: %v", list)
	}
}

func TestSQLiteAccountDAO_SoftDelete(t *testing.T) {
	dao := newTestDAO(t)
	a := &Account{AccountID: "acc-del", AccountName: "待删除"}
	_ = dao.Upsert(a)

	if err := dao.SoftDelete("acc-del", "2026-01-01T00:00:00Z"); err != nil {
		t.Fatalf("SoftDelete 失败: %v", err)
	}

	list, _ := dao.List(AccountFilter{})
	if len(list) != 0 {
		t.Fatalf("软删除后列表应为空: %v", list)
	}

	// GetByID 仍可取到（含 deleted_at）
	got, err := dao.GetByID("acc-del")
	if err != nil {
		t.Fatalf("软删除后应仍可 GetByID: %v", err)
	}
	if got.DeletedAt == "" {
		t.Fatalf("deleted_at 未写入")
	}

	// 不存在的账号软删除应报错
	err = dao.SoftDelete("nope", "2026-01-01T00:00:00Z")
	if err == nil || !strings.Contains(err.Error(), "不存在") {
		t.Fatalf("期望账号不存在错误, got: %v", err)
	}
}

// TestSQLiteAccountDAO_SoftDeleteFreesEmail 软删除后部分唯一索引不再约束该 email，
// 允许另一个账号复用同一 email（索引 WHERE 子句排除 deleted_at != ''）。
func TestSQLiteAccountDAO_SoftDeleteFreesEmail(t *testing.T) {
	dao := newTestDAO(t)
	first := &Account{AccountID: "acc-e1", AccountName: "A", Email: "dup@example.com"}
	if err := dao.Upsert(first); err != nil {
		t.Fatalf("Upsert 第一个账号失败: %v", err)
	}

	// 未删除时第二个同 email 账号应被唯一索引拒绝
	second := &Account{AccountID: "acc-e2", AccountName: "B", Email: "dup@example.com"}
	if err := dao.Upsert(second); err == nil {
		t.Fatalf("重复 email 应触发唯一索引冲突")
	}

	// 软删除第一个后，email 释放，可复用
	if err := dao.SoftDelete("acc-e1", "2026-01-01T00:00:00Z"); err != nil {
		t.Fatalf("SoftDelete 失败: %v", err)
	}
	if err := dao.Upsert(second); err != nil {
		t.Fatalf("软删除后应可复用 email: %v", err)
	}
}

func TestAccountPoolService_CreateRequiresName(t *testing.T) {
	dao := newTestDAO(t)
	svc := NewAccountPoolService(dao)
	if _, err := svc.Create(AccountInput{}); err == nil {
		t.Fatal("空 accountName 应报错")
	}

	acc, err := svc.Create(AccountInput{AccountName: "N", Tags: []string{"xhs"}})
	if err != nil {
		t.Fatalf("Create 失败: %v", err)
	}
	if acc.AccountID == "" || acc.Status != "active" {
		t.Fatalf("创建默认值异常: %+v", acc)
	}
}
