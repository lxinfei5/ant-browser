// Package dockicon 为每个浏览器实例（profile）生成定制 Dock 图标的 Chromium .app 克隆。
//
// 背景：macOS 的 Dock 图标取自进程所属 .app bundle（Info.plist 的 CFBundleIconFile +
// Resources/*.icns），与「直接 exec bundle 内二进制」无关；无法运行时按进程定制。
// 因此每个需要定制图标的 profile 克隆一份 .app，改 CFBundleIdentifier/CFBundleName +
// 注入定制 icns，再把启动 exe 指向克隆 bundle 内的可执行文件。
//
// 平台：仅 macOS 真正实现（dockicon_darwin.go）；其它平台为 no-op（dockicon_other.go）。
package dockicon

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"ant-chrome/backend/internal/browser"
	"ant-chrome/backend/internal/logger"
)

// LookupFunc 由 App 层注入：根据 profileId 查其绑定账号的图标设置与显示名。
// 用函数值注入，dockicon 无需 import accountpool，避免循环依赖。
type LookupFunc func(profileId string) (browser.DockIconAccount, bool)

// Resolver 管理 profile 的图标克隆缓存与物化。
// 实现 browser.DockIconResolver。
type Resolver struct {
	stateRoot string
	lookup    LookupFunc

	// mu 保护 locks 表；locks 按 profileId 串行化 Materialize，防并发启动重复克隆。
	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

// NewResolver 创建 Resolver。stateRoot 为应用状态根目录（见 apppath.StateRoot）。
func NewResolver(stateRoot string, lookup LookupFunc) *Resolver {
	return &Resolver{
		stateRoot: stateRoot,
		lookup:    lookup,
		locks:     make(map[string]*sync.Mutex),
	}
}

// ── 缓存布局 ────────────────────────────────────────────────────────────────

func (r *Resolver) iconsRoot() string  { return filepath.Join(r.stateRoot, "chrome-icons") }
func (r *Resolver) mastersRoot() string { return filepath.Join(r.stateRoot, "profile-icons") }

func (r *Resolver) cloneDir(profileId string) string {
	return filepath.Join(r.iconsRoot(), profileId)
}

func (r *Resolver) stampPath(profileId string) string {
	return filepath.Join(r.cloneDir(profileId), ".stamp")
}

// MasterPNGPath 返回某 profile 已持久化主图 PNG 的路径；无则返回空串。
func (r *Resolver) MasterPNGPath(profileId string) string {
	p := filepath.Join(r.mastersRoot(), profileId+".png")
	if st, err := os.Stat(p); err == nil && !st.IsDir() {
		return p
	}
	return ""
}

// SaveMasterPNG 持久化某 profile 的主图 PNG，返回其路径。
func (r *Resolver) SaveMasterPNG(profileId string, png []byte) (string, error) {
	if len(png) == 0 {
		return "", fmt.Errorf("PNG 数据为空")
	}
	if err := os.MkdirAll(r.mastersRoot(), 0o755); err != nil {
		return "", err
	}
	p := filepath.Join(r.mastersRoot(), profileId+".png")
	if err := os.WriteFile(p, png, 0o644); err != nil {
		return "", err
	}
	return p, nil
}

// ── 生命周期 ────────────────────────────────────────────────────────────────

// Invalidate 仅失效缓存（删除 stamp），下次启动重建；保留主图与克隆目录。
func (r *Resolver) Invalidate(profileId string) {
	_ = os.Remove(r.stampPath(profileId))
}

// Remove 删除该 profile 的克隆与主图。
func (r *Resolver) Remove(profileId string) {
	_ = os.RemoveAll(r.cloneDir(profileId))
	_ = os.Remove(filepath.Join(r.mastersRoot(), profileId+".png"))
}

// Sweep 清理不在 validProfileIds 内的孤儿克隆/主图。
func (r *Resolver) Sweep(validProfileIds []string) {
	valid := make(map[string]bool, len(validProfileIds))
	for _, id := range validProfileIds {
		valid[id] = true
	}
	sweepDir(r.iconsRoot(), valid, true)
	sweepDir(r.mastersRoot(), valid, false)
}

func sweepDir(root string, valid map[string]bool, dirEntries bool) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return // 目录不存在则无需清理
	}
	for _, e := range entries {
		name := e.Name()
		id := name
		if !dirEntries {
			id = strings.TrimSuffix(name, ".png")
			if id == name { // 非 .png 文件跳过
				continue
			}
		}
		if !valid[id] {
			_ = os.RemoveAll(filepath.Join(root, name))
		}
	}
}

// RebuildAll 失效全部缓存（清空 stamp），下次启动惰性重建。
func (r *Resolver) RebuildAll() {
	entries, err := os.ReadDir(r.iconsRoot())
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			_ = os.Remove(r.stampPath(e.Name()))
		}
	}
}

// ── 内部工具 ────────────────────────────────────────────────────────────────

// profileLock 返回某 profileId 的互斥锁，串行化其 Materialize。
func (r *Resolver) profileLock(profileId string) *sync.Mutex {
	r.mu.Lock()
	defer r.mu.Unlock()
	l, ok := r.locks[profileId]
	if !ok {
		l = &sync.Mutex{}
		r.locks[profileId] = l
	}
	return l
}

// stamp 内容：sourceExe + pngPath + png 字节哈希 + displayName。任一变化则重建。
func computeStamp(sourceExe, pngPath, displayName string) string {
	h := sha256.New()
	h.Write([]byte(sourceExe))
	h.Write([]byte{0})
	h.Write([]byte(pngPath))
	h.Write([]byte{0})
	if data, err := os.ReadFile(pngPath); err == nil {
		ph := sha256.Sum256(data)
		h.Write(ph[:])
	}
	h.Write([]byte{0})
	h.Write([]byte(displayName))
	return hex.EncodeToString(h.Sum(nil))
}

// readStamp 返回缓存的 stamp 与是否命中（内容一致且文件存在）。
func (r *Resolver) stampHit(profileId, want string) bool {
	data, err := os.ReadFile(r.stampPath(profileId))
	return err == nil && strings.TrimSpace(string(data)) == want
}

func (r *Resolver) writeStamp(profileId, stamp string) error {
	if err := os.MkdirAll(r.cloneDir(profileId), 0o755); err != nil {
		return err
	}
	return os.WriteFile(r.stampPath(profileId), []byte(stamp), 0o644)
}

func log() *logger.Logger { return logger.New("DockIcon") }
