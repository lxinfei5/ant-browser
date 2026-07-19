package proxy

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"ant-chrome/backend/internal/logger"
)

// runtime-manifest.json 中每个受管二进制的校验条目。
type runtimeManifestEntry struct {
	Path   string   `json:"path"`
	Sha256 string   `json:"sha256"`
	Targets []string `json:"targets"`
}

type runtimeManifestFile struct {
	SchemaVersion int                    `json:"schemaVersion"`
	Files         []runtimeManifestEntry  `json:"files"`
}

type runtimeHashCacheEntry struct {
	mtime time.Time
	sha   string
}

var (
	runtimeHashCacheMu sync.Mutex
	runtimeHashCache   = map[string]runtimeHashCacheEntry{}
)

// runtimeBypassEnv 为开发/自建二进制提供显式跳过校验的逃生阀。
const runtimeBypassEnv = "ANTCHROME_ALLOW_UNVERIFIED_RUNTIME"

// runtimeManifestCandidates 返回可能存在的 runtime-manifest.json 路径（按优先级）。
func runtimeManifestCandidates(appRoot string) []string {
	candidates := make([]string, 0, 4)
	if appRoot != "" {
		candidates = append(candidates,
			filepath.Join(appRoot, "publish", "runtime-manifest.json"),
			filepath.Join(appRoot, "runtime-manifest.json"),
		)
	}
	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		candidates = append(candidates,
			filepath.Join(exeDir, "runtime-manifest.json"),
			filepath.Join(exeDir, "publish", "runtime-manifest.json"),
		)
	}
	return candidates
}

func loadRuntimeManifest(appRoot string) (*runtimeManifestFile, string, error) {
	for _, c := range runtimeManifestCandidates(appRoot) {
		data, err := os.ReadFile(c)
		if err != nil {
			continue
		}
		var manifest runtimeManifestFile
		if err := json.Unmarshal(data, &manifest); err != nil {
			return nil, c, fmt.Errorf("解析 runtime-manifest.json 失败: %w", err)
		}
		return &manifest, c, nil
	}
	return nil, "", nil
}

// manifestPathMatches 判断二进制绝对路径是否对应清单条目（按路径段后缀匹配，兼容 appRoot/exeDir 两种放置）。
func manifestPathMatches(binaryPath, entryPath string) bool {
	bp := filepath.ToSlash(binaryPath)
	ep := strings.Trim(filepath.ToSlash(entryPath), "/")
	if bp == ep || strings.HasSuffix(bp, "/"+ep) {
		return true
	}
	return false
}

// cachedFileSha256 计算文件 sha256，按 path+mtime 缓存以避免重复哈希。
func cachedFileSha256(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	mtime := info.ModTime()

	runtimeHashCacheMu.Lock()
	if entry, ok := runtimeHashCache[path]; ok && entry.mtime.Equal(mtime) {
		runtimeHashCacheMu.Unlock()
		return entry.sha, nil
	}
	runtimeHashCacheMu.Unlock()

	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	sum := hex.EncodeToString(h.Sum(nil))

	runtimeHashCacheMu.Lock()
	runtimeHashCache[path] = runtimeHashCacheEntry{mtime: mtime, sha: sum}
	runtimeHashCacheMu.Unlock()
	return sum, nil
}

// verifyRuntimeBinary 在 exec 前校验磁盘上二进制的 sha256 是否与 runtime-manifest.json 的固定条目一致。
// 安全策略：
//   - 命中条目且 sha256 不匹配 -> 拒绝（疑似篡改）。
//   - 命中条目且匹配 -> 通过。
//   - 未随构建发布清单（找不到 manifest）-> 记录警告并放行，标记为未验证（避免砖掉未附带 manifest 的安装）。
//   - 清单存在但该路径无条目 -> 记录警告并放行，标记为未验证（兼容 windows 下载路径/mihomo 等），可通过设置 runtimeBypassEnv 完全跳过。
//   - 设置环境变量 ANTCHROME_ALLOW_UNVERIFIED_RUNTIME=1 -> 完全跳过（开发/自建二进制）。
func verifyRuntimeBinary(binaryPath, appRoot string) error {
	if strings.TrimSpace(os.Getenv(runtimeBypassEnv)) != "" {
		return nil
	}

	manifest, manifestPath, err := loadRuntimeManifest(appRoot)
	if err != nil {
		return fmt.Errorf("加载 runtime-manifest 失败: %w", err)
	}
	if manifest == nil {
		logger.New("ProxyRuntime").Warn("未找到 runtime-manifest.json，跳过二进制 sha256 校验（标记为未验证）",
			logger.F("binary", binaryPath))
		return nil
	}

	var entry *runtimeManifestEntry
	for i := range manifest.Files {
		if manifestPathMatches(binaryPath, manifest.Files[i].Path) {
			entry = &manifest.Files[i]
			break
		}
	}
	if entry == nil || strings.TrimSpace(entry.Sha256) == "" {
		logger.New("ProxyRuntime").Warn("runtime-manifest 无该二进制的校验条目，跳过 sha256 校验（标记为未验证）",
			logger.F("binary", binaryPath),
			logger.F("manifest", manifestPath))
		return nil
	}

	sum, err := cachedFileSha256(binaryPath)
	if err != nil {
		return fmt.Errorf("计算二进制 sha256 失败: %w", err)
	}
	if !strings.EqualFold(sum, entry.Sha256) {
		return fmt.Errorf("二进制 sha256 校验失败：%s 期望 %s 实际 %s（疑似篡改；开发可设置 %s=1 跳过）", binaryPath, entry.Sha256, sum, runtimeBypassEnv)
	}
	return nil
}