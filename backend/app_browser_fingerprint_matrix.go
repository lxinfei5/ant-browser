package backend

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

const fingerprintChrome144Major = 144

type BrowserFingerprintCapabilityReport struct {
	ProfileId     string                            `json:"profileId"`
	CoreId        string                            `json:"coreId"`
	CoreName      string                            `json:"coreName"`
	ChromeVersion string                            `json:"chromeVersion"`
	ChromeMajor   int                               `json:"chromeMajor"`
	VersionStatus string                            `json:"versionStatus"`
	RawArgs       []string                          `json:"rawArgs"`
	LaunchArgs    []string                          `json:"launchArgs"`
	Rows          []BrowserFingerprintCapabilityRow `json:"rows"`
	Warnings      []string                          `json:"warnings"`
}

type BrowserFingerprintCapabilityRow struct {
	Capability string `json:"capability"`
	Status     string `json:"status"`
	InputArg   string `json:"inputArg"`
	RuntimeArg string `json:"runtimeArg"`
	Action     string `json:"action"`
	Note       string `json:"note"`
}

type browserFingerprintLaunchPlan struct {
	launchArgs []string
	rows       []BrowserFingerprintCapabilityRow
	warnings   []string
}

var fingerprintStableArgLabels = map[string]string{
	"--fingerprint":                            "指纹种子",
	"--fingerprint-brand":                      "浏览器品牌",
	"--fingerprint-brand-version":              "品牌版本",
	"--fingerprint-platform":                   "平台",
	"--fingerprint-platform-version":           "系统版本",
	"--fingerprint-hardware-concurrency":       "CPU 核心数",
	"--fingerprinting-canvas-image-data-noise": "Canvas ImageData",
	"--fingerprinting-client-rects-noise":      "ClientRects",
	"--lang":                                   "语言",
	"--accept-lang":                            "Accept-Language",
	"--timezone":                               "时区",
	"--window-size":                            "窗口大小",
	"--disable-non-proxied-udp":                "WebRTC",
	"--webrtc-ip-handling-policy":              "WebRTC",
	"--disable-spoofing":                       "排除伪装",
}

var fingerprintNoEffectArgLabels = map[string]string{
	"--fingerprint-color-depth":                 "色深",
	"--fingerprint-device-memory":               "设备内存",
	"--fingerprint-touch-points":                "触控点",
	"--fingerprint-do-not-track":                "Do Not Track",
	"--fingerprint-media-devices":               "媒体设备",
	"--fingerprint-audio-noise":                 "Audio",
	"--fingerprint-font-list":                   "字体",
	"--fingerprint-fonts":                       "字体",
	"--fingerprint-webgl-vendor":                "WebGL Vendor",
	"--fingerprint-webgl-renderer":              "WebGL Renderer",
	"--fingerprint-screen-width":                "屏幕宽度",
	"--fingerprint-screen-height":               "屏幕高度",
	"--fingerprint-device-scale-factor":         "DPR",
	"--fingerprint-location":                    "地理位置",
	"--fingerprinting-canvas-measuretext-noise": "Canvas MeasureText",
}

var fingerprintChrome144ConvertedArgLabels = map[string]struct {
	label      string
	runtimeArg string
}{
	"--fingerprint-canvas-noise":       {label: "Canvas ImageData", runtimeArg: "--fingerprinting-canvas-image-data-noise"},
	"--fingerprint-client-rects-noise": {label: "ClientRects", runtimeArg: "--fingerprinting-client-rects-noise"},
}

var fingerprintEffectiveNoiseArgLabels = map[string]string{
	"--fingerprinting-canvas-image-data-noise": "Canvas ImageData",
	"--fingerprinting-client-rects-noise":      "ClientRects",
}

