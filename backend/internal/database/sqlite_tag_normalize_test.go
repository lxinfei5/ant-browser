package database

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestNormalizeStoredTags 三处历史脏标签(实例/账号/注册表)在迁移后被归一为小写去重,
// browser_tags 主键重建为 COLLATE NOCASE,且二次执行为幂等零变化。
func TestNormalizeStoredTags(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "norm.db")
	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	defer db.Close()

	// 最小化 schema:三张相关表 + 已记录到 v18(让 v19 重建 browser_tags)
	setup := []string{
		`CREATE TABLE browser_profiles (
			profile_id TEXT PRIMARY KEY, profile_name TEXT NOT NULL,
			tags TEXT NOT NULL DEFAULT '[]', deleted_at TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL
		)`,
		`CREATE TABLE accounts (
			account_id TEXT PRIMARY KEY, account_name TEXT NOT NULL,
			tags TEXT NOT NULL DEFAULT '[]', deleted_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE browser_tags (tag_name TEXT PRIMARY KEY, created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP)`,
		`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, desc TEXT NOT NULL DEFAULT '', applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP)`,
		// 记录到 18,使 v19 在本 Migrate 内执行
		`INSERT INTO schema_migrations (version, desc) VALUES (18, 'browser_tags')`,
		// 脏数据:大小写/空白变体
		`INSERT INTO browser_profiles (profile_id, profile_name, tags, created_at, updated_at) VALUES
			('p1', 'a', '[" OpenCode ","opencode","WORK"]', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'),
			('p2', 'b', '["vip"]', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`,
		`INSERT INTO accounts (account_id, account_name, tags) VALUES
			('a1', 'x', '[" OpenCode ","GROK"]')`,
		`INSERT INTO browser_tags (tag_name) VALUES ('OpenCode'), (' opencode '), ('VIP')`,
	}
	for _, stmt := range setup {
		if _, err := db.conn.Exec(stmt); err != nil {
			t.Fatalf("setup failed: %v", err)
		}
	}

	if err := db.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	// ① 实例标签小写去重
	assertTagsColumn(t, db, "browser_profiles", "profile_id", "p1", `["opencode","work"]`)
	assertTagsColumn(t, db, "browser_profiles", "profile_id", "p2", `["vip"]`)
	// ② 账号标签小写去重
	assertTagsColumn(t, db, "accounts", "account_id", "a1", `["opencode","grok"]`)
	// ③ 注册表去重为小写,且只剩 opencode/vip 两行
	var registryCount int
	if err := db.conn.QueryRow(`SELECT COUNT(*) FROM browser_tags`).Scan(&registryCount); err != nil {
		t.Fatalf("count browser_tags: %v", err)
	}
	if registryCount != 2 {
		t.Fatalf("browser_tags count = %d, want 2 (opencode, vip)", registryCount)
	}
	// ④ 主键已重建为 COLLATE NOCASE
	var tableSQL string
	if err := db.conn.QueryRow(`SELECT sql FROM sqlite_master WHERE name='browser_tags'`).Scan(&tableSQL); err != nil {
		t.Fatalf("read browser_tags schema: %v", err)
	}
	if !strings.Contains(strings.ToUpper(tableSQL), "COLLATE NOCASE") {
		t.Fatalf("browser_tags schema missing COLLATE NOCASE: %s", tableSQL)
	}
	// ⑤ NOCASE 生效:大小写变体 INSERT 被忽略
	if _, err := db.conn.Exec(`INSERT OR IGNORE INTO browser_tags (tag_name) VALUES ('OPENCODE')`); err != nil {
		t.Fatalf("insert variant: %v", err)
	}
	if err := db.conn.QueryRow(`SELECT COUNT(*) FROM browser_tags`).Scan(&registryCount); err != nil {
		t.Fatalf("recount browser_tags: %v", err)
	}
	if registryCount != 2 {
		t.Fatalf("after variant insert, browser_tags count = %d, want 2", registryCount)
	}

	// ⑥ 幂等:再次矫正无变化(行数不变、内容不变)
	if err := db.NormalizeStoredTags(); err != nil {
		t.Fatalf("second NormalizeStoredTags: %v", err)
	}
	assertTagsColumn(t, db, "browser_profiles", "profile_id", "p1", `["opencode","work"]`)
}

func assertTagsColumn(t *testing.T, db *DB, table, idCol, idVal, want string) {
	t.Helper()
	var got string
	if err := db.conn.QueryRow(`SELECT tags FROM `+table+` WHERE `+idCol+` = ?`, idVal).Scan(&got); err != nil {
		t.Fatalf("read %s.%s=%s: %v", table, idCol, idVal, err)
	}
	if got != want {
		t.Fatalf("%s tags for %s = %s, want %s", table, idVal, got, want)
	}
}
