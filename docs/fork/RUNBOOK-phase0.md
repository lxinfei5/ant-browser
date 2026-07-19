# Phase 0 人工验证 Runbook

本 runbook 用于在 macOS 上人工完成 Phase 0 运行时验证。所有命令已按本轮验证结果调好；逐节执行即可。
LaunchServer 统一端口默认 `19876`，API base = `http://127.0.0.1:19876`。

> 关键背景: `wails dev` 编译出的 `.app` bundle 被检测为 PRODUCTION 模式（exe 路径以 `/Contents/MacOS` 结尾，非 `/build/bin`），故 appRoot=exeDir，配置/DB 取自**用户 stateRoot** `~/Library/Application Support/ant-browser/`，**不读仓库根 `config.yaml`**。内核必须注册到 stateRoot 的 SQLite，而非改仓库 `config.yaml`。

---

## 前置

```sh
# 1. Wails CLI（PATH 中需可找到 wails）
go install github.com/wailsapp/wails/v2/cmd/wails@v2.13.0
export PATH="$PATH:$(go env GOPATH)/bin"
wails version            # 确认 wails 在 PATH

# 2. Playwright venv（cdp_probe 用此 venv 的 python，不要用系统 python）
cd /Users/lixinfei/Documents/Workbuddy/projects/Ollama/Ant-Browser
python3 -m venv docs/fork/.venv
docs/fork/.venv/bin/pip install playwright requests
docs/fork/.venv/bin/playwright install chromium

# 3. 系统 Chrome（作为初始内核；指纹隔离需后续换成 fingerprint-chromium）
ls "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"   # 应存在，版本 150.x

# 4. 私有 Go 缓存（避免与同机其它 go build 抢共享 GOMODCACHE 的 flock 死锁）
export GOCACHE=/tmp/ant-gocache
export GOMODCACHE=/tmp/ant-gomodcache
export GOPROXY=https://goproxy.cn,direct
# 若 /tmp/ant-gomodcache 为空，从系统 mod 缓存复制一份以省下载:
# cp -a "$(go env GOMODCACHE)" /tmp/ant-gomodcache

# 5. (可选) dev.sh 依赖 ss，macOS 无；如想用 ./dev.sh stable:
#   brew install iproute2mac   # 提供 ss
# 否则走下方“启动”中的 wails dev 手动路径，绕过 dev.sh。
```

> go.mod/go.sum 已被 wails CLI 升级到 wails v2.13.0 并 `go mod tidy`（uncommitted，构建必需，勿回退）。

---

## 启动

```sh
cd /Users/lixinfei/Documents/Workbuddy/projects/Ollama/Ant-Browser
# 首次构建前端
cd frontend && npm install && npm run build && cd ..

# 清掉旧 bundle 的 xattr（否则 codesign 会因 FinderInfo/resource fork 失败）
rm -rf build/bin
xattr -cr build 2>/dev/null || true
xattr -cr frontend/dist 2>/dev/null || true

# 启动（私有 GOCACHE/GOMODCACHE 已在前置里 export）
wails dev
```

启动成功标志（终端日志）:
```
Compiling application: Done.
Packaging application: Done.
LaunchServer 已启动 | port=19876
应用启动成功
```

确认 LaunchServer up（另一终端）:
```sh
curl -s http://127.0.0.1:19876/api/health
# 期望: {"ok":true}
lsof -nP -iTCP:19876 -sTCP:LISTEN   # 应见 ant-chrome PID 监听 127.0.0.1:19876
```

---

## 注册内核

app 读 stateRoot 配置，不读仓库 `config.yaml`，故 `config.yaml` 的 `cores:` 是 no-op。用下面的脚本化方法（已验证可行，无需重启）:

