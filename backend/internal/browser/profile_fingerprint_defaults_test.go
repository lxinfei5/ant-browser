package browser

import "testing"

func TestUpgradeLegacyMinimalFingerprintArgsStripsProprietaryArgs(t *testing.T) {
	args := upgradeLegacyMinimalFingerprintArgs([]string{
		"--fingerprint-brand=Chrome",
		"--fingerprint-platform=windows",
		"--fingerprinting-canvas-image-data-noise",
		"--lang=zh-CN",
	})

	if got, want := len(args), 1; got != want {
		t.Fatalf("fingerprint args length = %d, want %d: %#v", got, want, args)
	}
	assertStringSliceContains(t, args, "--lang=zh-CN")
}

func TestUpgradeLegacyMinimalFingerprintArgsStripsSeedArgs(t *testing.T) {
	args := upgradeLegacyMinimalFingerprintArgs([]string{"--fingerprint=123", "--fingerprint-brand=Chrome", "--window-size=1280,720"})

	if got, want := len(args), 1; got != want {
		t.Fatalf("fingerprint args length = %d, want %d: %#v", got, want, args)
	}
	assertStringSliceContains(t, args, "--window-size=1280,720")
}
