//go:build darwin

package dockicon

// TestMaterializeEndToEnd 端到端验证：以系统 Chrome 为源内核，物化一个定制图标的
// 克隆 bundle，断言克隆 exe 生成、CFBundleIdentifier 已改为按 profile 的值、且 codesign
// 通过（改 plist 后必须重签，否则进程起不来）。系统无 Chrome 时跳过。
//
// 运行：go test ./backend/internal/dockicon/ -run TestMaterializeEndToEnd -v
// （仅 macOS 且装有 /Applications/Google Chrome.app 时真正执行，否则 Skip。）

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"ant-chrome/backend/internal/browser"
)

func TestMaterializeEndToEnd(t *testing.T) {
	srcApp := "/Applications/Google Chrome.app"
	srcExe := filepath.Join(srcApp, "Contents", "MacOS", "Google Chrome")
	if _, err := os.Stat(srcExe); err != nil {
		t.Skip("系统无 /Applications/Google Chrome.app，跳过端到端测试")
	}

	stateRoot := t.TempDir()
	resolver := NewResolver(stateRoot, func(profileId string) (browser.DockIconAccount, bool) {
		return browser.DockIconAccount{
			Found:       true,
			IconKind:    "text",
			IconColor:   "#3b82f6",
			IconText:    "A",
			DisplayName: "AcctA",
		}, true
	})

	// 造一个最小合法 PNG 作为主图（用 sips 从系统 Chrome 图标导出，保证真实 PNG）。
	pngPath := filepath.Join(stateRoot, "profile-icons", "p1.png")
	if err := os.MkdirAll(filepath.Dir(pngPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("sips", "-s", "format", "png",
		filepath.Join(srcApp, "Contents", "Resources", "app.icns"), "--out", pngPath).CombinedOutput(); err != nil {
		t.Fatalf("生成测试 PNG 失败: %v: %s", err, out)
	}

	exe, err := resolver.Materialize("p1", srcExe, pngPath, "AcctA")
	if err != nil {
		t.Fatalf("Materialize 返回错误: %v", err)
	}
	if exe == srcExe {
		t.Fatal("Materialize 回退到了原 exe（应当产出克隆 exe）")
	}
	if !strings.Contains(exe, "chrome-icons") {
		t.Fatalf("克隆 exe 路径不在 chrome-icons 缓存内: %s", exe)
	}
	if _, err := os.Stat(exe); err != nil {
		t.Fatalf("克隆 exe 不存在: %v", err)
	}

	// 校验 bundle id 已改。
	dstApp := filepath.Join(resolver.cloneDir("p1"), "Chromium.app")
	idOut, err := exec.Command("/usr/libexec/PlistBuddy", "-c", "Print :CFBundleIdentifier",
		filepath.Join(dstApp, "Contents", "Info.plist")).Output()
	if err != nil {
		t.Fatalf("读 CFBundleIdentifier 失败: %v", err)
	}
	if got := strings.TrimSpace(string(idOut)); got != "com.antbrowser.profile.p1" {
		t.Fatalf("CFBundleIdentifier = %q, 期望 com.antbrowser.profile.p1", got)
	}

	// 校验重签有效（改 plist 后必须重签才能启动）。
	if out, err := exec.Command("codesign", "--verify", "--strict", dstApp).CombinedOutput(); err != nil {
		t.Fatalf("克隆 bundle codesign 校验失败（进程将无法启动）: %v: %s", err, out)
	}

	// 缓存命中：再次 Materialize 应复用同一 exe（stamp 命中）。
	exe2, err := resolver.Materialize("p1", srcExe, pngPath, "AcctA")
	if err != nil || exe2 != exe {
		t.Fatalf("缓存命中失败: exe2=%s err=%v", exe2, err)
	}

	// 主图变更 → stamp 失效 → 重建（这里仅验证 Invalidate 后仍能产出）。
	resolver.Invalidate("p1")
	if _, err := resolver.Materialize("p1", srcExe, pngPath, "AcctA"); err != nil {
		t.Fatalf("Invalidate 后重建失败: %v", err)
	}

	// Remove 清理。
	resolver.Remove("p1")
	if _, err := os.Stat(resolver.cloneDir("p1")); !os.IsNotExist(err) {
		t.Fatalf("Remove 后克隆目录仍存在")
	}
}
