package automation

import "regexp"

var (
	scriptRequirePattern     = regexp.MustCompile(`\brequire\s*\(\s*['"]([^'"]+)['"]\s*\)`)
	scriptDynamicImportRegex = regexp.MustCompile(`\bimport\s*\(\s*['"]([^'"]+)['"]\s*\)`)
	scriptImportFromPattern  = regexp.MustCompile(`(?m)\bimport\s+(?:[^'"]*?\s+from\s+)?['"]([^'"]+)['"]`)
	scriptExportFromPattern  = regexp.MustCompile(`(?m)\bexport\s+[^'"]*?\s+from\s+['"]([^'"]+)['"]`)
	nodeBuiltinModules       = map[string]struct{}{
		"assert":              {},
		"async_hooks":         {},
		"buffer":              {},
		"child_process":       {},
		"cluster":             {},
		"console":             {},
		"constants":           {},
		"crypto":              {},
		"dgram":               {},
		"diagnostics_channel": {},
		"dns":                 {},
		"domain":              {},
		"events":              {},
		"fs":                  {},
		"http":                {},
		"http2":               {},
		"https":               {},
		"inspector":           {},
		"module":              {},
		"net":                 {},
		"os":                  {},
		"path":                {},
		"perf_hooks":          {},
		"process":             {},
		"punycode":            {},
		"querystring":         {},
		"readline":            {},
		"repl":                {},
		"stream":              {},
		"string_decoder":      {},
		"sys":                 {},
		"timers":              {},
		"tls":                 {},
		"trace_events":        {},
		"tty":                 {},
		"url":                 {},
		"util":                {},
		"v8":                  {},
		"vm":                  {},
		"wasi":                {},
		"worker_threads":      {},
		"zlib":                {},
	}
	supportedScriptModuleExtensions = map[string]struct{}{
		".js":  {},
		".cjs": {},
		".mjs": {},
	}
	supportedLocalImportExtensions = map[string]struct{}{
		".js":   {},
		".cjs":  {},
		".mjs":  {},
		".json": {},
	}
	// deniedNodeBuiltinModules 为危险内置模块黑名单，无论来源一律拒绝（防止脚本逃逸沙箱/取得 RCE）。
	// 仅拦截能直接执行代码或逃逸进程边界的模块（child_process、vm）；fs/os/net 是合法脚本 I/O，
	// 内置 demo（news-query-txt、web-image-generate-download 等）与 runner 均依赖 fs，故不拦截。
	deniedNodeBuiltinModules = map[string]struct{}{
		"child_process": {},
		"vm":            {},
	}
)

type importedPackageJSON struct {
	Dependencies         map[string]any `json:"dependencies"`
	DevDependencies      map[string]any `json:"devDependencies"`
	PeerDependencies     map[string]any `json:"peerDependencies"`
	OptionalDependencies map[string]any `json:"optionalDependencies"`
}