```sh
sqlite3 "$HOME/Library/Application Support/ant-browser/data/app.db" \
  "INSERT INTO browser_cores (core_id, core_name, core_path, is_default, sort_order) VALUES ('system-chrome','System Chrome','/Applications/Google Chrome.app',1,0);"

# 验证
sqlite3 "$HOME/Library/Application Support/ant-browser/data/app.db" \
  "SELECT core_id,core_name,core_path,is_default FROM browser_cores;"
# 期望行: system-chrome|System Chrome|/Applications/Google Chrome.app|1
```

GUI 路径（等价，二选一）: 打开 app → 内核管理 / Core Management → 添加内核 → CorePath 填 `/Applications/Google Chrome.app` → 设为默认 → 保存。`FindCoreExecutable` 会把 `.app` 解析到内嵌 `Contents/MacOS/Google Chrome`。

> `ListCores()` 直读 CoreDAO，DB INSERT 后立即可见，无需重启 app。

### 后续换成 fingerprint-chromium（真正指纹隔离所必需）

stock Chrome 静默忽略 `--fingerprint=<seed>` 等 flag，无指纹差异；需 fingerprint-patched 内核:

```sh
# 1. 下载 x86_64 dmg（版本 148.0.7778.215）
#    https://github.com/adryfish/fingerprint-chromium/releases
#    选 fingerprint-chromium_148.0.7778.215_x86_64.dmg（x86_64 构建在 arm64 上经 Rosetta 运行）
# arm64 Mac 首次需 Rosetta:
softwareupdate --install-rosetta   # 若未装

# 2. 挂载并拷出 .app
hdiutil attach ~/Downloads/fingerprint-chromium_148.0.7778.215_x86_64.dmg
cp -R "/Volumes/fingerprint-chromium 148.0.7778.215/Google Chrome.app" /Applications/FPChrome.app
hdiutil detach "/Volumes/fingerprint-chromium 148.0.7778.215"

# 3. 去 quarantine（否则 Gatekeeper/codesign 会拦）
xattr -d com.apple.quarantine /Applications/FPChrome.app 2>/dev/null || true
xattr -cr /Applications/FPChrome.app

# 4. 注册为新内核（CorePath 指向 .app）
sqlite3 "$HOME/Library/Application Support/ant-browser/data/app.db" \
  "INSERT INTO browser_cores (core_id, core_name, core_path, is_default, sort_order) VALUES ('fp-chromium','Fingerprint Chromium','/Applications/FPChrome.app',1,0);"
# 把 system-chrome 设为非默认
sqlite3 "$HOME/Library/Application Support/ant-browser/data/app.db" \
  "UPDATE browser_cores SET is_default=0 WHERE core_id='system-chrome';"
```

之后创建实例时用 `coreId='fp-chromium'`。**指纹/IP 隔离类验证必须用此内核**，否则 `--fingerprint*` 不生效。

---

## 建实例 + 代理

GUI 步骤（建 2 个实例）: 打开 app → 实例/Profile 管理 → 新建 → 填 ProfileName、选 CoreId（`system-chrome` 或 `fp-chromium`）、launchArgs 可填 `--no-first-run --disable-sync` → 保存。重复一次建第二个。记下两个实例的 **launchCode**（6 位，列表中可见）。

绑定代理（GUI: 实例编辑页 → 代理绑定；或 HTTP API）:
```sh
# 带 user:pass 的 http/socks5 会触发本地桥接（xray/mihomo/sing-box）；无凭据直连。
# IP 出口隔离验证需 2 个真实代理（不同出口 IP）。示例（替换为你的真实代理）:
curl -s -X POST http://127.0.0.1:19876/api/profiles \
  -H 'Content-Type: application/json' \
  -d '{"profile":{"profileName":"Inst-A","coreId":"fp-chromium","launchArgs":["--no-first-run","--disable-sync"]}}'
curl -s -X POST http://127.0.0.1:19876/api/profiles \
  -H 'Content-Type: application/json' \
  -d '{"profile":{"profileName":"Inst-B","coreId":"fp-chromium","launchArgs":["--no-first-run","--disable-sync"]}}'
```
记下返回的 `profileId` 与 `launchCode`。代理绑定经 GUI 代理管理页（`ProxyConfig` 单 URL 字符串，如 `socks5://user:pass@host:port`）再绑定到实例；或经 Wails 绑定方法（无 HTTP 路由）。

