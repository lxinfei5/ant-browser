package backend

import (
	"fmt"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"ant-chrome/backend/internal/automation"
	"ant-chrome/backend/internal/logger"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type AutomationScriptImportIssue struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

type AutomationScriptBatchImportResult struct {
	Imported []automation.ScriptRecord     `json:"imported"`
	Failed   []AutomationScriptImportIssue `json:"failed"`
	Scanned  int                           `json:"scanned"`
}

func (a *App) AutomationScriptImportText(text string) (*automation.ScriptRecord, error) {
	bundle, err := automation.ImportBundleFromBytesWithOptions("automation-template.json", []byte(strings.TrimSpace(text)), "文本导入", a.automationScriptImportOptions())
	if err != nil {
		return nil, err
	}
	return a.saveImportedAutomationBundle(bundle)
}

func (a *App) AutomationScriptImportLocalFile() (*automation.ScriptRecord, error) {
	if a.ctx == nil {
		return nil, fmt.Errorf("应用上下文未初始化")
	}

	path, err := wailsruntime.OpenFileDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title: "选择脚本文件",
		Filters: []wailsruntime.FileFilter{
			{DisplayName: "脚本文件 (*.zip;*.json;*.js;*.cjs;*.mjs;*.ts;*.cts;*.mts)", Pattern: "*.zip;*.json;*.js;*.cjs;*.mjs;*.ts;*.cts;*.mts"},
			{DisplayName: "所有文件 (*.*)", Pattern: "*.*"},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("打开文件对话框失败: %w", err)
	}
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("未选择脚本文件")
	}

	bundle, err := automation.ImportBundleFromFileWithOptions(path, "本地文件 "+path, a.automationScriptImportOptions())
	if err != nil {
		return nil, err
	}
	return a.saveImportedAutomationBundle(bundle)
}

func (a *App) AutomationScriptImportLocalDirectory() (*automation.ScriptRecord, error) {
	if a.ctx == nil {
		return nil, fmt.Errorf("应用上下文未初始化")
	}

	path, err := wailsruntime.OpenDirectoryDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title: "选择脚本目录",
	})
	if err != nil {
		return nil, fmt.Errorf("打开目录对话框失败: %w", err)
	}
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("未选择脚本目录")
	}

	bundle, err := automation.ImportBundleFromDirectoryWithOptions(path, "", "本地目录 "+path, a.automationScriptImportOptions())
	if err != nil {
		return nil, err
	}
	return a.saveImportedAutomationBundle(bundle)
}

func (a *App) AutomationScriptImportLocalLibrary() (*AutomationScriptBatchImportResult, error) {
	if a.ctx == nil {
		return nil, fmt.Errorf("应用上下文未初始化")
	}

	path, err := wailsruntime.OpenDirectoryDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title: "选择脚本库目录",
	})
	if err != nil {
		return nil, fmt.Errorf("打开目录对话框失败: %w", err)
	}
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("未选择脚本库目录")
	}

	return a.importAutomationLocalLibrary(path)
}

func (a *App) AutomationScriptImportRemote(rawURL string) (*automation.ScriptRecord, error) {
	return a.AutomationScriptImportRemoteConfirm(rawURL, false)
}

// AutomationScriptImportRemoteConfirm 远程脚本导入，默认禁用；需显式 confirm=true 才允许，
// 且必须携带当前 LaunchServer API key（默认开启），导入会记录告警日志。
func (a *App) AutomationScriptImportRemoteConfirm(rawURL string, confirm bool) (*automation.ScriptRecord, error) {
	log := logger.New("Automation")
	if !confirm {
		log.Warn("远程脚本导入被拒绝：未显式确认")
		return nil, fmt.Errorf("远程脚本导入默认禁用，需显式确认（confirm=true）")
	}
	// 要求当前 API key 已配置（认证默认开启），降低匿名/CSRF 触发风险。
	if a.config != nil && a.config.LaunchServer.Auth.IsEnabled() && strings.TrimSpace(a.config.LaunchServer.Auth.APIKey) == "" {
		return nil, fmt.Errorf("远程脚本导入需要 LaunchServer API key 已配置")
	}
	log.Warn("远程脚本导入已获确认，开始拉取", logger.F("url", strings.TrimSpace(rawURL)))
	bundle, err := a.loadAutomationRemoteBundle(strings.TrimSpace(rawURL))
	if err != nil {
		return nil, err
	}
	return a.saveImportedAutomationBundle(bundle)
}

func (a *App) AutomationScriptImportGit(repoURL string, ref string, scriptPath string) (*automation.ScriptRecord, error) {
	bundle, err := a.loadAutomationGitBundle(strings.TrimSpace(repoURL), strings.TrimSpace(ref), strings.TrimSpace(scriptPath))
	if err != nil {
		return nil, err
	}
	return a.saveImportedAutomationBundle(bundle)
}

