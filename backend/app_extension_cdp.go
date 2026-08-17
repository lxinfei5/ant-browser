package backend

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"ant-chrome/backend/internal/logger"
)

func loadUnpackedExtensionsViaCDP(debugPort int, dirs []string) error {
	dirs = normalizeNonEmptyStrings(dirs)
	if debugPort <= 0 || len(dirs) == 0 {
		return nil
	}

	var failed []string
	for _, dir := range dirs {
		_, err := cdpBrowserCallResult(debugPort, "Extensions.loadUnpacked", map[string]any{
			"path": dir,
		})
		if err == nil || isBenignExtensionLoadError(err) {
			continue
		}
		failed = append(failed, fmt.Sprintf("%s: %v", dir, err))
	}
	if len(failed) == 0 {
		return nil
	}
	return fmt.Errorf("%s", strings.Join(failed, "; "))
}

func isBenignExtensionLoadError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "already") ||
		strings.Contains(msg, "duplicate") ||
		strings.Contains(msg, "already loaded") ||
		strings.Contains(msg, "already installed")
}

func applyLoadedExtensionsViaCDP(debugPort int, dirs []string, profile *BrowserProfile, profileID string) {
	err := loadUnpackedExtensionsViaCDP(debugPort, dirs)
	if err == nil {
		return
	}
	warning := "浏览器已启动，但插件未能通过调试口加载：" + err.Error()
	if profile != nil {
		profile.RuntimeWarning = warning
		profile.LastError = ""
	}
	logger.New("Browser").Warn("插件调试口加载失败",
		logger.F("profile_id", profileID),
		logger.F("debug_port", debugPort),
		logger.F("extension_count", len(normalizeNonEmptyStrings(dirs))),
		logger.F("error", err.Error()),
		logger.F("warning", warning),
	)
}

func (a *App) loadProfileExtensionsViaCDP(profileID string, debugPort int, profile *BrowserProfile) {
	if a == nil || a.browserMgr == nil || debugPort <= 0 {
		return
	}
	applyLoadedExtensionsViaCDP(debugPort, a.browserMgr.EnabledExtensionDirsForProfile(profileID), profile, profileID)
}

func writeExtensionDeveloperMode(userDataDir string) error {
	userDataDir = strings.TrimSpace(userDataDir)
	if userDataDir == "" {
		return nil
	}
	profileDir := filepath.Join(userDataDir, "Default")
	if err := os.MkdirAll(profileDir, 0o755); err != nil {
		return err
	}
	prefsPath := filepath.Join(profileDir, "Preferences")
	prefs := map[string]any{}
	if data, err := os.ReadFile(prefsPath); err == nil && len(strings.TrimSpace(string(data))) > 0 {
		if err := json.Unmarshal(data, &prefs); err != nil {
			return fmt.Errorf("解析浏览器偏好失败: %w", err)
		}
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	extensions, _ := prefs["extensions"].(map[string]any)
	if extensions == nil {
		extensions = map[string]any{}
	}
	ui, _ := extensions["ui"].(map[string]any)
	if ui == nil {
		ui = map[string]any{}
	}
	if enabled, _ := ui["developer_mode"].(bool); enabled {
		return nil
	}
	ui["developer_mode"] = true
	extensions["ui"] = ui
	prefs["extensions"] = extensions
	data, err := json.Marshal(prefs)
	if err != nil {
		return err
	}
	return os.WriteFile(prefsPath, data, 0o644)
}
