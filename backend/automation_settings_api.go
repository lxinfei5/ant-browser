package backend

import (
	"fmt"
	"path/filepath"
	"strings"

	"ant-chrome/backend/internal/config"
	"ant-chrome/backend/internal/logger"
)

func (a *App) SaveAutomationSettings(enabled bool, headlessDefault bool) (map[string]interface{}, error) {
	if a.config == nil {
		return nil, fmt.Errorf("automation config is not initialized")
	}

	a.config.Automation.Enabled = enabled
	a.config.Automation.HeadlessDefault = headlessDefault
	applyAutomationConfigDefaults(&a.config.Automation)

	if err := a.config.Save(a.resolveAppPath("config.yaml")); err != nil {
		logger.New("Automation").Error("自动化配置保存失败", logger.F("error", err.Error()))
		return nil, err
	}

	if a.automationMgr != nil {
		a.automationMgr.SetConfig(a.config)
		state := a.automationMgr.CurrentState()
		if enabled && !state.Ready && strings.EqualFold(a.config.Automation.InstallPolicy, config.DefaultAutomationInstallPolicy) {
			a.automationMgr.InstallAsync(a.ctx)
		}
	}

	return a.automationStatePayload(), nil
}

func (a *App) SaveAutomationRuntimeSettings(nodeSource string, systemNodePath string) (map[string]interface{}, error) {
	if a.config == nil {
		return nil, fmt.Errorf("automation config is not initialized")
	}

	a.config.Automation.NodeSource = normalizeAutomationNodeSourceInput(nodeSource)
	a.config.Automation.SystemNodePath = strings.TrimSpace(systemNodePath)
	applyAutomationConfigDefaults(&a.config.Automation)

	if err := a.config.Save(a.resolveAppPath("config.yaml")); err != nil {
		logger.New("Automation").Error("自动化运行时策略保存失败", logger.F("error", err.Error()))
		return nil, err
	}

	if a.automationMgr != nil {
		a.automationMgr.SetConfig(a.config)
		if a.config.Automation.Enabled && strings.EqualFold(a.config.Automation.InstallPolicy, config.DefaultAutomationInstallPolicy) {
			a.automationMgr.InstallAsync(a.ctx)
		}
	}

	return a.automationStatePayload(), nil
}

func (a *App) SaveAutomationScriptPackageSettings(allowTypeScriptBuild bool) (map[string]interface{}, error) {
	if a.config == nil {
		return nil, fmt.Errorf("automation config is not initialized")
	}

	a.config.Automation.AllowTypeScriptBuild = allowTypeScriptBuild
	applyAutomationConfigDefaults(&a.config.Automation)

	if err := a.config.Save(a.resolveAppPath("config.yaml")); err != nil {
		logger.New("Automation").Error("自动化脚本包配置保存失败", logger.F("error", err.Error()))
		return nil, err
	}

	if a.automationMgr != nil {
		a.automationMgr.SetConfig(a.config)
	}

	return a.automationStatePayload(), nil
}

func (a *App) InstallAutomationRuntime() (map[string]interface{}, error) {
	if a.automationMgr == nil {
		return nil, fmt.Errorf("automation runtime manager is not initialized")
	}
	a.automationMgr.InstallAsync(a.ctx)
	return a.automationStatePayload(), nil
}

func (a *App) AutomationProbeSystemNode(systemNodePath string) (map[string]interface{}, error) {
	if a.automationMgr == nil {
		return nil, fmt.Errorf("automation runtime manager is not initialized")
	}

	// 安全：仅允许探测已注册/受管的运行时 Node，拒绝任意路径以防被引导执行任意可执行文件。
	registeredPath := strings.TrimSpace(a.automationMgr.CurrentState().NodePath)
	requested := strings.TrimSpace(systemNodePath)
	if requested == "" && a.config != nil {
		requested = strings.TrimSpace(a.config.Automation.SystemNodePath)
	}
	if requested != "" && !sameNodePath(requested, registeredPath) {
		return nil, fmt.Errorf("拒绝探测任意 Node 路径 %q（仅允许已注册的受管运行时 Node）", requested)
	}

	probePath := requested
	if probePath == "" {
		probePath = registeredPath
	}
	if probePath == "" {
		return nil, fmt.Errorf("未找到已注册的受管 Node，请先安装运行时")
	}

	result, err := a.automationMgr.ProbeSystemNode(a.ctx, probePath)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"ok":      result.OK,
		"path":    result.Path,
		"version": result.Version,
	}, nil
}

// sameNodePath 判断两个 Node 路径是否指向同一文件（按绝对路径归一化比较，大小写不敏感）。
func sameNodePath(a, b string) bool {
	abs := func(p string) string {
		p = strings.TrimSpace(p)
		if ap, err := filepath.Abs(p); err == nil {
			return ap
		}
		return p
	}
	return strings.EqualFold(filepath.Clean(abs(a)), filepath.Clean(abs(b)))
}

func (a *App) AutomationRuntimeSelfCheck() (map[string]interface{}, error) {
	if a.automationMgr == nil {
		return nil, fmt.Errorf("automation runtime manager is not initialized")
	}
	result, err := a.automationMgr.SelfCheck(a.ctx)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"ok":                result.OK,
		"nodeSource":        result.NodeSource,
		"nodeVersion":       result.NodeVersion,
		"playwrightVersion": result.PlaywrightVersion,
	}, nil
}
