package backend

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ant-chrome/backend/internal/logger"
)

// proxyCoreSourcesEntry 对应 publish/runtime-sources.json 中的固定上游归档来源。
type proxyCoreSourcesEntry struct {
	ID             string `json:"id"`
	Target         string `json:"target"`
	Runtime        string `json:"runtime"`
	Version        string `json:"version"`
	ArchiveType    string `json:"archiveType"`
	URL            string `json:"url"`
	ArchiveSha256  string `json:"archiveSha256"`
	ArchiveBinaryPath string `json:"archiveBinaryPath"`
	DestPath       string `json:"destPath"`
}

type proxyCoreSourcesFile struct {
	SchemaVersion int                    `json:"schemaVersion"`
	Sources       []proxyCoreSourcesEntry `json:"sources"`
}

// proxyCoreAssetHostAllowlist 限定代理内核归档只能来自 GitHub 官方分发主机，且必须 https。
var proxyCoreAssetHostAllowlist = map[string]bool{
	"github.com":                true,
	"objects.githubusercontent.com": true,
	"api.github.com":            true,
}

// validateProxyCoreAssetURL 校验下载/资源 URL 必须为 https 且主机在官方白名单内，防止被引向第三方主机。
func validateProxyCoreAssetURL(rawURL string) error {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return fmt.Errorf("URL 解析失败: %w", err)
	}
	if strings.ToLower(u.Scheme) != "https" {
		return fmt.Errorf("仅允许 https 下载（拒绝 %s）", rawURL)
	}
	host := strings.ToLower(strings.Trim(u.Host, "[]"))
	if host == "" || !proxyCoreAssetHostAllowlist[host] {
		return fmt.Errorf("下载主机不在官方白名单内: %s", host)
	}
	return nil
}

// loadProxyCoreSources 从 appRoot/publish/runtime-sources.json 加载固定上游来源（找不到则返回 nil）。
func loadProxyCoreSources(appRoot string) (*proxyCoreSourcesFile, string) {
	candidates := []string{
		filepath.Join(appRoot, "publish", "runtime-sources.json"),
		filepath.Join(appRoot, "runtime-sources.json"),
	}
	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		candidates = append(candidates,
			filepath.Join(exeDir, "runtime-sources.json"),
			filepath.Join(exeDir, "publish", "runtime-sources.json"),
		)
	}
	for _, c := range candidates {
		data, err := os.ReadFile(c)
		if err != nil {
			continue
		}
		var file proxyCoreSourcesFile
		if err := json.Unmarshal(data, &file); err != nil {
			logger.New("ProxyCore").Warn("解析 runtime-sources.json 失败", logger.F("path", c), logger.F("error", err.Error()))
			continue
		}
		return &file, c
	}
	return nil, ""
}

// assetDigestSha256 从 GitHub API 返回的 digest 字段中提取 sha256 十六进制串（形如 "sha256:<hex>"）。
func assetDigestSha256(digest string) string {
	digest = strings.TrimSpace(digest)
	if digest == "" {
		return ""
	}
	if idx := strings.Index(digest, ":"); idx >= 0 {
		digest = strings.TrimSpace(digest[idx+1:])
	}
	if isHexSha256(digest) {
		return strings.ToLower(digest)
	}
	return ""
}

// verifyProxyCoreArchiveByHash 校验磁盘归档 sha256 等于给定值（来自官方校验文件或 API digest）。
func verifyProxyCoreArchiveByHash(archivePath, expectedSha256 string) error {
	expectedSha256 = strings.TrimSpace(expectedSha256)
	if expectedSha256 == "" {
		return fmt.Errorf("缺少期望的 sha256")
	}
	f, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("打开归档失败: %w", err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("计算归档 sha256 失败: %w", err)
	}
	sum := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(sum, expectedSha256) {
		return fmt.Errorf("归档 sha256 校验失败：期望 %s 实际 %s", expectedSha256, sum)
	}
	return nil
}

// pinnedArchiveSha256 返回与 assetURL 匹配的固定 archiveSha256（无匹配返回空串）。
func pinnedArchiveSha256(appRoot, assetURL string) string {
	file, _ := loadProxyCoreSources(appRoot)
	if file == nil {
		return ""
	}
	for _, src := range file.Sources {
		if strings.EqualFold(strings.TrimSpace(src.URL), strings.TrimSpace(assetURL)) {
			return strings.TrimSpace(src.ArchiveSha256)
		}
	}
	return ""
}

// verifyProxyCoreArchive 在解压前校验已下载归档的 sha256。
// 若该归档 URL 在 runtime-sources.json 中有固定 archiveSha256，则必须匹配；否则记录“未验证”警告并放行。
func verifyProxyCoreArchive(appRoot, archivePath, assetURL string) error {
	pinned := pinnedArchiveSha256(appRoot, assetURL)
	if pinned == "" {
		logger.New("ProxyCore").Warn("归档未在 runtime-sources.json 中固定 sha256，跳过校验（标记为未验证）",
			logger.F("url", assetURL),
			logger.F("archive", archivePath))
		return nil
	}
	f, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("打开归档失败: %w", err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("计算归档 sha256 失败: %w", err)
	}
	sum := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(sum, pinned) {
		return fmt.Errorf("归档 sha256 校验失败：%s 期望 %s 实际 %s", assetURL, pinned, sum)
	}
	logger.New("ProxyCore").Info("归档 sha256 校验通过", logger.F("url", assetURL), logger.F("sha256", sum))
	return nil
}

// fetchProxyCoreOfficialChecksum 尝试从同一 release 拉取官方校验文件并匹配资产 sha256。
// 用于版本未固定时的额外校验；找不到/无法解析时返回空串（调用方按“未验证”处理）。
func fetchProxyCoreOfficialChecksum(ctx context.Context, client *http.Client, spec proxyCoreSpec, release githubRelease, assetName string, timeout time.Duration) string {
	// 选取同 release 内的校验类资产（.dgst/.sha256/.sha256sums/checksums）。
	checksumAssets := make([]githubReleaseAsset, 0)
	for _, a := range release.Assets {
		name := strings.ToLower(a.Name)
		if strings.HasSuffix(name, ".dgst") || strings.HasSuffix(name, ".sha256") || strings.HasSuffix(name, ".sha256sums") || strings.HasSuffix(name, ".sha256sum") || strings.Contains(name, "checksum") {
			checksumAssets = append(checksumAssets, a)
		}
	}
	for _, ca := range checksumAssets {
		if err := validateProxyCoreAssetURL(ca.BrowserDownloadURL); err != nil {
			continue
		}
		data, err := fetchProxyCoreBytes(ctx, client, ca.BrowserDownloadURL, timeout)
		if err != nil {
			continue
		}
		if sum := parseChecksumForAsset(string(data), assetName); sum != "" {
			return sum
		}
	}
	return ""
}

func fetchProxyCoreBytes(ctx context.Context, client *http.Client, rawURL string, timeout time.Duration) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "ant-chrome-proxy-core-downloader")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 8<<20))
}

// parseChecksumForAsset 从校验文件内容中解析出与 assetName 对应的 sha256（兼容 `<sha>  <name>` 与 `<sha> <name>` 形式）。
func parseChecksumForAsset(content, assetName string) string {
	want := strings.ToLower(assetName)
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		sum := fields[0]
		name := strings.ToLower(filepath.Base(fields[len(fields)-1]))
		if name == want && isHexSha256(sum) {
			return strings.ToLower(sum)
		}
	}
	return ""
}

func isHexSha256(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}