func fingerprintNoiseRuntimeArgForKey(key string) (string, string, bool) {
	key = strings.ToLower(strings.TrimSpace(key))
	if converted, ok := fingerprintChrome144ConvertedArgLabels[key]; ok {
		return converted.runtimeArg, converted.label, true
	}
	if label, ok := fingerprintEffectiveNoiseArgLabels[key]; ok {
		return key, label, true
	}
	return "", "", false
}

func latestFingerprintNoiseArgIndexes(args []string) map[string]int {
	indexes := make(map[string]int)
	for index, arg := range args {
		key := browserFingerprintArgKey(arg)
		if runtimeArg, _, ok := fingerprintNoiseRuntimeArgForKey(key); ok {
			indexes[runtimeArg] = index
		}
	}
	return indexes
}

var fingerprintChrome144RemovedArgLabels = map[string]string{
	"--fingerprint-gpu-vendor":   "GPU Vendor",
	"--fingerprint-gpu-renderer": "GPU Renderer",
}

func (a *App) BrowserProfileFingerprintMatrix(profileId string, coreId string, fingerprintArgs []string) BrowserFingerprintCapabilityReport {
	return a.buildBrowserFingerprintCapabilityReport(profileId, coreId, fingerprintArgs)
}

func (a *App) buildBrowserFingerprintCapabilityReport(profileId string, coreId string, fingerprintArgs []string) BrowserFingerprintCapabilityReport {
	profileId = strings.TrimSpace(profileId)
	coreId = strings.TrimSpace(coreId)
	if coreId == "" && profileId != "" && a != nil && a.browserMgr != nil {
		if profile, ok := a.browserMgr.Profiles[profileId]; ok && profile != nil {
			coreId = strings.TrimSpace(profile.CoreId)
		}
	}

	var core BrowserCore
	var coreFound bool
	if a != nil && a.browserMgr != nil {
		core, coreFound = a.resolveBrowserCoreForFingerprintReport(coreId)
	}

	chromeVersion := ""
	if coreFound {
		chromeVersion = a.browserMgr.GetChromeVersion(core.CorePath)
	}

	plan := buildBrowserFingerprintLaunchPlan(profileId, fingerprintArgs, chromeVersion)
	report := BrowserFingerprintCapabilityReport{
		ProfileId:     profileId,
		ChromeVersion: chromeVersion,
		ChromeMajor:   parseChromeMajor(chromeVersion),
		VersionStatus: fingerprintVersionStatus(chromeVersion),
		RawArgs:       normalizeNonEmptyStrings(fingerprintArgs),
		LaunchArgs:    append([]string{}, plan.launchArgs...),
		Rows:          append([]BrowserFingerprintCapabilityRow{}, plan.rows...),
		Warnings:      append([]string{}, plan.warnings...),
	}
	if coreFound {
		report.CoreId = core.CoreId
		report.CoreName = core.CoreName
	} else {
		report.Warnings = appendUniqueString(report.Warnings, "未找到可用内核，矩阵按未知版本保守展示")
	}
	return report
}

func (a *App) resolveBrowserCoreForFingerprintReport(coreId string) (BrowserCore, bool) {
	if a == nil || a.browserMgr == nil || a.browserMgr.Config == nil {
		return BrowserCore{}, false
	}
	coreId = strings.TrimSpace(coreId)
	if strings.EqualFold(coreId, "default") {
		coreId = ""
	}
	if coreId != "" {
		if core, ok := a.browserMgr.GetCore(coreId); ok {
			return core, true
		}
	}
	return a.browserMgr.GetDefaultCore()
}

// isFingerprintChromiumProprietaryArg reports CLI flags that only exist on
// fingerprint-chromium / patched anti-detect builds. Stock Chromium and Google
// Chrome do not implement them and they must never be passed at launch.
func isFingerprintChromiumProprietaryArg(arg string) bool {
	key := browserFingerprintArgKey(arg)
	if key == "" {
		return false
	}
	switch {
	case key == "--fingerprint", strings.HasPrefix(key, "--fingerprint-"), strings.HasPrefix(key, "--fingerprinting-"):
		return true
	case key == "--disable-gpu-fingerprint", key == "--disable-spoofing":
		return true
	default:
		return false
	}
}

