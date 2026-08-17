package browser

import (
	"ant-chrome/backend/internal/browser/builtins/forcefont"
	"ant-chrome/backend/internal/logger"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const builtinFilesRoot = "files"

func IsBuiltinExtensionID(extensionID string) bool {
	return strings.TrimSpace(extensionID) == forcefont.ExtensionID
}

func IsBuiltinExtensionManifest(manifestData []byte) bool {
	return len(manifestData) > 0 && strings.Contains(string(manifestData), forcefont.ManifestKey)
}

func BuiltinExtensionIDs() []string {
	return []string{forcefont.ExtensionID}
}

func (m *Manager) EnsureBuiltinExtensions() error {
	if m == nil {
		return fmt.Errorf("浏览器管理器未初始化")
	}
	_, err := m.syncBuiltinForceFont()
	return err
}

func (m *Manager) syncBuiltinForceFont() (string, error) {
	files, err := readBuiltinForceFontFiles()
	if err != nil {
		return "", err
	}
	installDir := m.builtinForceFontInstallDir()
	if err := materializeBuiltinFiles(installDir, files); err != nil {
		return "", err
	}
	if err := verifyBuiltinFiles(installDir, files); err != nil {
		return "", err
	}
	if err := m.upsertBuiltinForceFont(installDir, files["manifest.json"]); err != nil {
		return "", err
	}
	return installDir, nil
}

func (m *Manager) builtinForceFontInstallDir() string {
	return m.ResolveRelativePath(filepath.Join("data", extensionsRootDir, forcefont.ExtensionID))
}

func readBuiltinForceFontFiles() (map[string][]byte, error) {
	files := map[string][]byte{}
	err := fs.WalkDir(forcefont.Files, builtinFilesRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		relative := strings.TrimPrefix(filepath.ToSlash(path), builtinFilesRoot+"/")
		if relative == "" || relative == path || strings.Contains(relative, "..") {
			return fmt.Errorf("内置插件包含非法路径: %s", path)
		}
		data, err := forcefont.Files.ReadFile(path)
		if err != nil {
			return fmt.Errorf("读取内置插件失败: %w", err)
		}
		files[relative] = data
		return nil
	})
	if err != nil {
		return nil, err
	}
	if _, ok := files["manifest.json"]; !ok {
		return nil, fmt.Errorf("内置插件缺少 manifest.json")
	}
	if _, ok := files["content.js"]; !ok {
		return nil, fmt.Errorf("内置插件缺少 content.js")
	}
	return files, nil
}

func materializeBuiltinFiles(installDir string, files map[string][]byte) error {
	if diskMatchesBuiltin(installDir, files) {
		return nil
	}
	tmpDir := installDir + ".tmp"
	_ = os.RemoveAll(tmpDir)
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		return fmt.Errorf("创建内置插件目录失败: %w", err)
	}
	success := false
	defer func() {
		if !success {
			_ = os.RemoveAll(tmpDir)
		}
	}()
	for relative, data := range files {
		target := filepath.Join(tmpDir, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("创建内置插件文件目录失败: %w", err)
		}
		if err := os.WriteFile(target, data, 0o644); err != nil {
			return fmt.Errorf("写入内置插件失败: %w", err)
		}
	}
	if err := os.RemoveAll(installDir); err != nil {
		return fmt.Errorf("清理旧内置插件失败: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(installDir), 0o755); err != nil {
		return fmt.Errorf("创建插件根目录失败: %w", err)
	}
	if err := os.Rename(tmpDir, installDir); err != nil {
		return fmt.Errorf("安装内置插件失败: %w", err)
	}
	success = true
	return nil
}

func diskMatchesBuiltin(installDir string, files map[string][]byte) bool {
	for relative, expected := range files {
		data, err := os.ReadFile(filepath.Join(installDir, filepath.FromSlash(relative)))
		if err != nil {
			return false
		}
		if sha256.Sum256(data) != sha256.Sum256(expected) {
			return false
		}
	}
	entries, err := listRegularRelativeFiles(installDir)
	if err != nil || len(entries) != len(files) {
		return false
	}
	return true
}

func verifyBuiltinFiles(installDir string, files map[string][]byte) error {
	if !diskMatchesBuiltin(installDir, files) {
		return fmt.Errorf("内置插件完整性校验失败")
	}
	return nil
}

func listRegularRelativeFiles(root string) ([]string, error) {
	var names []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("内置插件目录包含符号链接: %s", path)
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		names = append(names, filepath.ToSlash(relative))
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	return names, nil
}

func builtinFilesDigest(files map[string][]byte) string {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	sum := sha256.New()
	for _, name := range names {
		fileSum := sha256.Sum256(files[name])
		fmt.Fprintf(sum, "%s\n%s\n", name, hex.EncodeToString(fileSum[:]))
	}
	return hex.EncodeToString(sum.Sum(nil))
}

func (m *Manager) upsertBuiltinForceFont(installDir string, manifestData []byte) error {
	if m.ExtensionDAO == nil {
		return nil
	}
	existing, err := m.ExtensionDAO.Get(forcefont.ExtensionID)
	isNew := err != nil
	enabled := true
	installedAt := ""
	if !isNew {
		enabled = existing.Enabled
		installedAt = existing.InstalledAt
	}
	manifest, err := parseExtensionManifest(manifestData)
	if err != nil {
		return err
	}
	extension := Extension{
		ExtensionID:  forcefont.ExtensionID,
		Name:         resolveExtensionName(manifest, "强制字体"),
		Version:      firstNonEmpty(strings.TrimSpace(manifest.Version), forcefont.Version),
		Description:  strings.TrimSpace(manifest.Description),
		ManifestJSON: string(manifestData),
		SourceURL:    forcefont.SourceURL,
		InstallDir:   installDir,
		Enabled:      enabled,
		Builtin:      true,
		InstalledAt:  installedAt,
	}
	if err := m.ExtensionDAO.Upsert(extension); err != nil {
		return err
	}
	if isNew {
		if seedErr := m.ExtensionDAO.SeedExtensionIntoConfiguredProfiles(forcefont.ExtensionID); seedErr != nil {
			logger.New("Browser").Error("内置插件预置到已单独配置的实例失败",
				logger.F("extension_id", forcefont.ExtensionID),
				logger.F("error", seedErr.Error()),
			)
		}
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func (m *Manager) loadableExtensionDir(item Extension) string {
	dir := strings.TrimSpace(item.InstallDir)
	if IsBuiltinExtensionID(item.ExtensionID) {
		synced, err := m.syncBuiltinForceFont()
		if err != nil {
			logger.New("Browser").Error("内置插件完整性校验失败，已拒绝加载",
				logger.F("extension_id", item.ExtensionID),
				logger.F("error", err.Error()),
			)
			return ""
		}
		dir = synced
	}
	if dir == "" {
		return ""
	}
	if _, err := os.Stat(filepath.Join(dir, "manifest.json")); err != nil {
		return ""
	}
	return dir
}
