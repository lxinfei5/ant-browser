package backend

import (
	"path/filepath"
	"testing"

	"ant-chrome/backend/internal/accountpool"
	"ant-chrome/backend/internal/browser"
	"ant-chrome/backend/internal/config"
	"ant-chrome/backend/internal/database"
)

// newTagTestApp 构造一个共用同一 SQLite 的 App(browserMgr + accountPool),用于三清删除测试。
func newTagTestApp(t *testing.T) (*App, *database.DB) {
	t.Helper()
	db, err := database.NewDB(filepath.Join(t.TempDir(), "app_tags.db"))
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	m := browser.NewManager(&config.Config{}, t.TempDir())
	m.ProfileDAO = browser.NewSQLiteProfileDAO(db.GetConn())
	m.TagRegistry = browser.NewSQLiteTagRegistryDAO(db.GetConn())

	app := &App{
		browserMgr:  m,
		accountPool: accountpool.NewAccountPoolService(accountpool.NewSQLiteAccountDAO(db.GetConn())),
	}
	return app, db
}

// TestBrowserDeleteTag_PurgesRegistryProfilesAccounts 验证「删除三清」:
// 同一标签以不同 casing 分别挂在实例、账号、注册表上,一次删除后三处都被清空。
func TestBrowserDeleteTag_PurgesRegistryProfilesAccounts(t *testing.T) {
	app, db := newTagTestApp(t)

	// 实例挂 "OpenCode"(写入已归一为 opencode)
	p, err := app.browserMgr.Create(browser.ProfileInput{ProfileName: "inst", Tags: []string{"OpenCode"}})
	if err != nil {
		t.Fatalf("Create profile: %v", err)
	}
	// 账号挂 "opencode"(经 DAO 归一)
	if err := accountpool.NewSQLiteAccountDAO(db.GetConn()).Upsert(&accountpool.Account{
		AccountID: "acc1", AccountName: "acc1", Tags: []string{"opencode", "keep"},
		Credential: map[string]any{}, Metadata: map[string]any{},
	}); err != nil {
		t.Fatalf("seed account: %v", err)
	}
	// 注册表有 "OPENCODE"(DAO 归一为 opencode)
	if err := app.browserMgr.TagRegistry.Ensure("OPENCODE"); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	// 用又一种 casing 删除
	if err := app.BrowserDeleteTag(" openCode "); err != nil {
		t.Fatalf("BrowserDeleteTag: %v", err)
	}

	// ① 注册表空
	if tags, _ := app.browserMgr.TagRegistry.List(); len(tags) != 0 {
		t.Fatalf("registry after delete = %v, want empty", tags)
	}
	// ② 实例空
	got, err := app.browserMgr.ProfileDAO.GetById(p.ProfileId)
	if err != nil {
		t.Fatalf("GetById: %v", err)
	}
	if len(got.Tags) != 0 {
		t.Fatalf("profile tags after delete = %v, want empty", got.Tags)
	}
	// ③ 账号只剩 keep(标签被剥掉)
	acc, err := app.accountPool.Get("acc1")
	if err != nil {
		t.Fatalf("Get account: %v", err)
	}
	if len(acc.Tags) != 1 || acc.Tags[0] != "keep" {
		t.Fatalf("account tags after delete = %v, want [keep]", acc.Tags)
	}
}

// TestBrowserDeleteTag_NilAccountPool 账号池为 nil 时,注册表与实例仍被清理,且不 panic。
func TestBrowserDeleteTag_NilAccountPool(t *testing.T) {
	app, _ := newTagTestApp(t)
	app.accountPool = nil

	p, err := app.browserMgr.Create(browser.ProfileInput{ProfileName: "inst", Tags: []string{"vip"}})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := app.browserMgr.TagRegistry.Ensure("vip"); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	if err := app.BrowserDeleteTag("vip"); err != nil {
		t.Fatalf("BrowserDeleteTag with nil accountPool: %v", err)
	}
	if tags, _ := app.browserMgr.TagRegistry.List(); len(tags) != 0 {
		t.Fatalf("registry after delete = %v, want empty", tags)
	}
	got, _ := app.browserMgr.ProfileDAO.GetById(p.ProfileId)
	if len(got.Tags) != 0 {
		t.Fatalf("profile tags after delete = %v, want empty", got.Tags)
	}
}

// TestBrowserDeleteTag_EmptyName 空标签返回错误。
func TestBrowserDeleteTag_EmptyName(t *testing.T) {
	app, _ := newTagTestApp(t)
	if err := app.BrowserDeleteTag("   "); err == nil {
		t.Fatal("expected error for blank tag name, got nil")
	}
}
