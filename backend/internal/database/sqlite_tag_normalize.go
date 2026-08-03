package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"ant-chrome/backend/internal/tagutil"
)

// NormalizeStoredTags 幂等矫正三处历史标签为 trim + 小写 + 去重(统一走 tagutil 口径):
//   - browser_profiles.tags(JSON 数组,含回收站 deleted_at != '' 的实例,防恢复后复活)
//   - accounts.tags(JSON 数组)
//   - browser_tags(注册表,整表去重重插;NOCASE 主键见 v19 迁移)
//
// 每次启动在 Migrate 末尾执行:首次跑会清洗存量,收敛后零写入。仅在内容发生变化时落库。
func (db *DB) NormalizeStoredTags() error {
	if err := db.normalizeJSONTagsColumn("browser_profiles", "profile_id"); err != nil {
		return fmt.Errorf("矫正实例标签失败: %w", err)
	}
	if err := db.normalizeJSONTagsColumn("accounts", "account_id"); err != nil {
		return fmt.Errorf("矫正账号标签失败: %w", err)
	}
	if err := db.normalizeTagRegistry(); err != nil {
		return fmt.Errorf("矫正标签注册表失败: %w", err)
	}
	return nil
}

// normalizeJSONTagsColumn 矫正某张表中 JSON 数组形式的 tags 列。表不存在时静默跳过(老库容错)。
func (db *DB) normalizeJSONTagsColumn(table, idColumn string) error {
	if !db.tableExists(table) {
		return nil
	}
	// 仅处理非空、非 '[]' 的行,减少扫描与写放大
	rows, err := db.conn.Query(fmt.Sprintf(
		`SELECT %s, tags FROM %s WHERE tags IS NOT NULL AND TRIM(tags) <> '' AND TRIM(tags) <> '[]'`, idColumn, table))
	if err != nil {
		return err
	}
	defer rows.Close()

	type pending struct {
		id   string
		tags string
	}
	var updates []pending
	for rows.Next() {
		var id, raw string
		if err := rows.Scan(&id, &raw); err != nil {
			return err
		}
		var tags []string
		if err := json.Unmarshal([]byte(raw), &tags); err != nil {
			continue // 非法 JSON 不动,避免破坏数据
		}
		normalized := tagutil.NormalizeAll(tags)
		// 与现状比较:仅当归一后有变化才更新
		if !jsonTagsEqual(tags, normalized) {
			encoded, err := json.Marshal(normalized)
			if err != nil {
				continue
			}
			updates = append(updates, pending{id: id, tags: string(encoded)})
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, u := range updates {
		if _, err := db.conn.Exec(fmt.Sprintf(`UPDATE %s SET tags = ? WHERE %s = ?`, table, idColumn), u.tags, u.id); err != nil {
			return err
		}
	}
	return nil
}

// normalizeTagRegistry 矫正注册表:读出全部标签 → Go 侧归一去重 → 与现状不一致则整表重插。
// NOCASE 主键(v19)保证重插时 ASCII 大小写变体不再冲突。
func (db *DB) normalizeTagRegistry() error {
	if !db.tableExists("browser_tags") {
		return nil
	}
	rows, err := db.conn.Query(`SELECT tag_name, created_at FROM browser_tags`)
	if err != nil {
		return err
	}
	type entry struct {
		name      string
		createdAt string
	}
	var existing []entry
	for rows.Next() {
		var e entry
		if err := rows.Scan(&e.name, &e.createdAt); err != nil {
			rows.Close()
			return err
		}
		existing = append(existing, e)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	// Go 侧归一去重(保留每个归一值最早出现的 created_at)
	seen := make(map[string]string)
	order := make([]string, 0, len(existing))
	changed := false
	for _, e := range existing {
		n := tagutil.Normalize(e.name)
		if n == "" {
			changed = true // 空/全空白行应被清除
			continue
		}
		if n != e.name {
			changed = true // casing/空白需要改写
		}
		if _, ok := seen[n]; !ok {
			seen[n] = e.createdAt
			order = append(order, n)
		} else {
			changed = true // 重复(归一后)行需去除
		}
	}
	if !changed {
		return nil
	}

	tx, err := db.conn.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM browser_tags`); err != nil {
		tx.Rollback()
		return err
	}
	for _, name := range order {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO browser_tags (tag_name, created_at) VALUES (?, ?)`, name, seen[name]); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// tableExists 判断表是否存在。
func (db *DB) tableExists(table string) bool {
	var name string
	err := db.conn.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name = ?`, table).Scan(&name)
	if err == sql.ErrNoRows {
		return false
	}
	return err == nil
}

// jsonTagsEqual 比较原始标签列表与归一后列表是否完全一致(长度、顺序、取值)。
func jsonTagsEqual(original, normalized []string) bool {
	if len(original) != len(normalized) {
		return false
	}
	for i := range original {
		if original[i] != normalized[i] {
			return false
		}
	}
	return true
}

// backupDatabaseFile 把当前 DB 文件复制一份到 suffix 后缀(已存在则跳过)。
// 供 NormalizeStoredTags 首次执行前调用,作为不可逆小写改写的物理兜底。
func backupDatabaseFile(dbPath, suffix string) error {
	target := dbPath + suffix
	if _, err := os.Stat(target); err == nil {
		return nil // 已备份过
	}
	src, err := os.Open(dbPath)
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := os.Create(target)
	if err != nil {
		return err
	}
	defer dst.Close()
	if _, err := io.Copy(dst, src); err != nil {
		return err
	}
	return dst.Sync()
}
