//go:build darwin

package dockicon

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"ant-chrome/backend/internal/logger"
)

// Materialize 返回该 profile 定制图标克隆 bundle 内的可执行文件路径。
// 缓存命中（stamp 一致且克隆 exe 存在）直接复用；否则克隆源 .app、改 plist、
// 注入定制 icns、清 AppleDouble/xattr、adhoc 重签。任何失败返回 sourceExe（降级，不阻断启动）。
//
// 所有外部命令一律 exec.Command 直接调用（不经 shell）：本机 TCC 会拦截 shell 间接
// posix_spawn，但直接 exec 正常（已实测）。
func (r *Resolver) Materialize(profileId, sourceExe, pngPath, displayName string) (string, error) {
	lock := r.profileLock(profileId)
	lock.Lock()
	defer lock.Unlock()

	if pngPath == "" {
		return sourceExe, nil
	}
	if _, err := os.Stat(sourceExe); err != nil {
		return sourceExe, fmt.Errorf("源内核不存在: %w", err)
	}

	srcApp, innerRel, err := locateAppBundle(sourceExe)
	if err != nil {
		log().Warn("无法定位源 .app bundle，回退原内核", logger.F("source", sourceExe), logger.F("error", err.Error()))
		return sourceExe, nil
	}

	dstApp := filepath.Join(r.cloneDir(profileId), "Chromium.app")
	dstExe := filepath.Join(dstApp, innerRel)
	stamp := computeStamp(sourceExe, pngPath, displayName)

	// 缓存命中：stamp 一致且克隆 exe 存在。
	if r.stampHit(profileId, stamp) {
		if _, err := os.Stat(dstExe); err == nil {
			return dstExe, nil
		}
	}

	if err := r.buildClone(profileId, srcApp, dstApp, innerRel, pngPath, displayName); err != nil {
		log().Warn("Dock 图标克隆构建失败，回退原内核",
			logger.F("profile_id", profileId),
			logger.F("error", err.Error()),
		)
		return sourceExe, nil
	}

	if _, err := os.Stat(dstExe); err != nil {
		return sourceExe, nil
	}
	if err := r.writeStamp(profileId, stamp); err != nil {
		log().Warn("写入 stamp 失败", logger.F("error", err.Error()))
	}
	log().Info("Dock 图标克隆已就绪",
		logger.F("profile_id", profileId),
		logger.F("bundle", dstApp),
	)
	return dstExe, nil
}

// locateAppBundle 从内核 exe 路径向上找到所属 .app，返回 bundle 根与 exe 相对 bundle 的路径。
func locateAppBundle(exe string) (appRoot, innerRel string, err error) {
	dir := filepath.Dir(exe)
	for {
		if strings.HasSuffix(dir, ".app") {
			rel, rerr := filepath.Rel(dir, exe)
			if rerr != nil {
				return "", "", rerr
			}
			return dir, rel, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", "", fmt.Errorf("exe 不在任何 .app 内: %s", exe)
		}
		dir = parent
	}
}

func (r *Resolver) buildClone(profileId, srcApp, dstApp, innerRel, pngPath, displayName string) error {
	// 清掉旧克隆，整体重建（成本低，逻辑简单可靠）。
	_ = os.RemoveAll(dstApp)
	if err := os.MkdirAll(r.cloneDir(profileId), 0o755); err != nil {
		return err
	}

	// 1) 克隆 .app：优先 APFS 写时复制（cp -c），失败回退普通复制（cp -R，非 APFS 卷）。
	if err := run("cp", "-cR", srcApp, dstApp); err != nil {
		if err2 := run("cp", "-R", srcApp, dstApp); err2 != nil {
			return fmt.Errorf("克隆 .app 失败: cp -cR: %v; cp -R: %v", err, err2)
		}
	}

	plist := filepath.Join(dstApp, "Contents", "Info.plist")

	// 2) 改 bundle 标识与显示名。
	bundleID := fmt.Sprintf("com.antbrowser.profile.%s", sanitizeBundleID(profileId))
	if err := plistSet(plist, "CFBundleIdentifier", bundleID); err != nil {
		return fmt.Errorf("改 CFBundleIdentifier 失败: %w", err)
	}
	if displayName != "" {
		_ = plistSet(plist, "CFBundleName", displayName)
		_ = plistSet(plist, "CFBundleDisplayName", displayName)
	}

	// 3) 注入定制 icns。
	if err := r.injectIcon(dstApp, plist, pngPath); err != nil {
		return fmt.Errorf("注入图标失败: %w", err)
	}

	// 4) 清 AppleDouble 文件 + xattr + provenance，否则 codesign 报 detritus 错误。
	removeAppleDouble(dstApp)
	_ = run("xattr", "-cr", dstApp)
	stripProvenance(dstApp)

	// 5) adhoc 重签：改 plist 后必须重签，否则 RunningBoard 拒绝 spawn（进程起不来）。
	//    用 shallow 重签（只签最外层，不加 --deep）：改的是外层 Info.plist，而 --deep 会把
	//    内嵌的合法 Framework 重签成 "invalid version"（已实测 shallow 通过 codesign --verify
	//    --strict 且能正常启动）。失败仅警告继续——无有效签名的 Chromium 快照可容忍。
	if err := run("codesign", "--force", "--sign", "-", dstApp); err != nil {
		log().Warn("adhoc 重签失败（继续，部分内核可容忍）", logger.F("error", err.Error()))
	}
	return nil
}