func fingerprintCapabilityLabelForArg(arg string) string {
	key := browserFingerprintArgKey(arg)
	if label, ok := fingerprintStableArgLabels[key]; ok {
		return label
	}
	if label, ok := fingerprintNoEffectArgLabels[key]; ok {
		return label
	}
	if converted, ok := fingerprintChrome144ConvertedArgLabels[key]; ok {
		return converted.label
	}
	if label, ok := fingerprintChrome144RemovedArgLabels[key]; ok {
		return label
	}
	if label, ok := fingerprintEffectiveNoiseArgLabels[key]; ok {
		return label
	}
	switch key {
	case "--disable-gpu-fingerprint":
		return "GPU 伪装开关"
	case "--disable-spoofing":
		return "排除伪装"
	}
	if key != "" {
		return key
	}
	return "未知参数"
}

// buildBrowserFingerprintLaunchPlan prepares launch args for stock Chromium/Chrome.
// It never injects fingerprint-chromium seeds and always strips proprietary flags
// so official binaries are not launched with unsupported CLI.
func buildBrowserFingerprintLaunchPlan(profileId string, rawArgs []string, chromeVersion string) browserFingerprintLaunchPlan {
	_ = profileId
	_ = chromeVersion
	originalArgs := normalizeNonEmptyStrings(rawArgs)
	args := normalizeBrowserLanguageArgs(originalArgs)

	plan := browserFingerprintLaunchPlan{
		launchArgs: make([]string, 0, len(args)),
		rows:       make([]BrowserFingerprintCapabilityRow, 0, len(args)+2),
		warnings: []string{
			"内核策略：官方 Chromium / Google Chrome。已移除 fingerprint-chromium 专有启动参数与自动 --fingerprint 种子注入。",
		},
	}

	plan.rows = append(plan.rows, BrowserFingerprintCapabilityRow{
		Capability: "指纹种子",
		Status:     "unsupported_stock",
		Action:     "不注入",
		Note:       "官方 Chromium/Chrome 不支持 --fingerprint；实例隔离依赖独立 user-data-dir 与代理，不再注入指纹种子",
	})

	appendLanguageCompletionRows(&plan, originalArgs, args)

	for _, arg := range args {
		key := browserFingerprintArgKey(arg)
		if isFingerprintChromiumProprietaryArg(arg) {
			plan.rows = append(plan.rows, BrowserFingerprintCapabilityRow{
				Capability: fingerprintCapabilityLabelForArg(arg),
				Status:     "unsupported_stock",
				InputArg:   arg,
				Action:     "运行时剥离",
				Note:       "fingerprint-chromium 专有参数，官方内核不支持，启动时丢弃",
			})
			continue
		}
		plan.launchArgs = append(plan.launchArgs, arg)
		if label, ok := fingerprintStableArgLabels[key]; ok {
			if key == "--accept-lang" {
				continue
			}
			plan.rows = append(plan.rows, BrowserFingerprintCapabilityRow{
				Capability: label,
				Status:     "kept",
				InputArg:   arg,
				RuntimeArg: arg,
				Action:     "保留",
				Note:       "官方 Chromium 兼容参数",
			})
			continue
		}
		if key != "" {
			plan.rows = append(plan.rows, BrowserFingerprintCapabilityRow{
				Capability: key,
				Status:     "kept",
				InputArg:   arg,
				RuntimeArg: arg,
				Action:     "保留",
				Note:       "按原样传递给官方 Chromium/Chrome",
			})
		}
	}

	return plan
}

