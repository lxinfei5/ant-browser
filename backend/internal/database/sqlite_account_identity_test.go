package database

import (
	"path/filepath"
	"strings"
	"testing"
)

// ── 测试辅助 ────────────────────────────────────────────────────────────────

// setupAccountMigrationBase 建立 v20/v21 之前的最小公共 schema：
// browser_profiles / browser_tags（Migrate 末尾的关键列自愈与标签归一需要）+ schema_migrations。
func setupAccountMigrationBase(t *testing.T, db *DB, recordedVersion int) {
	t.Helper()
	stmts := []string{
		`CREATE TABLE browser_profiles (
			profile_id TEXT PRIMARY KEY, profile_name TEXT NOT NULL,
			tags TEXT NOT NULL DEFAULT '[]', deleted_at TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL
		)`,
		`CREATE TABLE browser_tags (tag_name TEXT PRIMARY KEY, created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP)`,
		`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, desc TEXT NOT NULL DEFAULT '', applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP)`,
	}
	for _, stmt := range stmts {
		if _, err := db.conn.Exec(stmt); err != nil {
			t.Fatalf("setup base failed: %v", err)
		}
	}
	if _, err := db.conn.Exec(`INSERT INTO schema_migrations (version, desc) VALUES (?, 'base')`, recordedVersion); err != nil {
		t.Fatalf("record version failed: %v", err)
	}
}

