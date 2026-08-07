package config

import "testing"

func TestDefaultFingerprintArgsAreEmptyForStockChromium(t *testing.T) {
	cfg := DefaultConfig()
	if len(cfg.Browser.DefaultFingerprintArgs) != 0 {
		t.Fatalf("default fingerprint args = %#v, want empty for stock Chromium", cfg.Browser.DefaultFingerprintArgs)
	}
	assertStringSliceContains(t, cfg.Browser.DefaultLaunchArgs, "--disable-sync")
	assertStringSliceContains(t, cfg.Browser.DefaultLaunchArgs, "--no-first-run")
	assertStringSliceContains(t, cfg.Browser.DefaultLaunchArgs, "--disable-non-proxied-udp")
}

func TestNormalizeConfigStripsProprietaryDefaultFingerprintArgs(t *testing.T) {
	config := &Config{}
	config.Browser.DefaultFingerprintArgs = []string{
		"--fingerprint-brand=Chrome",
		"--fingerprint-platform=windows",
		"--fingerprinting-canvas-image-data-noise",
		"--lang=zh-CN",
	}

	normalizeConfig(config)

	if got, want := len(config.Browser.DefaultFingerprintArgs), 1; got != want {
		t.Fatalf("default fingerprint args length = %d, want %d: %#v", got, want, config.Browser.DefaultFingerprintArgs)
	}
	assertStringSliceContains(t, config.Browser.DefaultFingerprintArgs, "--lang=zh-CN")
}

func TestNormalizeConfigDoesNotReintroduceProprietaryFingerprintArgs(t *testing.T) {
	config := &Config{}
	config.Browser.DefaultFingerprintArgs = []string{"--fingerprint=123", "--fingerprint-brand=Chrome"}

	normalizeConfig(config)

	if got, want := len(config.Browser.DefaultFingerprintArgs), 0; got != want {
		t.Fatalf("default fingerprint args length = %d, want %d: %#v", got, want, config.Browser.DefaultFingerprintArgs)
	}
}

func TestNormalizeConfigUpgradesLegacyMinimalLaunchArgs(t *testing.T) {
	config := &Config{}
	config.Browser.DefaultLaunchArgs = []string{"--disable-sync", "--no-first-run"}

	normalizeConfig(config)

	assertStringSliceContains(t, config.Browser.DefaultLaunchArgs, "--disable-non-proxied-udp")
}

func assertStringSliceContains(t *testing.T, values []string, expected string) {
	t.Helper()
	for _, value := range values {
		if value == expected {
			return
		}
	}
	t.Fatalf("values %#v missing %q", values, expected)
}
