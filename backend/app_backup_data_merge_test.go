package backend

import (
	"path/filepath"
	"testing"

	"ant-chrome/backend/internal/database"
)

// newMergeTestApp 构造一个带 v21 实时库的 App，供备份合并测试直接调用 backupMergeDatabaseFromSource。
func newMergeTestApp(t *testing.T) (*App, *database.DB) {
	t.Helper()
	db, err := database.NewDB(filepath.Join(t.TempDir(), "live.db"))
	if err != nil {
		t.Fatalf("NewDB(live): %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(); err != nil {
		t.Fatalf("Migrate(live): %v", err)
	}
	return &App{db: db}, db
}

// createBackupSourceDB 在独立文件建一个「备份源」库（模拟旧版本导出的 app.db），
// 用 v13 形态（含 platform、无 email/phone）+ 可选 v21 形态，由调用者选择。
func createBackupSourceDB(t *testing.T, v13Shape bool) *database.DB {
	t.Helper()
	src, err := database.NewDB(filepath.Join(t.TempDir(), "src.db"))
	if err != nil {
		t.Fatalf("NewDB(src): %v", err)
	}
	t.Cleanup(func() { _ = src.Close() })
	if v13Shape {
		// v13 形态：有 platform，无 email/phone（模拟 v20 之前的备份）
		if _, err := src.GetConn().Exec(`CREATE TABLE accounts (
			account_id TEXT PRIMARY KEY, account_name TEXT NOT NULL,
			platform TEXT NOT NULL DEFAULT '', account_ref TEXT NOT NULL DEFAULT '',
			bound_profile_id TEXT NOT NULL DEFAULT '', proxy_id TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'active', cooldown_until TEXT NOT NULL DEFAULT '',
			notes TEXT NOT NULL DEFAULT '', tags TEXT NOT NULL DEFAULT '[]', group_id TEXT NOT NULL DEFAULT '',
			credential_json TEXT NOT NULL DEFAULT '{}', metadata_json TEXT NOT NULL DEFAULT '{}',
			last_used_at TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			deleted_at TEXT NOT NULL DEFAULT ''
		)`); err != nil {
			t.Fatalf("create src accounts(v13): %v", err)
		}
	} else {
		// v21 形态：无 platform，有 email/phone（模拟当前版本的备份）
		if _, err := src.GetConn().Exec(`CREATE TABLE accounts (
			account_id TEXT PRIMARY KEY, account_name TEXT NOT NULL,
			account_ref TEXT NOT NULL DEFAULT '', email TEXT NOT NULL DEFAULT '', phone TEXT NOT NULL DEFAULT '',
			bound_profile_id TEXT NOT NULL DEFAULT '', proxy_id TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'active', cooldown_until TEXT NOT NULL DEFAULT '',
			notes TEXT NOT NULL DEFAULT '', tags TEXT NOT NULL DEFAULT '[]', group_id TEXT NOT NULL DEFAULT '',
			credential_json TEXT NOT NULL DEFAULT '{}', metadata_json TEXT NOT NULL DEFAULT '{}',
			last_used_at TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			deleted_at TEXT NOT NULL DEFAULT ''
		)`); err != nil {
			t.Fatalf("create src accounts(v21): %v", err)
		}
	}
	return src
}

func srcDBPath(t *testing.T, src *database.DB) string {
	t.Helper()
	// database.DB 暴露 GetConn 但不暴露路径；测试通过 PRAGMA database_list 取回文件路径。
	var path string
	if err := src.GetConn().QueryRow(`SELECT file FROM pragma_database_list WHERE name='main'`).Scan(&path); err != nil {
		t.Fatalf("read src path: %v", err)
	}
	return path
}

func liveAccountCount(t *testing.T, db *database.DB) int {
	t.Helper()
	var n int
	if err := db.GetConn().QueryRow(`SELECT COUNT(1) FROM accounts WHERE COALESCE(deleted_at,'')=''`).Scan(&n); err != nil {
		t.Fatalf("count live accounts: %v", err)
	}
	return n
}

func liveAccountTags(t *testing.T, db *database.DB, id string) string {
	t.Helper()
	var tags string
	if err := db.GetConn().QueryRow(`SELECT tags FROM accounts WHERE account_id=?`, id).Scan(&tags); err != nil {
		t.Fatalf("read tags %s: %v", id, err)
	}
	return tags
}

// TestBackupMerge_EmailCollisionDoesNotAbortImport 锁定最高危修复：
// 合并恢复时，源备份里一个与本地不同 account_id 但 email 相同的账号，命中 idx_accounts_email
// 唯一索引。修复前：整条 INSERT 报错并回滚整个数据库导入（所有表同一事务），恢复整体失败。
// 修复后：OR IGNORE 逐行跳过该冲突账号，导入成功，冲突行计入 skipped，其余账号照常入库。
func TestBackupMerge_EmailCollisionDoesNotAbortImport(t *testing.T) {
	app, live := newMergeTestApp(t)

	// 本地已有一个 email=shared@x.com 的账号（account_id=local-1）
	if _, err := live.GetConn().Exec(`INSERT INTO accounts (account_id, account_name, email) VALUES ('local-1', 'Local', 'shared@x.com')`); err != nil {
		t.Fatalf("seed live: %v", err)
	}

	// 备份源：同 email（大小写变体）但不同 account_id + 一个不冲突的新账号
	src := createBackupSourceDB(t, false)
	if _, err := src.GetConn().Exec(`INSERT INTO accounts (account_id, account_name, email) VALUES
		('backup-1', 'BackupDup', 'SHARED@x.com'),
		('backup-2', 'BackupNew', 'new@x.com')`); err != nil {
		t.Fatalf("seed src: %v", err)
	}

	stats := &backupMergeStats{}
	if err := app.backupMergeDatabaseFromSource(srcDBPath(t, src), false, stats); err != nil {
		t.Fatalf("合并恢复不应因单个 email 冲突而整体失败, got: %v", err)
	}

	// 本地原有账号保留；冲突的 backup-1 被跳过；backup-2 正常导入
	if got := liveAccountCount(t, live); got != 2 {
		t.Fatalf("合并后应有 2 个未删除账号(local-1 + backup-2), got %d", got)
	}
	var cnt int
	if err := live.GetConn().QueryRow(`SELECT COUNT(1) FROM accounts WHERE account_id='backup-1'`).Scan(&cnt); err != nil {
		t.Fatalf("query backup-1: %v", err)
	}
	if cnt != 0 {
		t.Fatalf("冲突的 backup-1 应被跳过(不导入)")
	}
	if err := live.GetConn().QueryRow(`SELECT COUNT(1) FROM accounts WHERE account_id='backup-2'`).Scan(&cnt); err != nil {
		t.Fatalf("query backup-2: %v", err)
	}
	if cnt != 1 {
		t.Fatalf("不冲突的 backup-2 应正常导入")
	}
	// 冲突行计入 skipped（而非无声消失）
	if stats.Skipped < 1 {
		t.Fatalf("冲突行应计入 skipped, got stats=%+v", stats)
	}
}

// TestBackupMerge_PreV21BackupFoldsPlatformIntoTags 锁定旧备份平台折叠修复：
// v21 之前的备份(有 platform、无 email/phone)恢复时，platform∈{xhs,x} 必须折进 tags，
// 否则平台归属会随目标库无 platform 列而永久丢失（就地升级有 BackfillPlatformTags，恢复路径此前没有）。
func TestBackupMerge_PreV21BackupFoldsPlatformIntoTags(t *testing.T) {
	app, live := newMergeTestApp(t)

	// v13 形态备份源：两条带 platform 的账号
	src := createBackupSourceDB(t, true)
	if _, err := src.GetConn().Exec(`INSERT INTO accounts (account_id, account_name, platform, tags) VALUES
		('old-1', 'OldXhs', 'xhs', '["vip"]'),
		('old-2', 'OldX', 'x', '[]'),
		('old-3', 'OldOther', 'other', '["keep"]')`); err != nil {
		t.Fatalf("seed src: %v", err)
	}

	stats := &backupMergeStats{}
	if err := app.backupMergeDatabaseFromSource(srcDBPath(t, src), false, stats); err != nil {
		t.Fatalf("旧备份合并恢复失败: %v", err)
	}

	// platform 折叠进 tags（保留既有标签），other 不折叠
	if got := liveAccountTags(t, live, "old-1"); got != `["vip","xhs"]` {
		t.Fatalf("old-1 tags = %s, want [vip xhs]（platform 应折叠）", got)
	}
	if got := liveAccountTags(t, live, "old-2"); got != `["x"]` {
		t.Fatalf("old-2 tags = %s, want [x]（platform 应折叠）", got)
	}
	if got := liveAccountTags(t, live, "old-3"); got != `["keep"]` {
		t.Fatalf("old-3 tags = %s, want [keep]（other 不折叠）", got)
	}
}