func appendLanguageCompletionRows(plan *browserFingerprintLaunchPlan, originalArgs []string, normalizedArgs []string) {
	if plan == nil {
		return
	}
	originalLang := browserArgValue(originalArgs, browserLangArg)
	originalAcceptLang := browserArgValue(originalArgs, browserAcceptLangArg)
	normalizedLang := browserArgValue(normalizedArgs, browserLangArg)
	normalizedAcceptLang := browserArgValue(normalizedArgs, browserAcceptLangArg)
	if originalLang != "" && originalAcceptLang == "" && normalizedAcceptLang != "" {
		runtimeArg := fmt.Sprintf("%s=%s", browserAcceptLangArg, normalizedAcceptLang)
		plan.rows = append(plan.rows, BrowserFingerprintCapabilityRow{
			Capability: "Accept-Language",
			Status:     "inferred",
			InputArg:   fmt.Sprintf("%s=%s", browserLangArg, originalLang),
			RuntimeArg: runtimeArg,
			Action:     "后端补齐",
			Note:       "避免 navigator.language 与请求语言只配置一侧",
		})
	}
	if originalLang == "" && originalAcceptLang != "" && normalizedLang != "" {
		runtimeArg := fmt.Sprintf("%s=%s", browserLangArg, normalizedLang)
		plan.rows = append(plan.rows, BrowserFingerprintCapabilityRow{
			Capability: "语言",
			Status:     "inferred",
			InputArg:   fmt.Sprintf("%s=%s", browserAcceptLangArg, originalAcceptLang),
			RuntimeArg: runtimeArg,
			Action:     "后端补齐",
			Note:       "避免只设置 Accept-Language 造成语言指纹不完整",
		})
	}
}

func parseChromeMajor(version string) int {
	version = strings.TrimSpace(version)
	if version == "" {
		return 0
	}
	parts := strings.FieldsFunc(version, func(r rune) bool { return !unicode.IsDigit(r) })
	if len(parts) == 0 || parts[0] == "" {
		return 0
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil || major < 0 {
		return 0
	}
	return major
}

func fingerprintVersionStatus(version string) string {
	major := parseChromeMajor(version)
	if major <= 0 {
		return "unknown"
	}
	if major >= fingerprintChrome144Major {
		return "chrome144_plus"
	}
	return "legacy"
}

func browserFingerprintArgKey(arg string) string {
	arg = strings.TrimSpace(arg)
	if arg == "" || !strings.HasPrefix(arg, "--") {
		return ""
	}
	if index := strings.Index(arg, "="); index > 0 {
		return strings.ToLower(strings.TrimSpace(arg[:index]))
	}
	return strings.ToLower(arg)
}

func browserArgWithKey(args []string, key string) string {
	key = strings.ToLower(strings.TrimSpace(key))
	for index := len(args) - 1; index >= 0; index-- {
		arg := strings.TrimSpace(args[index])
		if browserFingerprintArgKey(arg) == key {
			return arg
		}
	}
	return ""
}

func browserFingerprintArgEnabled(arg string) bool {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return false
	}
	if index := strings.Index(arg, "="); index >= 0 {
		value := strings.ToLower(strings.TrimSpace(arg[index+1:]))
		return value == "1" || value == "true" || value == "yes" || value == "on"
	}
	return true
}

func splitCSV(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func appendUniqueStringValues(values []string, additions ...string) []string {
	for _, addition := range additions {
		addition = strings.TrimSpace(addition)
		if addition == "" {
			continue
		}
		seen := false
		for _, value := range values {
			if strings.EqualFold(value, addition) {
				seen = true
				break
			}
		}
		if !seen {
			values = append(values, addition)
		}
	}
	return values
}

func legacyKeepStatus(unknownVersion bool) string {
	if unknownVersion {
		return "kept_unconfirmed"
	}
	return "kept_legacy"
}

func legacyKeepNote(unknownVersion bool) string {
	if unknownVersion {
		return "未识别内核版本，无法确认支持范围；为避免误删按原样传递"
	}
	return "144 之前的内核按旧参数兼容传递"
}
