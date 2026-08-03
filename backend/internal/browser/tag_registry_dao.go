package browser

import (
	"database/sql"
	"fmt"
	"strings"
)

// TagRegistryDAO 标签注册表持久化接口。
// 注册表只记录“用户创建过/认可的标签名”，与挂在实例上的标签是两套数据：
//   - 注册表：标签管理页「新建标签」持久化到这里，用于下拉建议与筛选可选项；
//   - 实例标签（browser_profiles.tags）：真正打在实例上的标签。
type TagRegistryDAO interface {
	// Ensure 幂等注册一个标签（已存在时忽略）
	Ensure(tagName string) error
	// Delete 从注册表删除一个标签（不影响已挂在实例上的标签）
	Delete(tagName string) error
	// List 返回注册表中的全部标签（按名称升序）
	List() ([]string, error)
}

// SQLiteTagRegistryDAO 基于 SQLite 的 TagRegistryDAO 实现
type SQLiteTagRegistryDAO struct {
	db *sql.DB
}

// NewSQLiteTagRegistryDAO 创建 SQLiteTagRegistryDAO
func NewSQLiteTagRegistryDAO(db *sql.DB) *SQLiteTagRegistryDAO {
	return &SQLiteTagRegistryDAO{db: db}
}

// Ensure 幂等注册标签。DAO 层自卫:入库前 trim + 转小写,保证注册表不混入大小写/空白变体
// (配合 browser_tags.tag_name 的 COLLATE NOCASE 主键双保险)。空标签直接忽略。
func (d *SQLiteTagRegistryDAO) Ensure(tagName string) error {
	tagName = strings.ToLower(strings.TrimSpace(tagName))
	if tagName == "" {
		return nil
	}
	_, err := d.db.Exec(`INSERT OR IGNORE INTO browser_tags (tag_name) VALUES (?)`, tagName)
	if err != nil {
		return fmt.Errorf("保存标签失败: %w", err)
	}
	return nil
}

// Delete 删除注册表标签。同样先 trim + 转小写,与 Ensure 口径一致(并有 NOCASE 主键兜底)。
func (d *SQLiteTagRegistryDAO) Delete(tagName string) error {
	tagName = strings.ToLower(strings.TrimSpace(tagName))
	if tagName == "" {
		return nil
	}
	_, err := d.db.Exec(`DELETE FROM browser_tags WHERE tag_name = ?`, tagName)
	if err != nil {
		return fmt.Errorf("删除标签失败: %w", err)
	}
	return nil
}

// List 返回全部注册表标签（按名称升序）
func (d *SQLiteTagRegistryDAO) List() ([]string, error) {
	rows, err := d.db.Query(`SELECT tag_name FROM browser_tags ORDER BY tag_name ASC`)
	if err != nil {
		return nil, fmt.Errorf("查询标签列表失败: %w", err)
	}
	defer rows.Close()

	var tags []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		if t = strings.TrimSpace(t); t != "" {
			tags = append(tags, t)
		}
	}
	return tags, rows.Err()
}
