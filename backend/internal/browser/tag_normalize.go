package browser

import (
	"ant-chrome/backend/internal/logger"
	"ant-chrome/backend/internal/tagutil"
)

// NormalizeTags 实例标签归一(trim + 小写 + 去重),委托 tagutil,供 browser 包与 App 层复用。
// 所有写入 browser_profiles.tags 的路径都应先经过它,保证全库同一标签只有小写一种写法。
func NormalizeTags(tags []string) []string {
	return tagutil.NormalizeAll(tags)
}

// NormalizeTag 单标签归一(trim + 小写)。
func NormalizeTag(tag string) string {
	return tagutil.Normalize(tag)
}

// ensureRegisteredTags 把标签同步进标签注册表(browser_tags),让「新建标签」之外的写入路径
// (实例创建/更新、批量打标)也能让注册表始终保持为全量标签的超集。
// TagRegistry 为 nil 或单个 Ensure 失败都不阻断主流程,仅记录日志。
func (m *Manager) ensureRegisteredTags(tags []string) {
	if m.TagRegistry == nil {
		return
	}
	log := logger.New("Browser")
	for _, t := range tags {
		if err := m.TagRegistry.Ensure(t); err != nil {
			log.Warn("同步标签到注册表失败", logger.F("tag", t), logger.F("error", err))
		}
	}
}
