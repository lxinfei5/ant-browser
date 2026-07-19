package browser

import (
	"ant-chrome/backend/internal/logger"
	"fmt"
	"path/filepath"
	"strings"
)

// GetProxyConfigById 根据代理 ID 获取代理配置
func (m *Manager) GetProxyConfigById(proxyId string) (string, bool) {
	if proxy, ok := m.GetProxyByID(proxyId); ok {
		return strings.TrimSpace(proxy.ProxyConfig), true
	}
	return "", false
}

// ResolveUserDataDir 解析用户数据目录。
// 安全约束：只允许 (a) 相对名称/ID 解析到 UserDataRoot 之下，或 (b) 位于 UserDataRoot 之内的绝对路径。
// 拒绝绝对路径越界、以及任何 Clean 后逃出数据根目录的相对路径，并返回错误。
func (m *Manager) ResolveUserDataDir(profile *Profile) (string, error) {
	if profile == nil {
		return "", fmt.Errorf("profile 为空")
	}
	userDataDir := strings.TrimSpace(profile.UserDataDir)
	if userDataDir == "" {
		userDataDir = profile.ProfileId
	}
	root := strings.TrimSpace(m.Config.Browser.UserDataRoot)
	if root == "" {
		root = "data"
	}
	root = m.ResolveRelativePath(root)

	cleaned := filepath.Clean(userDataDir)
	if cleaned == "" || cleaned == "." {
		// 空目录等价于直接使用根目录本身。
		return root, nil
	}

	var resolved string
	if filepath.IsAbs(cleaned) {
		resolved = cleaned
	} else {
		resolved = filepath.Join(root, cleaned)
	}

	if err := ensureWithinUserDataRoot(root, resolved); err != nil {
		return "", fmt.Errorf("用户数据目录不被允许：%w（dir=%s）", err, userDataDir)
	}
	return resolved, nil
}

// ensureWithinUserDataRoot 校验 target 解析后位于 root 之内（含 root 自身）。
func ensureWithinUserDataRoot(root, target string) error {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("解析数据根目录失败: %w", err)
	}
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return fmt.Errorf("解析目标目录失败: %w", err)
	}
	rel, err := filepath.Rel(absRoot, absTarget)
	if err != nil {
		return fmt.Errorf("无法定位相对数据根目录: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return fmt.Errorf("路径越出数据根目录")
	}
	return nil
}

// MigrateConfig 迁移旧配置到新格式
func (m *Manager) MigrateConfig() bool {
	log := logger.New("Browser")

	// 如果存在 environments 但没有 cores，执行迁移
	if len(m.Config.Browser.Environments) > 0 && len(m.Config.Browser.Cores) == 0 {
		log.Info("检测到旧配置格式，开始迁移")

		for _, env := range m.Config.Browser.Environments {
			m.Config.Browser.Cores = append(m.Config.Browser.Cores, Core{
				CoreId:    env.CoreId,
				CoreName:  env.CoreName,
				CorePath:  env.CorePath,
				IsDefault: env.IsDefault,
			})
		}

		// 清空旧字段
		m.Config.Browser.Environments = nil
		m.Config.Browser.ChromeBinaryPath = ""
		m.Config.Browser.CoreRoot = ""
		m.Config.Browser.DefaultCoreId = ""
		m.Config.Browser.DefaultConnectorType = ""

		if err := m.Config.Save(m.ResolveRelativePath("config.yaml")); err != nil {
			log.Error("配置迁移保存失败", logger.F("error", err.Error()))
			return false
		}

		log.Info("配置迁移完成", logger.F("cores_count", len(m.Config.Browser.Cores)))
		return true
	}

	return false
}
