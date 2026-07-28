package database

import (
	"path/filepath"
	"testing"
)

// TestMigrateHealsMissingRestoreLastSession 模拟历史分支占用 version=15 后，
// MAX(version) 已到 16 但 restore_last_session 仍缺失的库：迁移应自愈补齐该列。
func TestMigrateHealsMissingRestoreLastSession(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "heal.db")
	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	defer db.Close()

	// 最小化历史 schema：有 browser_profiles，但无 restore_last_session；
	// schema_migrations 已记录到 16（模拟 dock-icon 占用 v15 + memory_limit 为 v16）。
	setup := []string{
		`CREATE TABLE browser_profiles (
			profile_id TEXT PRIMARY KEY,
			profile_name TEXT NOT NULL,
			user_data_dir TEXT NOT NULL DEFAULT '',
			core_id TEXT NOT NULL DEFAULT '',
			fingerprint_args TEXT NOT NULL DEFAULT '[]',
			proxy_id TEXT NOT NULL DEFAULT '',
			proxy_config TEXT NOT NULL DEFAULT '',
			launch_args TEXT NOT NULL DEFAULT '[]',
			tags TEXT NOT NULL DEFAULT '[]',
			keywords TEXT NOT NULL DEFAULT '[]',
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			deleted_at TEXT NOT NULL DEFAULT '',
			memory_limit_mb INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE schema_migrations (
			version INTEGER PRIMARY KEY,
			desc TEXT NOT NULL DEFAULT '',
			applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`INSERT INTO schema_migrations (version, desc) VALUES
			(15, '为 browser_profiles 与 accounts 增加 Dock 图标定制列'),
			(16, '实例表添加内存限制字段（上游 v14）')`,
		`INSERT INTO browser_profiles (profile_id, profile_name, created_at, updated_at)
			VALUES ('p1', 'demo', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`,
	}
	for _, stmt := range setup {
		if _, err := db.conn.Exec(stmt); err != nil {
			t.Fatalf("setup failed [%s]: %v", stmt[:min(40, len(stmt))], err)
		}
	}

	// 迁移前：关键列缺失，模拟 ProfileDAO.List 会失败
	if columnExists(t, db, "browser_profiles", "restore_last_session") {
		t.Fatal("precondition: restore_last_session should be missing")
	}

	if err := db.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	if !columnExists(t, db, "browser_profiles", "restore_last_session") {
		t.Fatal("expected restore_last_session to be healed after Migrate")
	}

	// ProfileDAO 风格查询应能成功
	var id, restore, deleted string
	err = db.conn.QueryRow(`
		SELECT profile_id, COALESCE(restore_last_session, ''), COALESCE(deleted_at, '')
		FROM browser_profiles WHERE COALESCE(deleted_at, '') = ''`).Scan(&id, &restore, &deleted)
	if err != nil {
		t.Fatalf("profile list-style query failed after heal: %v", err)
	}
	if id != "p1" {
		t.Fatalf("unexpected profile_id: %s", id)
	}
}

func columnExists(t *testing.T, db *DB, table, column string) bool {
	t.Helper()
	rows, err := db.conn.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		t.Fatalf("PRAGMA table_info: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt any
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if name == column {
			return true
		}
	}
	return false
}
