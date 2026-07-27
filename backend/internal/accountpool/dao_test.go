package accountpool

import (
	"database/sql"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// applyAccountsSchema 重建 accounts 表，模拟 v15 迁移
func applyAccountsSchema(t *testing.T, db *sql.DB) {
	t.Helper()
	schema := `
CREATE TABLE IF NOT EXISTS accounts (
	account_id        TEXT PRIMARY KEY,
	account_name      TEXT NOT NULL,
	platform          TEXT NOT NULL DEFAULT '',
	account_ref       TEXT NOT NULL DEFAULT '',
	bound_profile_id  TEXT NOT NULL DEFAULT '',
	proxy_id          TEXT NOT NULL DEFAULT '',
	status            TEXT NOT NULL DEFAULT 'active',
	cooldown_until    TEXT NOT NULL DEFAULT '',
	notes             TEXT NOT NULL DEFAULT '',
	tags              TEXT NOT NULL DEFAULT '[]',
	group_id          TEXT NOT NULL DEFAULT '',
	credential_json   TEXT NOT NULL DEFAULT '{}',
	metadata_json     TEXT NOT NULL DEFAULT '{}',
	icon_kind         TEXT NOT NULL DEFAULT '',
	icon_color        TEXT NOT NULL DEFAULT '',
	icon_text         TEXT NOT NULL DEFAULT '',
	icon_image        TEXT NOT NULL DEFAULT '',
	last_used_at      TEXT NOT NULL DEFAULT '',
	created_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	deleted_at        TEXT NOT NULL DEFAULT ''
)`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("建表失败: %v", err)
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
		Platform:    "xhs",
		AccountRef:  "uid123",
		Tags:        []string{"vip"},
		Credential: map[string]any{"cookie": "abc"},
		Metadata:    map[string]any{"region": "cn"},
	}
	if err := dao.Upsert(a); err != nil {
		t.Fatalf("Upsert 失败: %v", err)
	}

	got, err := dao.GetByID("acc-1")
	if err != nil {
		t.Fatalf("GetByID 失败: %v", err)
	}
	if got.AccountName != "测试账号" || got.Platform != "xhs" {
		t.Fatalf("字段回读不一致: %+v", got)
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
	if err := dao.Upsert(a); err != nil {
		t.Fatalf("二次 Upsert 失败: %v", err)
	}
	got, _ = dao.GetByID("acc-1")
	if got.AccountName != "更新后" || got.Status != "disabled" {
		t.Fatalf("更新未生效: %+v", got)
	}

	// 列表过滤
	a2 := &Account{AccountID: "acc-2", AccountName: "B", Platform: "x", Status: "active"}
	_ = dao.Upsert(a2)

	list, err := dao.List(AccountFilter{Platform: "xhs"})
	if err != nil {
		t.Fatalf("List 失败: %v", err)
	}
	if len(list) != 1 || list[0].AccountID != "acc-1" {
		t.Fatalf("platform 过滤失败: %v", list)
	}

	list, _ = dao.List(AccountFilter{Status: "active"})
	if len(list) != 1 || list[0].AccountID != "acc-2" {
		t.Fatalf("status 过滤失败: %v", list)
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

func TestAccountPoolService_CreateRequiresName(t *testing.T) {
	dao := newTestDAO(t)
	svc := NewAccountPoolService(dao)
	if _, err := svc.Create(AccountInput{}); err == nil {
		t.Fatal("空 accountName 应报错")
	}

	acc, err := svc.Create(AccountInput{AccountName: "N", Platform: "xhs"})
	if err != nil {
		t.Fatalf("Create 失败: %v", err)
	}
	if acc.AccountID == "" || acc.Status != "active" {
		t.Fatalf("创建默认值异常: %+v", acc)
	}
}