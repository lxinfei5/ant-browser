# Phase 0 基线 (Baseline)

本文档基于上游 Ant-Browser V1.2.0 的静态代码审计，记录 fork 分支起点的关键事实。所有事实均来自代码审计，缺失项标注 `待确认 (runtime)`，留待运行时/GUI 验证。

---

## 仓库与分支

- Fork 仓库: `github.com/lxinfei5/Ant-Browser`
- 上游 (upstream): Ant-Browser 原项目（基线 tag `V1.2.0`）
- 基线 tag: `V1.2.0`
- 分支模型:
  - `develop` — fork 日常开发分支
  - `upstream-sync` — 跟踪上游的同步分支
  - `master` — 主干
- 同步流程: `fetch upstream → upstream-sync → develop`（先在 `upstream-sync` 上合并上游，再合入 `develop`，避免污染 `develop`）
- Remotes（已配置）:
  - `origin` → `git@github.com:lxinfei5/Ant-Browser.git`（fork，push/pull 主干）
  - `upstream` → `https://github.com/black-ant/Ant-Browser.git`（只 fetch，不 push）
  - 校验: `git remote -v`

---

## 工具链版本

| 组件 | 版本 | 来源 |
|---|---|---|
| Go | `1.22.0` | `go.mod:3` (`go 1.22.0`) |
| Wails | `v2.12.0` | `go.mod:12` (`github.com/wailsapp/wails/v2 v2.12.0`) |
| Wails CLI | 需单独安装（见下） | `wails.json` 无版本字段 |
| 前端框架 | React `18.2`, react-router-dom `6.20`, zustand `4.4.7`, recharts `2.10` | `frontend/package.json` |
| 构建工具 | Vite `5.0.8`, TypeScript `5.3.3`, tailwindcss `3.3.6`, @vitejs/plugin-react-swc `4.3.0` | `frontend/package.json` |
| Node (自动化运行时) | `22.15.1` | `config.yaml` automation 段（playwright runtime） |
| Playwright-core | `1.59.0` | `config.yaml` automation 段 |
| SQLite 驱动 | `modernc.org/sqlite v1.28.0`（纯 Go，无 CGo） | `go.mod:1` |

`wails.json` 摘要: `name` = `Ant Browser`，`outputfilename` = `ant-chrome`，`productVersion` = `1.3.0`，`wailsjsdir` = `./frontend/src`（`wails.json:3,5,15`）。前端 `frontend/package.json` name = `ant-browser-frontend` v1.3.0。

---

## 启动与开发

> 前置: 必须先安装 Wails CLI（`go install github.com/wailsapp/wails/v2/cmd/wails@v2.12.0`），否则 `wails dev` 不可用。开发命令是 `wails dev`，不是 `go run`。

### macOS / Linux (`dev.sh`)

用法: `./dev.sh [stable|live|help]`（`dev.sh:103,132`）

- **stable (默认)**: 先 `cd frontend && npm install && npm run build:clean`，然后:
  ```
  exec wails dev -m -nogorebuild -noreload -s -skipbindings -assetdir frontend/dist -devserver "$WAILS_DEVSERVER_ADDRESS"
  ```
- **live**: 启动前端 Vite 开发服务器（`npm run dev:raw -- --host 127.0.0.1 --port $frontend_port`，`FRONTEND_PORT` 默认 `5218`），然后:
  ```
  exec wails dev -m -s -skipbindings -frontenddevserverurl "http://127.0.0.1:$frontend_port" -viteservertimeout 60 -devserver "$WAILS_DEVSERVER_ADDRESS"
  ```
- `WAILS_DEVSERVER_ADDRESS` 自动从 `WAILS_DEVSERVER_PORT`（默认 `34115`）起在 `127.0.0.1` 上找第一个空闲端口（`dev.sh:34`）。

### Windows (`bat/dev.bat`)

用法: `bat\dev.bat [stable|live|limited] [--no-pause]`（`bat/dev.bat:102,174`）

- **stable (默认)**: `go mod download` → `npm install`（无 node_modules 时）→ `bat\generate-bindings.bat` → `npm run build`，然后:
  ```
  wails dev -m -nogorebuild -noreload -s -skipbindings -assetdir frontend/dist
  ```
