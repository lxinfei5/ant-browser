package backend

import "testing"

func TestBuildBrowserFingerprintLaunchPlanStripsProprietaryArgs(t *testing.T) {
	plan := buildBrowserFingerprintLaunchPlan("profile-a", []string{
		"--fingerprint=123",
		"--fingerprint-brand=Chrome",
		"--fingerprint-platform=windows",
		"--fingerprint-gpu-vendor=NVIDIA",
		"--fingerprint-canvas-noise=1",
		"--fingerprinting-canvas-image-data-noise",
		"--disable-gpu-fingerprint",
		"--disable-spoofing=font",
		"--lang=zh-CN",
		"--window-size=1280,720",
	}, "151.0.7922.108")

	assertArgsDoNotContainKey(t, plan.launchArgs, "--fingerprint")
	assertArgsDoNotContainKey(t, plan.launchArgs, "--fingerprint-brand")
	assertArgsDoNotContainKey(t, plan.launchArgs, "--fingerprint-platform")
	assertArgsDoNotContainKey(t, plan.launchArgs, "--fingerprint-gpu-vendor")
	assertArgsDoNotContainKey(t, plan.launchArgs, "--fingerprint-canvas-noise")
	assertArgsDoNotContainKey(t, plan.launchArgs, "--fingerprinting-canvas-image-data-noise")
	assertArgsDoNotContainKey(t, plan.launchArgs, "--disable-gpu-fingerprint")
	assertArgsDoNotContainKey(t, plan.launchArgs, "--disable-spoofing")
	assertArgsContain(t, plan.launchArgs, "--lang=zh-CN")
	assertArgsContain(t, plan.launchArgs, "--accept-lang=zh-CN,zh")
	assertArgsContain(t, plan.launchArgs, "--window-size=1280,720")
	assertRowsContainStatus(t, plan.rows, "指纹种子", "unsupported_stock")
	assertRowsContainStatus(t, plan.rows, "浏览器品牌", "unsupported_stock")
	assertRowsContainStatus(t, plan.rows, "Accept-Language", "inferred")
}

func TestBuildBrowserFingerprintLaunchPlanDoesNotInjectSeed(t *testing.T) {
	plan := buildBrowserFingerprintLaunchPlan("profile-a", []string{"--lang=zh-CN"}, "151.0.7922.108")

	assertArgsDoNotContainKey(t, plan.launchArgs, "--fingerprint")
	assertArgsContain(t, plan.launchArgs, "--lang=zh-CN")
	assertArgsContain(t, plan.launchArgs, "--accept-lang=zh-CN,zh")
	assertRowsContainStatus(t, plan.rows, "指纹种子", "unsupported_stock")
	assertRowsContainStatus(t, plan.rows, "Accept-Language", "inferred")
}

func TestBuildBrowserFingerprintLaunchPlanKeepsStockArgsOnly(t *testing.T) {
	plan := buildBrowserFingerprintLaunchPlan("profile-a", []string{
		"--timezone=Etc/GMT-8",
		"--disable-sync",
	}, "")

	assertArgsContain(t, plan.launchArgs, "--timezone=Etc/GMT-8")
	assertArgsContain(t, plan.launchArgs, "--disable-sync")
	if len(plan.warnings) == 0 {
		t.Fatalf("expected stock-chromium strategy warning")
	}
}

func assertArgsContain(t *testing.T, args []string, want string) {
	t.Helper()
	for _, arg := range args {
		if arg == want {
			return
		}
	}
	t.Fatalf("args %#v do not contain %q", args, want)
}

func assertArgsContainKey(t *testing.T, args []string, key string) {
	t.Helper()
	for _, arg := range args {
		if browserFingerprintArgKey(arg) == key {
			return
		}
	}
	t.Fatalf("args %#v do not contain key %q", args, key)
}

func assertArgsDoNotContainKey(t *testing.T, args []string, key string) {
	t.Helper()
	for _, arg := range args {
		if browserFingerprintArgKey(arg) == key {
			t.Fatalf("args %#v contain key %q", args, key)
		}
	}
}

func assertRowsContainStatus(t *testing.T, rows []BrowserFingerprintCapabilityRow, capability string, status string) {
	t.Helper()
	for _, row := range rows {
		if row.Capability == capability && row.Status == status {
			return
		}
	}
	t.Fatalf("rows %#v do not contain capability %q with status %q", rows, capability, status)
}
