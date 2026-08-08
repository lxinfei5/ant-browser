package apppath

import (
	"os"
	"path/filepath"
	"testing"
)

// TestSeedPath_PrefersResourcesOverMacOS 锁定种子文件定位契约：
// 新版 mac 发布包把 config.yaml / chrome 等种子放进 Contents/Resources(避免 codesign
// 把 Contents/MacOS 下的非可执行文件当嵌套代码对象签名,导致 zip 解压后 seal 失效);
// seedPath 必须优先返回 Resources 路径,缺失时回退到 installRoot(即 Contents/MacOS),
// 以兼容旧布局与开发态。
func TestSeedPath_PrefersResourcesOverMacOS(t *testing.T) {
	// 构造一个仿 .app 的目录: installRoot = <bundle>/Contents/MacOS
	bundle := t.TempDir()
	installRoot := filepath.Join(bundle, "Contents", "MacOS")
	resources := filepath.Join(bundle, "Contents", "Resources")
	if err := os.MkdirAll(installRoot, 0o755); err != nil {
		t.Fatalf("mkdir MacOS: %v", err)
	}
	if err := os.MkdirAll(resources, 0o755); err != nil {
		t.Fatalf("mkdir Resources: %v", err)
	}

	// 1) 两处都不存在 -> 回退 installRoot(MacOS) 路径
	if got := seedPath(installRoot, "config.yaml"); got != filepath.Join(installRoot, "config.yaml") {
		t.Fatalf("无种子时应回退 MacOS 路径, got %q", got)
	}

	// 2) 仅 MacOS 存在(旧布局) -> 返回 MacOS 路径
	macosCfg := filepath.Join(installRoot, "config.yaml")
	if err := os.WriteFile(macosCfg, []byte("macos"), 0o644); err != nil {
		t.Fatalf("write macos cfg: %v", err)
	}
	if got := seedPath(installRoot, "config.yaml"); got != macosCfg {
		t.Fatalf("仅 MacOS 存在时应返回 MacOS 路径, got %q", got)
	}

	// 3) Resources 也存在(新布局) -> 优先 Resources 路径
	resCfg := filepath.Join(resources, "config.yaml")
	if err := os.WriteFile(resCfg, []byte("resources"), 0o644); err != nil {
		t.Fatalf("write resources cfg: %v", err)
	}
	want := filepath.Clean(resCfg)
	if got := seedPath(installRoot, "config.yaml"); got != want {
		t.Fatalf("Resources 存在时应优先返回 Resources 路径, got %q, want %q", got, want)
	}

	// 4) 目录种子(chrome)同样优先 Resources
	resChrome := filepath.Join(resources, "chrome")
	if err := os.MkdirAll(resChrome, 0o755); err != nil {
		t.Fatalf("mkdir resources chrome: %v", err)
	}
	if got := seedPath(installRoot, "chrome"); got != filepath.Clean(resChrome) {
		t.Fatalf("chrome 目录种子应优先 Resources, got %q", got)
	}
}

// TestSeedPath_NoBundleLayout 非 .app 布局(开发态,installRoot 旁无 Resources)时回退 installRoot。
func TestSeedPath_NoBundleLayout(t *testing.T) {
	installRoot := t.TempDir() // 普通目录,无 ../Resources
	cfg := filepath.Join(installRoot, "config.yaml")
	if err := os.WriteFile(cfg, []byte("x"), 0o644); err != nil {
		t.Fatalf("write cfg: %v", err)
	}
	if got := seedPath(installRoot, "config.yaml"); got != cfg {
		t.Fatalf("开发态应回退 installRoot, got %q", got)
	}
}