> 注意: IP 出口隔离需要 **2 个真实代理**（不同出口 IP）；本轮未绑定代理，该验证项为 blocked。

---

## CDP 验证

用 venv 的 python（不要用系统 python）:
```sh
cd /Users/lixinfei/Documents/Workbuddy/projects/Ollama/Ant-Browser
# 先启动实例并取 cdpUrl
curl -s -X POST http://127.0.0.1:19876/api/runtime/session \
  -H 'Content-Type: application/json' \
  -d '{"code":"<LAUNCHCODE>","timeoutMs":30000}'
# 返回 ok:true, active:true, debugReady:true, debugPort:<动态>, cdpUrl:'http://127.0.0.1:19876', directDebugUrl:'http://127.0.0.1:<debugPort>'
# 注: cdpUrl 仅在 active=true 时填充；若为空就轮询直到 debugReady=true。

# 探针
docs/fork/.venv/bin/python docs/fork/scripts/cdp_probe.py <LAUNCHCODE> \
  --base http://127.0.0.1:19876 \
  --url 'data:text/html,<title>AntProbe</title><h1>AntProbe</h1>AntProbe'
```
成功标志:
```
[ok] cdpUrl = http://127.0.0.1:19876
[page] title=AntProbe
[hint] ua=Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/150.0.0.0 Safari/537.36 lang=zh-CN
[stop] status=200 ... stopped:true
```

---

## 指纹/IP 隔离

> 必须 2 个实例 + fingerprint-chromium 内核（`fp-chromium`）+ 各绑不同真实代理，隔离才有意义。

对每个实例启动后用 cdp_probe（或 Playwright `connect_over_cdp`）打开:
```sh
docs/fork/.venv/bin/python docs/fork/scripts/cdp_probe.py <CODE_A> \
  --base http://127.0.0.1:19876 --url 'https://browserleaks.com/javascript'
docs/fork/.venv/bin/python docs/fork/scripts/cdp_probe.py <CODE_B> \
  --base http://127.0.0.1:19876 --url 'https://browserleaks.com/javascript'
# IP 探针:
#   --url 'https://my.ippure.com/v1/info'    (后端默认 IP 健康探针)
```
对比 2 个实例的:
- 指纹: BrowserLeaks 的 canvas/WebGL/audio/fonts hash —— 期望互不相同（需 fp-chromium；stock Chrome 下相同）。
- IP: `my.ippure.com/v1/info` 的出口 IP —— 期望分别为各自代理 IP（需 2 个真实代理）。
- UA/platform: 期望随 `--fingerprint-platform` 不同（需 fp-chromium；stock Chrome 下一致）。

---

## 清理

```sh
# 停止实例
curl -s -X POST http://127.0.0.1:19876/api/runtime/stop -H 'Content-Type: application/json' -d '{"code":"<CODE_A>"}'
curl -s -X POST http://127.0.0.1:19876/api/runtime/stop -H 'Content-Type: application/json' -d '{"code":"<CODE_B>"}'
curl -s http://127.0.0.1:19876/api/runtime/active   # 期望 active:false

# 退出 app: 在 app 窗口 Cmd+Q，或
pkill -f 'wails dev'
pkill -f 'ant-chrome'
lsof -nP -iTCP:19876 -sTCP:LISTEN   # 期望 FREE
lsof -nP -iTCP:34115 -sTCP:LISTEN   # 期望 FREE（devserver 端口）

# (可选) 清测试实例/私有缓存
#   经 GUI 或 DELETE /api/profiles/{id} 删测试实例；system-chrome / fp-chromium 内核保留
#   rm -rf /tmp/ant-gocache /tmp/ant-gomodcache
```