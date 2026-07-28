package backend

import (
	"ant-chrome/backend/internal/logger"
	"strings"
)

type managedLaunchArgSpec struct {
	prefix     string
	takesValue bool
}

// managedLaunchArgSpecs 既是系统接管参数，也是安全敏感参数的 DENYLIST：
// 无论来源（profile.LaunchArgs / 一次性 ExtraLaunchArgs / FingerprintArgs）都会被剥离。
// 合法的指纹参数（--fingerprint* / --fingerprint-brand / --fingerprint-platform）不在列表中，予以保留。
var managedLaunchArgSpecs = []managedLaunchArgSpec{
	{prefix: "--user-data-dir", takesValue: true},
	{prefix: "--remote-debugging-port", takesValue: true},
	{prefix: "--remote-debugging-address", takesValue: true},
	{prefix: "--remote-debugging-pipe", takesValue: false},
	{prefix: "--proxy-server", takesValue: true},
	{prefix: "--proxy-pac-url", takesValue: true},
	{prefix: "--load-extension", takesValue: true},
	{prefix: "--disable-extensions-except", takesValue: true},
	{prefix: "--ignore-certificate-errors", takesValue: false},
	{prefix: "--disable-web-security", takesValue: false},
	{prefix: "--ignore-urlfetcher-cert-requests", takesValue: false},
	{prefix: "--restore-last-session", takesValue: false},
}

func sanitizeManagedLaunchArgs(args []string) ([]string, []string) {
	if len(args) == 0 {
		return nil, nil
	}

	sanitized := make([]string, 0, len(args))
	removed := make([]string, 0, 4)

	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		if arg == "" {
			continue
		}

		spec, matched := matchManagedLaunchArg(arg)
		if !matched {
			sanitized = append(sanitized, arg)
			continue
		}

		removed = appendUniqueString(removed, spec.prefix)
		if spec.takesValue && !strings.Contains(arg, "=") && i+1 < len(args) {
			next := strings.TrimSpace(args[i+1])
			if next != "" && !strings.HasPrefix(next, "-") {
				i++
			}
		}
	}

	return sanitized, removed
}

func matchManagedLaunchArg(arg string) (managedLaunchArgSpec, bool) {
	// 归一化前导连字符：Chromium 接受 -foo / --foo 两种长 flag 形式，否则用单连字符即可绕过 denylist。
	normalizedArg := strings.TrimLeft(strings.ToLower(arg), "-")
	for _, spec := range managedLaunchArgSpecs {
		normalizedPrefix := strings.TrimLeft(strings.ToLower(spec.prefix), "-")
		if normalizedArg == normalizedPrefix || strings.HasPrefix(normalizedArg, normalizedPrefix+"=") {
			return spec, true
		}
	}
	return managedLaunchArgSpec{}, false
}

func logManagedLaunchArgOverrides(log *logger.Logger, profileId string, source string, managedArgs []string) {
	if log == nil || len(managedArgs) == 0 {
		return
	}
	log.Warn("忽略由系统接管的浏览器启动参数",
		logger.F("profile_id", profileId),
		logger.F("source", source),
		logger.F("managed_args", managedArgs),
	)
}

func appendUniqueString(items []string, value string) []string {
	for _, item := range items {
		if strings.EqualFold(item, value) {
			return items
		}
	}
	return append(items, value)
}