- **live / limited**: 启动前端 watcher（`scripts/dev-watcher.mjs`，或受内存限制的 `run-limited-frontend-dev.ps1`），经 `frontend/scripts/dev-port-helper.mjs` 解析空闲前端端口（首选 `5218`，`bat/dev.bat:23`），等待其监听后:
  ```
  wails dev -m -s -skipbindings -frontenddevserverurl http://127.0.0.1:%FRONTEND_PORT% -viteservertimeout 60
  ```
- Windows 版 **无** `-devserver` 标志。

---

## 内核 (Kernel) 管理

### 模型与持久化

内核在内部称为 **Core**，结构 `config.BrowserCore`（`backend/internal/config/config.go:128`）：

| 字段 | JSON | 说明 |
|---|---|---|
| `CoreId` | `coreId` | UUID 主键 |
| `CoreName` | `coreName` | 人类可读标签 |
| `CorePath` | `corePath` | 含浏览器可执行文件的目录（可相对 AppRoot） |
| `IsDefault` | `isDefault` | 是否默认内核（至多一个为 true） |

注意: **不存储 version 字段**；版本在运行时由内核目录的 `manifest.json` 或 `*.manifest` 文件名派生（`backend/internal/browser/core_info.go:20`，例如 `142.0.7444.175.manifest` → `142.0.7444.175`）。

持久化于 SQLite 表 `browser_cores`（`backend/internal/database/sqlite.go:66`，列: `core_id` PK, `core_name`, `core_path`, `is_default` INTEGER 0/1, `sort_order`, `created_at`）。当 `CoreDAO` 为 nil 时回退到 `config.yaml` 的 `cores:` 列表。

### 目录布局约定

启动时 `ensureDefaultCores()` 调用 `scanChromeDir` 扫描内核根目录（`backend/app_utils.go:179,218`）：

- 内核根: `config.Browser.CoreRoot`（`yaml core_root`，`backend/internal/config/config.go:122`），未设则默认字面量 `chrome`（`backend/app_utils.go:122`）。
- 若 `chrome/` 根本身含可执行文件 → 注册单个 `default` 内核指向根。
- 否则扫描 `chrome/` 的每个直接子目录；包含浏览器可执行文件的子目录注册为内核，`CoreId = core-<dirname>`，`CoreName = Chrome <dirname>`，`CorePath = chrome/<dirname>`。

可执行文件候选名（`CoreExecutableCandidates()`，`backend/internal/browser/core_binary.go:11`）——这是平台差异的关键：

| OS | 候选名 |
|---|---|
| Windows | `chrome.exe` |
| macOS (darwin) | `Google Chrome.app/Contents/MacOS/Google Chrome`、`Chromium.app/Contents/MacOS/Chromium`、`chrome` |
| Linux | `chrome`、`chrome-bin`、`chromium`、`chromium-browser`、`ungoogled-chromium`、`chrome.exe` |

README 记载的路径约定（`README.md:274`）：
```
chrome/
  chrom-142/
    chrome.exe        # Windows
```
macOS 等价: `chrome/chrom-142/Google Chrome.app/Contents/MacOS/Google Chrome`（或 `chrome/chrom-142/chrome`）；Linux: `chrome/chrom-142/chrome`。

`FindCoreExecutable` 先浅搜索（直接路径、.app bundle、候选名），再递归至多深度 5。`chrome/README.md` 说明: 仓库内 `chrome/` 仅占位；实际内核二进制由用户管理，放在运行时 state root 下可写的 `chrome/` 目录。

### fingerprint-chromium 接入

