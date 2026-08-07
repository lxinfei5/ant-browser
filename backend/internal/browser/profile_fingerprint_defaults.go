package browser

import "strings"

// upgradeLegacyMinimalFingerprintArgs strips fingerprint-chromium proprietary
// CLI flags so stock Chromium/Chrome profiles never carry them forward.
// The name is kept for call-site compatibility (load/save path).
func upgradeLegacyMinimalFingerprintArgs(args []string) []string {
	return stripProprietaryFingerprintArgs(args)
}

func stripProprietaryFingerprintArgs(args []string) []string {
	if len(args) == 0 {
		return []string{}
	}
	out := make([]string, 0, len(args))
	for _, arg := range args {
		trimmed := strings.TrimSpace(arg)
		if trimmed == "" {
			continue
		}
		if isProprietaryFingerprintArg(trimmed) {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}

func isProprietaryFingerprintArg(arg string) bool {
	normalized := strings.ToLower(strings.TrimSpace(arg))
	normalized = strings.TrimLeft(normalized, "-")
	if normalized == "" {
		return false
	}
	key := normalized
	if idx := strings.Index(key, "="); idx >= 0 {
		key = key[:idx]
	}
	switch {
	case key == "fingerprint", strings.HasPrefix(key, "fingerprint-"), strings.HasPrefix(key, "fingerprinting-"):
		return true
	case key == "disable-gpu-fingerprint", key == "disable-spoofing":
		return true
	default:
		return false
	}
}
