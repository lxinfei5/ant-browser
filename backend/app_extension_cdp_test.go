package backend

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsBenignExtensionLoadError(t *testing.T) {
	t.Parallel()
	cases := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{errors.New("CDP 错误: could not load manifest"), false},
		{errors.New("Extensions.loadUnpacked wasn't found"), false},
		{errors.New("CDP 错误: already loaded"), true},
		{fmt.Errorf("duplicate extension"), true},
		{errors.New("Already installed"), true},
	}
	for _, tc := range cases {
		if got := isBenignExtensionLoadError(tc.err); got != tc.want {
			t.Fatalf("isBenignExtensionLoadError(%v) = %v, want %v", tc.err, got, tc.want)
		}
	}
}

func TestLoadUnpackedExtensionsViaCDPNoop(t *testing.T) {
	t.Parallel()
	if err := loadUnpackedExtensionsViaCDP(0, []string{"/tmp/ext"}); err != nil {
		t.Fatalf("zero port: %v", err)
	}
	if err := loadUnpackedExtensionsViaCDP(9222, nil); err != nil {
		t.Fatalf("empty dirs: %v", err)
	}
}

func TestBuildBrowserLaunchArgsAddsUnsafeExtensionDebugging(t *testing.T) {
	t.Parallel()
	args := buildBrowserLaunchArgs("profile-dir", 9222, "direct://", []string{"/tmp/ext-a", "/tmp/ext-b"}, nil, nil, nil, nil, false)
	want := []string{
		"--enable-unsafe-extension-debugging",
		"--load-extension=/tmp/ext-a,/tmp/ext-b",
	}
	if containsArgPrefix(args, "--disable-extensions-except") {
		t.Fatalf("args = %#v, did not want --disable-extensions-except", args)
	}
	for _, flag := range want {
		if !containsExactArg(args, flag) {
			t.Fatalf("args = %#v, want %s", args, flag)
		}
	}
}

func TestBuildBrowserLaunchArgsOmitsExtensionFlagsWithoutDirs(t *testing.T) {
	t.Parallel()
	args := buildBrowserLaunchArgs("profile-dir", 9222, "direct://", nil, nil, nil, nil, nil, false)
	for _, prefix := range []string{"--enable-unsafe-extension-debugging", "--load-extension", "--disable-extensions-except"} {
		if containsArgPrefix(args, prefix) {
			t.Fatalf("args = %#v, did not want %s", args, prefix)
		}
	}
}

func TestWriteExtensionDeveloperMode(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := writeExtensionDeveloperMode(dir); err != nil {
		t.Fatal(err)
	}
	if err := writeExtensionDeveloperMode(dir); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "Default", "Preferences"))
	if err != nil {
		t.Fatal(err)
	}
	var prefs map[string]any
	if err := json.Unmarshal(data, &prefs); err != nil {
		t.Fatal(err)
	}
	extensions, _ := prefs["extensions"].(map[string]any)
	ui, _ := extensions["ui"].(map[string]any)
	enabled, _ := ui["developer_mode"].(bool)
	if !enabled {
		t.Fatalf("prefs = %s, want developer_mode true", data)
	}
}

func TestSanitizeManagedLaunchArgsStripsUnsafeExtensionDebugging(t *testing.T) {
	t.Parallel()
	sanitized, removed := sanitizeManagedLaunchArgs([]string{
		"--disable-sync",
		"--enable-unsafe-extension-debugging",
		"--load-extension=/tmp/evil",
	})
	if !containsExactArg(sanitized, "--disable-sync") {
		t.Fatalf("sanitized = %#v, want --disable-sync kept", sanitized)
	}
	if containsArgPrefix(sanitized, "--enable-unsafe-extension-debugging") || containsArgPrefix(sanitized, "--load-extension") {
		t.Fatalf("sanitized = %#v, want managed extension flags stripped", sanitized)
	}
	if !containsExactArg(removed, "--enable-unsafe-extension-debugging") || !containsExactArg(removed, "--load-extension") {
		t.Fatalf("removed = %#v, want managed extension flags recorded", removed)
	}
}

func containsExactArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

func containsArgPrefix(args []string, prefix string) bool {
	for _, arg := range args {
		if arg == prefix || strings.HasPrefix(arg, prefix+"=") {
			return true
		}
	}
	return false
}