README 推荐使用 [fingerprint-chromium](https://github.com/adryfish/fingerprint-chromium) 作为内核（`README.md:11`）。接入方式: 将某 Core 的 `CorePath` 指向包含 fingerprint-chromium 构建的目录（沿用同样的 `chrome`/`chrome.exe`/`.app` 命名），无需改代码——启动器的 flag 接口已匹配。

关键: 启动器**无条件**注入 fingerprint-chromium 风格的 `--fingerprint=<seed>` 与 `--fingerprint-brand`/`--fingerprint-platform` flag（见下节“实例模型”）。若把 Core 指向 stock Google Chrome，Chrome 会在启动时因未知 flag 报错——即启动器**假定 fingerprint-chromium**。

### config.yaml 的角色

`config.yaml`（仓库根）的 `browser` 段控制全局默认（`config.yaml:24-34`）: `user_data_root`、`default_fingerprint_args`、`default_launch_args`、`default_start_urls`、`light_start_enabled`、`restore_last_session`、`start_ready_timeout_ms`（3000）、`start_stable_window_ms`（1200）、`default_connector_type`（xray）。**Cores 默认不在 `config.yaml` 中**（由 SQLite/内核管理 UI 管理），`CoreRoot` 未设故使用 `chrome/` 目录。

---

## 实例 (Instance) 模型

“Instance” 在内部是 Go 结构体 `Profile`（`backend/internal/browser/types.go:11`），经类型别名 `BrowserProfile = browser.Profile` 重导出到 Wails 层（`backend/app_browser_profile_api.go:11`）。没有名为 `Instance` 的结构体。

### 关键字段（JSON tag）

`ProfileId`, `ProfileName`, `UserDataDir`, `CoreId`, `FingerprintArgs []string`, `ProxyId`, `ProxyConfig`, `ProxyBindSourceID`, `ProxyBindSourceURL`, `ProxyBindName`, `ProxyBindUpdatedAt`, `LaunchArgs []string`, `Tags []string`, `Keywords []string`, `GroupId`, `LaunchCode`, `Running` bool, `DebugPort` int, `DebugReady` bool, `Pid` int, `RuntimeWarning`, `LastError`, `CreatedAt`, `UpdatedAt`, `DeletedAt`, `LastStartAt`, `LastStopAt`（`backend/internal/browser/types.go:11`）。

### user-data-dir

由 `ResolveUserDataDir` 计算（`backend/internal/browser/environment.go:18`）：
- `profile.UserDataDir` 为空 → 用 `profile.ProfileId`；
- 绝对路径 → 原样使用；
- 否则拼接到 `Config.Browser.UserDataRoot`（默认 `data`，相对 AppRoot 解析）下。

启动前 `os.MkdirAll`（`backend/app_instance_start_prepare.go:204`）。flag: `--user-data-dir=<resolved path>`。

### 指纹种子 / launch args

`buildBrowserLaunchArgs`（`backend/app_instance_start_prepare.go:275-315`）按序构建 argv：
1. `--user-data-dir=<dir>`
2. `--remote-debugging-port=<port>`（每次启动经 `nextAvailablePort()` 动态分配，`app_instance_start_prepare.go:149`，**无固定 9222**）
3. `--disable-session-crashed-bubble`
4. 指纹种子: 若 `profile.FingerprintArgs` 无 `--fingerprint=`，则由 `ProfileId` 字符串哈希派生确定性非负 seed（`seed = (seed<<5)-seed + int(char)`，负则取反），注入 `--fingerprint=<seed>`（`app_instance_start_prepare.go:282-298`）
5. 代理: `--proxy-server=<url>` 或 `--no-proxy-server`（`direct://`）
6. 扩展: `--disable-extensions-except=` / `--load-extension=`（`app_instance_start_prepare.go:306`）
7. 追加 `profile.FingerprintArgs`（逐条）
8. 经 `sanitizeManagedLaunchArgs` 过滤后的 `profile.LaunchArgs` 与 extra launch args（受管 flag 如 `--user-data-dir`、`--remote-debugging-port`、`--proxy-server`、`--load-extension` 会被剥离，系统值优先）
9. start URLs

默认指纹 args: `--fingerprint-brand=Chrome --fingerprint-platform=<mac|windows|linux>`（`backend/internal/config/config_defaults.go:327`，`config.yaml:28`）。这些均为 fingerprint-chromium CLI flag，stock Chrome 不识别。

### 持久化表

SQLite 表 `browser_profiles`（`backend/internal/database/sqlite.go:41`），`app.db`。列: `profile_id` PK, `profile_name`, `user_data_dir`, `core_id`, `fingerprint_args` TEXT(JSON `'[]'`), `proxy_id`, `proxy_config` TEXT, `proxy_bind_source_id/source_url/name/updated_at`, `launch_args` TEXT(JSON `'[]'`), `tags` TEXT(JSON `'[]'`), `keywords` TEXT(JSON `'[]'`), `group_id`, `created_at`, `updated_at`, `deleted_at` TEXT（软删除，`''` = 在用）。DAO: `SQLiteProfileDAO`，upsert 用 `INSERT ... ON CONFLICT(profile_id) DO UPDATE`（`backend/internal/browser/profile_dao.go:120`）。

> 启停生命周期: `BrowserInstanceStop` 先尝试 CDP `Browser.close`（5s 宽限），失败回退到进程 kill（Unix `Process.Kill` / Windows `taskkill /T /F`）（`backend/app_instance_stop.go:21,28`）。**无后端批量启停 API**——每个实例单独启停（`BrowserInstanceStart/Stop`），前端循环调用（`待确认 (runtime)`）。

---

## 代理 (Proxy)

### 模型与绑定

代理模型 `config.BrowserProxy`（`backend/internal/config/config.go:135`），别名为 `browser.Proxy`。字段: `ProxyId`, `ProxyName`, `ProxyConfig`（单一 URL 字符串，如 `socks5://user:pass@host:port`、`vmess://...` 或 Clash YAML 节点）、`PreferredKernel`, `DnsServers`, `GroupName` 等。**无显式 Type/Host/Port/Username/Password 字段**——全部编码在 `ProxyConfig` 字符串内。

实例通过 `Profile.ProxyId`(+ `Profile.ProxyConfig`) 绑定代理（`backend/internal/config/config.go:164`）。`BindProfileToProxy` 写入绑定快照（`ProxyBindSourceID/SourceURL/Name/UpdatedAt`），便于代理池变更后自动重绑（`backend/internal/browser/proxy_binding.go:57,122`）。

### 代理如何到达 Chrome（鉴权代理的关键）

启动时 `resolveBrowserStartProxy` 返回 `effectiveProxy`，`buildBrowserLaunchArgs` 注入 `--proxy-server=<effectiveProxy>`（`backend/app_instance_start_prepare.go:300-304`）。两种路径由 `ResolveProxyKernel` 决定（`backend/internal/proxy/kernel_resolver.go:122`）：

1. **直连 (native)**: 无凭据的 `http`/`https`/`socks5` 与 `direct://`。原始 `ProxyConfig` 直接作为 `--proxy-server`（`backend/app_instance_start_proxy.go:135`）。因 Chromium `--proxy-server` **无法携带账密**，带凭据的代理走下一条。
2. **本地桥接 (bridge)**: 带用户名/密码的 `http`/`https`/`socks5`（`RequiresLocalProxyBridgeForBrowser` 当 `url.User != nil` 触发，`backend/internal/proxy/browser_bridge.go:63`）、`vmess`/`vless`/`trojan`/`ss`/`chain` → `[xray, mihomo]`；`hysteria`/`hysteria2`/`tuic`/`anytls` → `[sing-box, mihomo]`；`mieru` → `[mihomo]`。启动一个本地连接器进程监听 `127.0.0.1:<port>`，Chrome 的 `--proxy-server` 指向该本地端口:
   - Xray 桥: `socks5://127.0.0.1:<port>`（`backend/internal/proxy/xray_bridge_launch.go:231`）
   - SingBox 桥: `socks5://127.0.0.1:<port>`（`backend/internal/proxy/singbox_bridge_runtime.go:108`）
   - Mihomo 桥: `http://127.0.0.1:<port>`（`mixed-port`，`backend/internal/proxy/mihomo_bridge.go:152`）

桥端口经 `nextAvailablePort()` 绑 `127.0.0.1:0` 由 OS 分配（`backend/internal/proxy/runtime_bridge_helpers.go:119`），**无固定端口范围**。桥以 `sha256(proxyConfig+dns)` 为 key 引用计数，多实例共享同一代理复用同一本地端口。实例停止时 `releaseProfileProxyBridge` 释放桥（`backend/app_bridge_refs.go:41`，`backend/app_instance_launch_args.go:127`）。

> 连接器配置文件不入库——所有 xray/mihomo/sing-box 配置在运行时按桥 key 生成（`xray_runtime_config.go:46`，`mihomo_bridge.go:327`）。

### 代理 API（Wails 绑定，非 HTTP 路由）

代理无 HTTP 路由；前端经 Wails 绑定直接调 Go 方法: `BrowserProxyList`/`SaveBrowserProxies`/`ValidateProxyConfig`/`BrowserProxyFetchClashByURL`/`BrowserProxyTestSpeed`/`BrowserProxyCheckIPHealth` 等（`backend/app_proxy_query.go:12`，`backend/app_proxy_health.go:17`）。默认 IP 健康探针: `https://my.ippure.com/v1/info`（`backend/internal/proxy/iphealth.go:15`）；测速: `http://www.gstatic.com/generate_204`（`backend/internal/proxy/speedtest.go:22`）。

---

## Launch / CDP API

### 路由（方法 + 路径）

LaunchServer 是本地 HTTP 服务器（`backend/internal/launchcode/`），用原生 `net/http` ServeMux（无 gin/echo）。路由（`backend/internal/launchcode/server_http.go:7-22`）：

| 方法 | 路径 | handler | 说明 |
|---|---|---|---|
| GET | `/api/launch/{code}` | `handleLaunch` | 按 launch code 启动 |
| POST | `/api/launch` | `handleLaunchWithBody` | 按 selector 启动（`LaunchSelector`） |
| GET | `/api/launch/logs` | — | 日志 |
| POST | `/api/runtime/stop` | `handleRuntimeStop` | 停止 |
| POST | `/api/runtime/status` | — | 状态 |
| GET | `/api/runtime/active` | `handleRuntimeActive` | 活跃实例 |
| POST | `/api/runtime/session` | `handleRuntimeSession` | 启动并等待 ready（默认 45s） |
| GET/POST | `/api/profiles` | — | 列表/创建 |
| GET/PUT/DELETE | `/api/profiles/{id}` | — | 读/改/删，含子动作 `/status`、`/stop` |
| — | `/api/automation/*` | — | 自动化 |
| GET | `/api/health` | `handleHealth` | 健康检查 |
| * | `/` (catch-all) | `handleCDPProxy` | 反向代理活跃实例的 CDP 端点 |

`POST /api/launch` 的 `LaunchSelector`（`backend/internal/launchcode/selector_types.go:17`）: `code`, `key`, `profileId`, `profileName`, `keyword(s)`, `tag(s)`, `groupId`, `matchMode`（`unique|first|all`，`all` 启动全部匹配）。

### CDP/WebSocket URL 字段

启动成功响应 `launchSuccessPayload`（`backend/internal/launchcode/server_launch.go:19`）返回 JSON：
```
ok, profileId, profileName, launchCode, pid, debugPort, debugReady,
runtimeWarning, cdpPort, cdpUrl
```
**承载 CDP/WebSocket 调试 URL 的字段是 `cdpUrl`**（小写 `c`，`server_launch.go:19`），值形如 `http://127.0.0.1:{port}`。默认指向统一 LaunchServer 端口；仅当 LaunchServer 无端口时回退到实例直连 `debugPort`（`server_launch.go:11`）。运行时 payload 另暴露 `directDebugUrl`（`http://127.0.0.1:{debugPort}`）。

> `cdpUrl` 是 HTTP base（`http://127.0.0.1:{port}`），**不是 `ws://` URL**。客户端需自行追加 `/devtools/page/{id}` 或 `/json` 访问原始 WS 端点（`待确认 (runtime)`：实时 `/json/version` 响应形状需运行 app 验证）。

### 默认端口

两类端口:
1. **LaunchServer HTTP/CDP-proxy 统一端口**: 默认 `19876`（`config.yaml:40`，常量 `DefaultLaunchServerPort=19876`，`backend/internal/config/config.go:4`），仅绑 `127.0.0.1`；配置端口 `<=0` 则 OS 随机分配，否则固定端口须空闲否则 `Start` 失败（`backend/internal/launchcode/server.go:139,190`）。
2. **每实例浏览器 `--remote-debugging-port`**: 每次 launch 经 `nextAvailablePort()` 动态分配（`127.0.0.1:0` 随机临时端口，双重绑定校验，`backend/app_utils.go:32`，`backend/app_instance_start_prepare.go:149`），**无固定 9222 式默认**。活跃实例的 debug 端口存在 server 上并经统一端口代理。

`--remote-debugging-address` 不由 app 添加（浏览器仅监听 localhost）。`--remote-debugging-port`/`--remote-debugging-address`/`--remote-debugging-pipe`/`--user-data-dir` 是“受管”参数，调用方在 `LaunchRequestParams.launchArgs` 中提供的值会被 `sanitizeManagedLaunchArgs` 剥离并由 app 覆盖。统一 CDP 入口经 LaunchServer 的 `handleCDPProxy`（`ReverseProxy`）转发非 `/api` 路径到活跃实例 debug 端口；`/json`、`/json/version`、`/devtools/*` 经此到达真实 CDP。

### API base URL 与默认 HTTP 端口

- Base URL: `http://127.0.0.1:{launchServerPort}`（`CDPURL()`，`backend/internal/launchcode/server.go:240`），默认端口 `19876`。
- 健康检查: `GET /api/health`。
- 单实例锁: `app-instance.lock` + TCP listener（`127.0.0.1:0` 随机端口，`single_instance.go:15,33`）。

---

## 鉴权

两层防护（`backend/internal/launchcode/auth.go`，`server_http_utils.go:14`）：

1. **`localhostMiddleware`**: 仅接受来自 `127.0.0.1` 的请求，否则 403。
2. **`apiAuthMiddleware`**: 仅作用于 `/api/*` 路径；**仅当 `config.LaunchServer.Auth.Enabled=true` 且 `api_key` 非空时激活**（`auth.go:68`）。激活时要求 header `X-Ant-Api-Key`（默认，`DefaultAPIKeyHeader`，`auth.go:9`，可经 `auth.header` 配置）与 `api_key` 常量时间比较匹配，否则 401。

**鉴权是可选 (OPT-IN)，默认关闭。** `config.yaml:42`:
```
auth:
  enabled: false
  api_key: ""
```
默认无需任何 API key。非 `/api` 的 CDP 代理路径 `/` **不受 `apiAuthMiddleware` 保护**（仅受 localhost 约束），即 CDP 端点本身仅依赖 `127.0.0.1` 绑定。无 Bearer/Authorization 方案。

---

## Playwright 接入

通过 CDP 接入活跃实例。启动路由返回 `cdpUrl`（`http://127.0.0.1:{port}`），Playwright 用 `connect_over_cdp` 连接（默认无需认证，本地 only）。

10 行 Python 骨架（配套探针脚本见 `docs/fork/scripts/cdp_probe.py`）：

```python
import requests, sys
from playwright.sync_api import sync_playwright
base = "http://127.0.0.1:19876"              # LaunchServer base (config.yaml launch_server.port)
r = requests.post(f"{base}/api/runtime/session",
                  json={"code": sys.argv[1], "timeoutMs": 30000}).json()  # 启动并等待 ready
cdp_url = r["cdpUrl"]                        # 真实字段名: cdpUrl (小写, http://127.0.0.1:{port})
with sync_playwright() as p:
    browser = p.chromium.connect_over_cdp(cdp_url)   # 经 CDP 接入活跃实例
    page = browser.contexts[0].new_page()
    page.goto("https://browserleaks.com/javascript")
    print("title:", page.title())            # 打印标题 + 指纹提示
    print("ua:", page.evaluate("navigator.userAgent"))
requests.post(f"{base}/api/runtime/stop", json={"code": sys.argv[1]})   # 停止实例
```

> 说明: 本地默认 `no auth / local-only`（`auth.enabled: false`）。若启用了 `X-Ant-Api-Key`，需在 `requests` 头加 `X-Ant-Api-Key: <key>`。

---

## 待运行时验证 (Deferred to runtime/GUI)

以下为审计中的 `open_questions`，需运行 GUI/build 才能确认。**核对清单**：

- [x] **wsEndpoint/WS URL 字段形状**: 已验证 `cdpUrl` 为 `http://127.0.0.1:{port}`（实测 `cdpUrl=http://127.0.0.1:19876`，即 LaunchServer 统一端口，非 `ws://`）；cdp_probe 经 Playwright `connect_over_cdp(cdpUrl)` 成功打开页面（title=AntProbe），证实客户端经该 HTTP base 追加 `/json`、`/devtools/page/{id}` 即可到达原始 WS。另返回 `directDebugUrl=http://127.0.0.1:{debugPort}` 与 `debugPort`。
- [x] **CDP 代理 WebSocket 升级**: 已验证 `handleCDPProxy` 经统一端口 `19876` 正确代理 CDP——cdp_probe 经 `connect_over_cdp('http://127.0.0.1:19876')` 完成真实浏览器 target 的 WS 升级并执行 evaluate（`ua`、`title`），`[stop] status=200`。
- [ ] **实例启停 / 多选**: (blocked: 单实例启停 API 已验证 `POST /api/runtime/session` ok + `POST /api/runtime/stop` stopped:true；但前端多选循环 `BrowserInstanceStart/Stop` 未在 `frontend/src` 验证)
- [x] **`app.db` 磁盘路径**: 已验证为用户 stateRoot 下 `~/Library/Application Support/ant-browser/data/app.db`（PRODUCTION 模式下 appRoot=exeDir，stateRoot=`~/Library/Application Support/ant-browser`），非仓库根 `data/`；`sqlite3` 直接 INSERT/SELECT `browser_cores` 成功。
- [ ] **fingerprint-chromium flag 生效**: (blocked: 需 fingerprint-patched Chrome 内核；实测 stock Chrome 150 静默忽略 `--fingerprint=<seed>`/`--fingerprint-brand`/`--fingerprint-platform`，两实例 `navigator.userAgent` 完全相同，指纹隔离未生效)
- [x] **stock Chrome 拒绝 `--fingerprint*`**: 已验证 stock Chrome **不拒绝**——`/Applications/Google Chrome.app` 150.0.7871.115 带 `--fingerprint=12345 --fingerprint-brand=... --fingerprint-platform=...` 正常启动且 `DevTools listening on ws://...`，无报错。即启动器对 fingerprint-chromium 并非硬依赖（修正原审计假设：stock Chrome 静默忽略未知 flag）。
- [ ] **BrowserLeaks 指纹隔离**: (blocked: 需 fingerprint-patched Chrome 内核；stock Chrome 下两实例仅 userDataDir + debugPort 不同，UA 一致，canvas/WebGL/audio/fonts 隔离无法验证)
- [ ] **IP 出口**: (blocked: 需 2 个真实代理；本轮未绑定代理，`--no-proxy-server` 直连，未验证出口 IP 隔离)
- [ ] **内核注册 (macOS .app 布局)**: (blocked: `findAppBundleExecutable` 将 `.app` CorePath 解析到内嵌 MacOS 二进制已验证（`/Applications/Google Chrome.app` → `Contents/MacOS/Google Chrome`）；但 `scanChromeDir` 深度 5 递归是否误注册 `.app` 内 Helper 为独立内核未实测——本轮内核经 DB INSERT 注册，未走扫描路径)
- [ ] **内核下载命名/布局**: (blocked: 未运行 `download_core_*.go` 下载流程)
- [ ] **DeleteGroup 悬挂 group_id**: (blocked: 未运行 GUI 分组删除)
- [ ] **代理凭据静态加密**: (blocked: 未绑定带凭据代理，未查 `ProxyDAO` schema)
- [ ] **代理凭据日志脱敏**: (blocked: 未触发带凭据代理的 resolution 日志)
- [ ] **Mihomo external-controller 可达性**: (blocked: 未启动 Mihomo 桥)
- [ ] **`StartReadyTimeoutMs`/`StartStableWindowMs` 默认值**: (blocked: 未对比 `app_instance_start_execute.go` 运行时就绪检测默认值；`config.yaml` 显示 3000/1200)
- [x] **9222 默认端口**: 已验证无 9222——每实例 `--remote-debugging-port` 经 `nextAvailablePort()` 动态分配（实测 debugPort=57714）；LaunchServer 统一端口默认 19876。

---

## 下一步 (Phase 0 manual checklist)

人工执行验证清单：

- [ ] 安装 Wails CLI: `go install github.com/wailsapp/wails/v2/cmd/wails@v2.12.0`
- [ ] 安装 Playwright (Python): `pip install playwright requests && playwright install chromium`
- [ ] `./dev.sh`（或 Windows `bat\dev.bat`）启动 `wails dev`，确认前端+后端起来
- [ ] 注册 fingerprint-chromium 内核: 将 fingerprint-chromium 构建放入 `chrome/<dir>/`，在内核管理 UI 注册该 Core 并设为默认
- [ ] 创建 2 个实例（Profile），各绑定不同指纹种子（由 ProfileId 自动派生）
- [ ] 创建 2 个代理（至少 1 个带账密以触发本地桥接），分别绑定到 2 个实例
- [ ] 启动实例，访问 https://browserleaks.com/javascript 验证指纹隔离（canvas/WebGL/audio/fonts 互异）
- [ ] 访问 IP 健康探针验证 IP 出口为代理 IP
- [ ] 运行 `python docs/fork/scripts/cdp_probe.py <instance_code> http://127.0.0.1:19876` 验证 CDP 接入
- [ ] 记录运行时验证结果，回填本文档“待运行时验证”清单