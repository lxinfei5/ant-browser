package database

import (
	"encoding/json"
	"fmt"
	"strings"

	"ant-chrome/backend/internal/tagutil"
)

// BackfillPlatformTags 幂等把 accounts.platform ∈ {xhs, x} 归一入 accounts.tags（服务即标签）。
//
// 背景：platform 独立列在 v21 被物理删除。删除前必须把仍有信息量的 platform 值折叠进 tags，
// 否则这部分平台归属信息将随列一同丢失。本步骤在 Migrate 内、v21 事务之前执行；
// 同时在 Migrate 末尾再幂等重跑一次作为兜底。
//
// 设计要点：
//   - 仅在 platform 列仍存在时工作；列不存在（已是 v21 结构）时静默 no-op，保证可每次启动重跑。
//   - 仅处理未删除账号（deleted_at == ''），软删账号不打扰。
//   - 复用 tagutil.NormalizeAll 做归一并集（platform 先 Normalize，再与既有 tags 求并），
//     仅在有变化时落库，收敛后零写入。
func (db *DB) BackfillPlatformTags() error {
	if !db.tableExists("accounts") {
		return nil
	}
	if !db.columnExists("accounts", "platform") {
		return nil
	}

	rows, err := db.conn.Query(`SELECT account_id, platform, tags FROM accounts WHERE COALESCE(deleted_at, '') = ''`)
	if err != nil {
		return fmt.Errorf("查询账号 platform/tags 失败: %w", err)
	}
	defer rows.Close()

	type pending struct {
		id   string
		tags string
	}
	var updates []pending
	for rows.Next() {
		var id, platform, raw string
		if err := rows.Scan(&id, &platform, &raw); err != nil {
			return err
		}
		p := tagutil.Normalize(platform)
		if p != "xhs" && p != "x" {
			continue // 只折叠 xhs/x；其它（含空）无平台语义，跳过
		}
		var tags []string
		if err := json.Unmarshal([]byte(raw), &tags); err != nil {
			tags = nil // 非法 JSON 视为空，随后以 platform 重建，避免丢平台
		}
		if tagutil.ContainsFold(tags, p) {
			continue // 已在 tags 中，幂等跳过
		}
		merged := tagutil.NormalizeAll(append(append([]string{}, tags...), p))
		encoded, err := json.Marshal(merged)
		if err != nil {
			continue
		}
		updates = append(updates, pending{id: id, tags: string(encoded)})
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, u := range updates {
		if _, err := db.conn.Exec(`UPDATE accounts SET tags = ? WHERE account_id = ?`, u.tags, u.id); err != nil {
			return fmt.Errorf("回填 platform 到 tags 失败(%s): %w", u.id, err)
		}
	}
	return nil
}

// columnExists 判断表是否存在某列（PRAGMA table_info）。table 为内部常量，不存在注入面。
func (db *DB) columnExists(table, column string) bool {
	rows, err := db.conn.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return false
	}
	defer rows.Close()
	for rows.Next() {
		var (
			cid     int
			name    string
			colType string
			notNull int
			dflt    any
			pk      int
		)
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			return false
		}
		if strings.EqualFold(name, column) {
			return true
		}
	}
	return false
}