// createAccountsV13Shape 建 v13 形态的 accounts（含 platform、无 email/phone）。
func createAccountsV13Shape(t *testing.T, db *DB) {
	t.Helper()
	if _, err := db.conn.Exec(`CREATE TABLE accounts (
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
		t.Fatalf("create accounts v13 failed: %v", err)
	}
}

// createAccountsV20Shape 建 v20 形态的 accounts（含 platform + email/phone）。
func createAccountsV20Shape(t *testing.T, db *DB) {
	t.Helper()
	stmts := []string{
		`CREATE TABLE accounts (
			account_id TEXT PRIMARY KEY, account_name TEXT NOT NULL,
			platform TEXT NOT NULL DEFAULT '', account_ref TEXT NOT NULL DEFAULT '',
			email TEXT NOT NULL DEFAULT '', phone TEXT NOT NULL DEFAULT '',
			bound_profile_id TEXT NOT NULL DEFAULT '', proxy_id TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'active', cooldown_until TEXT NOT NULL DEFAULT '',
			notes TEXT NOT NULL DEFAULT '', tags TEXT NOT NULL DEFAULT '[]', group_id TEXT NOT NULL DEFAULT '',
			credential_json TEXT NOT NULL DEFAULT '{}', metadata_json TEXT NOT NULL DEFAULT '{}',
			last_used_at TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			deleted_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_accounts_email ON accounts(LOWER(email)) WHERE email<>'' AND COALESCE(deleted_at,'')=''`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_accounts_phone ON accounts(LOWER(phone)) WHERE phone<>'' AND COALESCE(deleted_at,'')=''`,
	}
	for _, stmt := range stmts {
		if _, err := db.conn.Exec(stmt); err != nil {
			t.Fatalf("create accounts v20 failed: %v", err)
		}
	}
}

// createAccountLeases 建 v14 形态的 account_leases（验证 v21 将其删除）。
func createAccountLeases(t *testing.T, db *DB) {
	t.Helper()
	if _, err := db.conn.Exec(`CREATE TABLE account_leases (
		lease_id TEXT PRIMARY KEY, account_id TEXT NOT NULL, profile_id TEXT NOT NULL,
		worker_id TEXT NOT NULL DEFAULT '', purpose TEXT NOT NULL DEFAULT 'scrape',
		status TEXT NOT NULL DEFAULT 'held', cdp_endpoint TEXT NOT NULL DEFAULT '',
		leased_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, expires_at TEXT NOT NULL DEFAULT '',
		heartbeat_at TEXT NOT NULL DEFAULT '', released_at TEXT NOT NULL DEFAULT '',
		release_result TEXT NOT NULL DEFAULT '', auto_started INTEGER NOT NULL DEFAULT 0,
		metadata_json TEXT NOT NULL DEFAULT '{}',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		t.Fatalf("create account_leases failed: %v", err)
	}
}

func indexExists(t *testing.T, db *DB, name string) bool {
	t.Helper()
	var cnt int
	if err := db.conn.QueryRow(`SELECT COUNT(1) FROM sqlite_master WHERE type='index' AND name=?`, name).Scan(&cnt); err != nil {
		t.Fatalf("indexExists %s: %v", name, err)
	}
	return cnt > 0
}

func accountTags(t *testing.T, db *DB, id string) string {
	t.Helper()
	var tags string
	if err := db.conn.QueryRow(`SELECT tags FROM accounts WHERE account_id=?`, id).Scan(&tags); err != nil {
		t.Fatalf("read tags for %s: %v", id, err)
	}
	return tags
}

func accountEmailPhone(t *testing.T, db *DB, id string) (string, string) {
	t.Helper()
	var email, phone string
	if err := db.conn.QueryRow(`SELECT email, phone FROM accounts WHERE account_id=?`, id).Scan(&email, &phone); err != nil {
		t.Fatalf("read email/phone for %s: %v", id, err)
	}
	return email, phone
}

// ── 迁移测试 ────────────────────────────────────────────────────────────────

// TestMigrate_V20_V21_FromV13 从 v13 形态(有 platform、无 email/phone)升到 v21：
// v20 加 email/phone + 索引；v21 删 platform 与 account_leases；xhs/x 折叠进 tags。
func TestMigrate_V20_V21_FromV13(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "migrate.db")
	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	defer db.Close()

	setupAccountMigrationBase(t, db, 19) // 记录到 v19，使 v20/v21 在本次 Migrate 内执行
	createAccountsV13Shape(t, db)
	createAccountLeases(t, db)

	// 预置数据：xhs / x / other / 空 / 软删 五类
	seed := []string{
		`INSERT INTO accounts (account_id, account_name, platform, tags) VALUES
			('a-xhs', 'A', 'xhs', '["vip"]'),
			('a-x', 'B', 'x', '[]'),
			('a-other', 'C', 'other', '["keep"]'),
			('a-empty', 'D', '', '[]')`,
		`INSERT INTO accounts (account_id, account_name, platform, tags, deleted_at) VALUES
			('a-del', 'E', 'xhs', '["old"]', '2026-01-01T00:00:00Z')`,
		`INSERT INTO account_leases (lease_id, account_id, profile_id) VALUES ('l1', 'a-xhs', 'p1')`,
	}
	for _, stmt := range seed {
		if _, err := db.conn.Exec(stmt); err != nil {
			t.Fatalf("seed failed: %v", err)
		}
	}

	if err := db.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	// v20：email/phone 列已加；两条 LOWER 部分唯一索引已建
	if !db.columnExists("accounts", "email") || !db.columnExists("accounts", "phone") {
		t.Fatalf("v20 应新增 email/phone 列")
	}
	if !indexExists(t, db, "idx_accounts_email") || !indexExists(t, db, "idx_accounts_phone") {
		t.Fatalf("v20 应建立 email/phone 唯一索引")
	}

	// v21：platform 列与 account_leases 表已删；idx_accounts_platform 消失
	if db.columnExists("accounts", "platform") {
		t.Fatalf("v21 应删除 platform 列")
	}
	if db.tableExists("account_leases") {
		t.Fatalf("v21 应删除 account_leases 表")
	}
	if indexExists(t, db, "idx_accounts_platform") {
		t.Fatalf("v21 后 idx_accounts_platform 应随旧表删除")
	}
	// 重建后常规索引仍在
	for _, idx := range []string{"idx_accounts_status", "idx_accounts_bound_profile", "idx_accounts_deleted_at"} {
		if !indexExists(t, db, idx) {
			t.Fatalf("v21 应重建索引 %s", idx)
		}
	}

	// backfill：xhs/x 折叠进 tags（保留既有标签），other/空不折叠，软删行不打扰
	if got := accountTags(t, db, "a-xhs"); got != `["vip","xhs"]` {
		t.Fatalf("a-xhs tags = %s, want [vip xhs]", got)
	}
	if got := accountTags(t, db, "a-x"); got != `["x"]` {
		t.Fatalf("a-x tags = %s, want [x]", got)
	}
	if got := accountTags(t, db, "a-other"); got != `["keep"]` {
		t.Fatalf("a-other tags = %s, want [keep]（other 不折叠）", got)
	}
	if got := accountTags(t, db, "a-empty"); got != `[]` {
		t.Fatalf("a-empty tags = %s, want []", got)
	}
	if got := accountTags(t, db, "a-del"); got != `["old"]` {
		t.Fatalf("软删行 a-del tags = %s, want [old]（软删不回填）", got)
	}

	// 幂等：再次 Migrate（版本门控，全部跳过）与再次 Backfill（platform 已删，no-op）均不出错、数据不变
	if err := db.Migrate(); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
	if err := db.BackfillPlatformTags(); err != nil {
		t.Fatalf("post BackfillPlatformTags: %v", err)
	}
	if got := accountTags(t, db, "a-xhs"); got != `["vip","xhs"]` {
		t.Fatalf("二次运行后 a-xhs tags = %s, want [vip xhs]（应稳定）", got)
	}
}

// TestMigrate_V20_ToleratesExistingEmailPhone 模拟「冲突库」：accounts 已有 email/phone 列
// 但版本仍记录在 v20 之前。v20 的 ADD COLUMN 命中 isColumnExistsError 被忽略，升级应干净完成。
func TestMigrate_V20_ToleratesExistingEmailPhone(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "conflicted.db")
	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	defer db.Close()

	setupAccountMigrationBase(t, db, 19)
	// 冲突形态：email/phone 已存在（但版本未到 20），platform 仍在
	createAccountsV20Shape(t, db)
	if _, err := db.conn.Exec(`INSERT INTO accounts (account_id, account_name, platform, email, phone, tags)
		VALUES ('a1', 'A', 'xhs', 'u@example.com', '+8613800138000', '[]')`); err != nil {
		t.Fatalf("seed failed: %v", err)
	}

	if err := db.Migrate(); err != nil {
		t.Fatalf("Migrate on conflicted DB should succeed, got: %v", err)
	}
	if db.columnExists("accounts", "platform") {
		t.Fatalf("v21 应删除 platform 列")
	}
	email, phone := accountEmailPhone(t, db, "a1")
	if email != "u@example.com" || phone != "+8613800138000" {
		t.Fatalf("email/phone 应保留, got email=%q phone=%q", email, phone)
	}
	if got := accountTags(t, db, "a1"); got != `["xhs"]` {
		t.Fatalf("a1 tags = %s, want [xhs]", got)
	}
}

// TestMigrate_V21_PreservesEmailPhone 从 v20 形态（含 platform + 已填充 email/phone）升 v21：
// 物理删除 platform 的同时完整保留既有 email/phone/tags。
func TestMigrate_V21_PreservesEmailPhone(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "v21.db")
	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	defer db.Close()

	setupAccountMigrationBase(t, db, 20) // 记录到 v20，仅 v21 在本次执行
	createAccountsV20Shape(t, db)
	createAccountLeases(t, db)
	if _, err := db.conn.Exec(`INSERT INTO accounts (account_id, account_name, platform, email, phone, tags)
		VALUES
		('k1', 'K', 'x', 'keep@example.com', '13800138000', '["a","b"]'),
		('k2', 'L', '', '', '', '[]')`); err != nil {
		t.Fatalf("seed failed: %v", err)
	}

	if err := db.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	if db.columnExists("accounts", "platform") {
		t.Fatalf("v21 应删除 platform 列")
	}
	email, phone := accountEmailPhone(t, db, "k1")
	if email != "keep@example.com" || phone != "13800138000" {
		t.Fatalf("k1 email/phone 应保留, got email=%q phone=%q", email, phone)
	}
	// platform=x 折叠进既有 tags 尾部
	if got := accountTags(t, db, "k1"); got != `["a","b","x"]` {
		t.Fatalf("k1 tags = %s, want [a b x]", got)
	}
	// 空 platform 行的 email/phone 保持空
	email2, phone2 := accountEmailPhone(t, db, "k2")
	if email2 != "" || phone2 != "" {
		t.Fatalf("k2 email/phone 应为空, got email=%q phone=%q", email2, phone2)
	}
}

// TestBackfillPlatformTags_Standalone 直接对含 platform 的库调用 BackfillPlatformTags：
// 首次折叠、二次幂等 no-op、仅处理未删除行、非法 JSON tags 以 platform 重建。
func TestBackfillPlatformTags_Standalone(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "backfill.db")
	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	defer db.Close()

	// 仅需 accounts（含 platform），不走 Migrate
	createAccountsV13Shape(t, db)
	// b1 大写平台应归一为 xhs 并折叠；b2 tags 已含 x(大小写变体)幂等跳过；
	// b3 非法 JSON 以 platform 重建；b4 非 xhs/x 不动；b5 软删不动。
	seed := []string{
		`INSERT INTO accounts (account_id, account_name, platform, tags) VALUES
			('b1', 'A', 'XHS', '["vip"]'),
			('b2', 'B', 'x', '["X"]'),
			('b3', 'C', 'xhs', 'not-json'),
			('b4', 'D', 'other', '["keep"]')`,
		`INSERT INTO accounts (account_id, account_name, platform, tags, deleted_at) VALUES
			('b5', 'E', 'xhs', '["old"]', '2026-01-01T00:00:00Z')`,
	}
	for _, stmt := range seed {
		if _, err := db.conn.Exec(stmt); err != nil {
			t.Fatalf("seed failed: %v", err)
		}
	}

	if err := db.BackfillPlatformTags(); err != nil {
		t.Fatalf("BackfillPlatformTags: %v", err)
	}
	if got := accountTags(t, db, "b1"); got != `["vip","xhs"]` {
		t.Fatalf("b1 tags = %s, want [vip xhs]", got)
	}
	if got := accountTags(t, db, "b2"); got != `["X"]` {
		t.Fatalf("b2 tags = %s, want [X]（已含 x，幂等不改写）", got)
	}
	if got := accountTags(t, db, "b3"); got != `["xhs"]` {
		t.Fatalf("b3 tags = %s, want [xhs]（非法 JSON 重建）", got)
	}
	if got := accountTags(t, db, "b4"); got != `["keep"]` {
		t.Fatalf("b4 tags = %s, want [keep]", got)
	}
	if got := accountTags(t, db, "b5"); got != `["old"]` {
		t.Fatalf("b5(软删) tags = %s, want [old]", got)
	}

	// 二次执行：幂等，无变化
	if err := db.BackfillPlatformTags(); err != nil {
		t.Fatalf("second BackfillPlatformTags: %v", err)
	}
	if got := accountTags(t, db, "b1"); got != `["vip","xhs"]` {
		t.Fatalf("二次后 b1 tags = %s, want [vip xhs]", got)
	}
}

// TestMigrate_V21_UniqueIndexEnforcedAfterRebuild 重建后 email 部分唯一索引仍然生效：
// 两个未删除账号同 email（大小写不同）应冲突。
func TestMigrate_V21_UniqueIndexEnforcedAfterRebuild(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "uniq.db")
	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	defer db.Close()

	setupAccountMigrationBase(t, db, 19)
	createAccountsV13Shape(t, db)
	if err := db.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	if _, err := db.conn.Exec(`INSERT INTO accounts (account_id, account_name, email) VALUES ('u1', 'A', 'dup@example.com')`); err != nil {
		t.Fatalf("insert u1: %v", err)
	}
	// 大小写不同但 LOWER 相同 -> 违反 LOWER() 唯一索引
	_, err = db.conn.Exec(`INSERT INTO accounts (account_id, account_name, email) VALUES ('u2', 'B', 'DUP@example.com')`)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "unique") {
		t.Fatalf("大小写变体同 email 应触发唯一索引冲突, got: %v", err)
	}
}

// TestMigrate_V21_PreservesAllColumns 全列哨兵：给 v20 行的 18 个拷贝列各填一个互不相同的哨兵值，
// 升级 v21 后逐列断言完全相等。v21 的 INSERT...SELECT 是手工维护的 18 列位置列表，
// 任何同类型列错位（如 credential_json<->metadata_json、created_at<->updated_at）都会在此暴露——
// 而既有测试只断言 email/phone/tags 三列，且其余列多为默认值，错位不可见。
// credential_json 承载 PII（cookie/密码），错位即静默错档，故必须全列锁定。
func TestMigrate_V21_PreservesAllColumns(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "v21full.db")
	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	defer db.Close()

	setupAccountMigrationBase(t, db, 20) // 仅 v21 在本次执行
	createAccountsV20Shape(t, db)

	// 18 个拷贝列（v21 目标列顺序；platform 不入新表）各赋唯一哨兵。
	sentinel := map[string]string{
		"account_ref":      "ref-XYZ",
		"email":            "Keep@Example.com",
		"phone":            "+8613900139000",
		"bound_profile_id": "prof-777",
		"proxy_id":         "proxy-888",
		"status":           "cooldown",
		"cooldown_until":   "2030-12-31T23:59:59Z",
		"notes":            "note-∆-中文",
		"tags":             `["t1","Xhs","t2"]`,
		"group_id":         "grp-555",
		"credential_json":  `{"cookie":"SECRET-CRED-1"}`,
		"metadata_json":    `{"k":"META-2"}`,
		"last_used_at":     "2029-01-02T03:04:05Z",
		"created_at":       "2020-01-01T00:00:01Z",
		"updated_at":       "2021-02-03T04:05:06Z",
		"deleted_at":       "",
	}
	if _, err := db.conn.Exec(`INSERT INTO accounts
		(account_id, account_name, account_ref, email, phone, bound_profile_id, proxy_id,
		 status, cooldown_until, notes, tags, group_id, credential_json, metadata_json,
		 last_used_at, created_at, updated_at, deleted_at)
		VALUES ('full1', 'FullRow', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sentinel["account_ref"], sentinel["email"], sentinel["phone"], sentinel["bound_profile_id"],
		sentinel["proxy_id"], sentinel["status"], sentinel["cooldown_until"], sentinel["notes"],
		sentinel["tags"], sentinel["group_id"], sentinel["credential_json"], sentinel["metadata_json"],
		sentinel["last_used_at"], sentinel["created_at"], sentinel["updated_at"], sentinel["deleted_at"],
	); err != nil {
		t.Fatalf("seed failed: %v", err)
	}

	if err := db.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if db.columnExists("accounts", "platform") {
		t.Fatalf("v21 应删除 platform 列")
	}

	// 逐列读回并断言。tags 例外：platform='' 不折叠，但启动末尾的 NormalizeStoredTags
	// 会把标签归一为 trim+小写+去重（["t1","Xhs","t2"] -> ["t1","xhs","t2"]），属预期归一。
	var (
		accountRef, email, phone, boundProfileID, proxyID string
		status, cooldownUntil, notes, tags, groupID        string
		credential, metadata                               string
		lastUsedAt, createdAt, updatedAt, deletedAt        string
	)
	if err := db.conn.QueryRow(`SELECT account_ref, email, phone, bound_profile_id, proxy_id,
		status, cooldown_until, notes, tags, group_id, credential_json, metadata_json,
		last_used_at, created_at, updated_at, deleted_at FROM accounts WHERE account_id='full1'`).
		Scan(&accountRef, &email, &phone, &boundProfileID, &proxyID, &status, &cooldownUntil,
			&notes, &tags, &groupID, &credential, &metadata, &lastUsedAt, &createdAt, &updatedAt, &deletedAt); err != nil {
		t.Fatalf("read back failed: %v", err)
	}

	wantTags := `["t1","xhs","t2"]` // 归一后（见上注释）
	checks := []struct{ col, got, want string }{
		{"account_ref", accountRef, sentinel["account_ref"]},
		{"email", email, sentinel["email"]},
		{"phone", phone, sentinel["phone"]},
		{"bound_profile_id", boundProfileID, sentinel["bound_profile_id"]},
		{"proxy_id", proxyID, sentinel["proxy_id"]},
		{"status", status, sentinel["status"]},
		{"cooldown_until", cooldownUntil, sentinel["cooldown_until"]},
		{"notes", notes, sentinel["notes"]},
		{"tags", tags, wantTags},
		{"group_id", groupID, sentinel["group_id"]},
		{"credential_json", credential, sentinel["credential_json"]},
		{"metadata_json", metadata, sentinel["metadata_json"]},
		{"last_used_at", lastUsedAt, sentinel["last_used_at"]},
		{"created_at", createdAt, sentinel["created_at"]},
		{"updated_at", updatedAt, sentinel["updated_at"]},
		{"deleted_at", deletedAt, sentinel["deleted_at"]},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Fatalf("列 %s 错位/丢失: got %q, want %q", c.col, c.got, c.want)
		}
	}
}