// injectIcon 由 PNG 生成 iconset → icns，替换克隆 bundle 的图标资源。
func (r *Resolver) injectIcon(dstApp, plist, pngPath string) error {
	iconName, _ := plistPrint(plist, "CFBundleIconFile")
	if iconName == "" {
		iconName = "app"
	}
	iconName = strings.TrimSuffix(iconName, ".icns")

	iconset, err := os.MkdirTemp("", "dockicon-*.iconset")
	if err != nil {
		return err
	}
	defer os.RemoveAll(iconset)

	// 标准 iconset 尺寸（含 @2x）。
	type size struct {
		name string
		px   string
	}
	sizes := []size{
		{"icon_16x16.png", "16"}, {"icon_16x16@2x.png", "32"},
		{"icon_32x32.png", "32"}, {"icon_32x32@2x.png", "64"},
		{"icon_128x128.png", "128"}, {"icon_128x128@2x.png", "256"},
		{"icon_256x256.png", "256"}, {"icon_256x256@2x.png", "512"},
		{"icon_512x512.png", "512"}, {"icon_512x512@2x.png", "1024"},
	}
	for _, s := range sizes {
		out := filepath.Join(iconset, s.name)
		if err := run("sips", "-z", s.px, s.px, pngPath, "--out", out); err != nil {
			return fmt.Errorf("sips 缩放 %s 失败: %w", s.px, err)
		}
	}

	// 生成到临时文件再落进 bundle，避免半写状态。
	tmpIcns := filepath.Join(os.TempDir(), fmt.Sprintf("dockicon-%d.icns", os.Getpid()))
	defer os.Remove(tmpIcns)
	if err := run("iconutil", "-c", "icns", iconset, "-o", tmpIcns); err != nil {
		return fmt.Errorf("iconutil 生成 icns 失败: %w", err)
	}
	dstIcns := filepath.Join(dstApp, "Contents", "Resources", iconName+".icns")
	if err := os.Rename(tmpIcns, dstIcns); err != nil {
		// 跨设备时回退复制。
		if err2 := run("cp", tmpIcns, dstIcns); err2 != nil {
			return fmt.Errorf("写入 icns 失败: %v / %v", err, err2)
		}
	}
	return nil
}

// removeAppleDouble 删除 ._* AppleDouble 文件（cp 在特定文件系统上会产生；codesign 拒绝之）。
func removeAppleDouble(root string) {
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasPrefix(info.Name(), "._") {
			_ = os.Remove(path)
		}
		return nil
	})
}

// stripProvenance 删除 com.apple.provenance xattr：cp -cR 会把它带进克隆体，而它
// 无法用 xattr -cr 清除，会让 codesign 报「resource fork / detritus not allowed」。
// 须逐文件 xattr -d（已实测）。在 codesign 前对每个文件调用。
func stripProvenance(root string) {
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			_ = run("xattr", "-d", "com.apple.provenance", path)
		}
		return nil
	})
}

// sanitizeBundleID 把 profileId 收敛为合法 bundle id 片段（小写字母/数字/点/连字符）。
func sanitizeBundleID(id string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(id) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '.':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	s := b.String()
	if s == "" {
		s = "x"
	}
	return s
}

// plistSet 设置（不存在则新增）一个字符串键。
func plistSet(plist, key, val string) error {
	if err := run("/usr/libexec/PlistBuddy", "-c", fmt.Sprintf("Set :%s %s", key, val), plist); err != nil {
		return run("/usr/libexec/PlistBuddy", "-c", fmt.Sprintf("Add :%s string %s", key, val), plist)
	}
	return nil
}

func plistPrint(plist, key string) (string, error) {
	out, err := exec.Command("/usr/libexec/PlistBuddy", "-c", fmt.Sprintf("Print :%s", key), plist).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// run 直接执行命令（不经 shell），返回带 stderr 的错误。
func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s %s: %v: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}