func (a *App) AutomationScriptRefresh(scriptID string) (*automation.ScriptRecord, error) {
	normalizedID := strings.TrimSpace(scriptID)
	if normalizedID == "" {
		return nil, fmt.Errorf("脚本 ID 不能为空")
	}

	existing, err := a.automationScriptStore().Get(normalizedID)
	if err != nil {
		return nil, fmt.Errorf("读取脚本失败: %w", err)
	}

	bundle, err := a.loadAutomationBundleFromSource(existing.Source)
	if err != nil {
		return nil, err
	}

	bundle.Record.ID = existing.ID
	bundle.Record.CreatedAt = existing.CreatedAt
	bundle.Record.Status = existing.Status
	bundle.Record.PublicAPI = existing.PublicAPI

	record, err := a.automationScriptStore().ImportBundle(bundle)
	if err != nil {
		return nil, err
	}
	return &record, nil
}

func (a *App) saveImportedAutomationBundle(bundle automation.ImportedBundle) (*automation.ScriptRecord, error) {
	record, err := a.automationScriptStore().ImportBundle(bundle)
	if err != nil {
		return nil, err
	}
	return &record, nil
}

func (a *App) importAutomationLocalLibrary(rootDir string) (*AutomationScriptBatchImportResult, error) {
	directories, err := automation.DiscoverImportableScriptDirectoriesWithOptions(rootDir)
	if err != nil {
		return nil, err
	}

	existingBySource, err := a.automationScriptsBySourceKey()
	if err != nil {
		return nil, err
	}

	result := &AutomationScriptBatchImportResult{
		Imported: make([]automation.ScriptRecord, 0, len(directories)),
		Failed:   []AutomationScriptImportIssue{},
		Scanned:  len(directories),
	}

	store := a.automationScriptStore()
	for _, dir := range directories {
		bundle, err := automation.ImportBundleFromDirectoryWithOptions(dir, "", "本地目录 "+dir, a.automationScriptImportOptions())
		if err != nil {
			result.Failed = append(result.Failed, AutomationScriptImportIssue{
				Path:    dir,
				Message: err.Error(),
			})
			continue
		}

		sourceKey := automationScriptSourceKey(bundle.Record.Source)
		if existing, exists := existingBySource[sourceKey]; exists {
			bundle.Record.ID = existing.ID
			bundle.Record.CreatedAt = existing.CreatedAt
			bundle.Record.Status = existing.Status
			bundle.Record.PublicAPI = existing.PublicAPI
		}

		record, err := store.ImportBundle(bundle)
		if err != nil {
			result.Failed = append(result.Failed, AutomationScriptImportIssue{
				Path:    dir,
				Message: err.Error(),
			})
			continue
		}

		enriched := a.enrichAutomationScriptRecord(record)
		result.Imported = append(result.Imported, enriched)
		existingBySource[automationScriptSourceKey(record.Source)] = record
	}

	sort.Slice(result.Imported, func(i, j int) bool {
		return strings.TrimSpace(result.Imported[i].UpdatedAt) > strings.TrimSpace(result.Imported[j].UpdatedAt)
	})

	if len(result.Imported) == 0 {
		if len(result.Failed) == 0 {
			return nil, fmt.Errorf("未导入任何脚本")
		}
		firstFailure := result.Failed[0]
		if len(result.Failed) == 1 {
			return nil, fmt.Errorf("导入失败: %s", firstFailure.Message)
		}
		return nil, fmt.Errorf("导入失败: %s；另有 %d 个脚本包也失败", firstFailure.Message, len(result.Failed)-1)
	}

	return result, nil
}

func (a *App) automationScriptsBySourceKey() (map[string]automation.ScriptRecord, error) {
	items, err := a.automationScriptStore().List()
	if err != nil {
		return nil, err
	}

	result := make(map[string]automation.ScriptRecord, len(items))
	for _, item := range items {
		key := automationScriptSourceKey(item.Source)
		if key == "" {
			continue
		}
		result[key] = item
	}
	return result, nil
}

func automationScriptSourceKey(source automation.ScriptSource) string {
	sourceType := strings.TrimSpace(source.Type)
	if sourceType == "" {
		return ""
	}

	normalizeLocalPath := func(value string) string {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return ""
		}
		cleaned := filepath.Clean(trimmed)
		if strings.EqualFold(cleaned, ".") {
			return ""
		}
		if runtime.GOOS == "windows" {
			return strings.ToLower(cleaned)
		}
		return cleaned
	}

	uri := strings.TrimSpace(source.URI)
	ref := strings.TrimSpace(source.Ref)
	path := strings.TrimSpace(source.Path)

	switch sourceType {
	case "local-file", "local-dir":
		uri = normalizeLocalPath(firstNonBlank(uri, path))
		path = ""
	case "git":
		if path != "" {
			path = filepath.ToSlash(filepath.Clean(path))
		}
	default:
		uri = strings.TrimSpace(uri)
		path = strings.TrimSpace(path)
	}

	return strings.Join([]string{
		strings.ToLower(sourceType),
		uri,
		ref,
		path,
	}, "|")
}

func (a *App) automationScriptImportOptions() automation.ImportOptions {
	if a.config == nil {
		return automation.ImportOptions{}
	}
	return automation.ImportOptions{
		AllowTypeScriptBuild: a.config.Automation.AllowTypeScriptBuild,
	}
}
