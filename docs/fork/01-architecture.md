# 01 — 架构总览（ProfilePool fork）

> 本文描述 ProfilePool fork 的两层架构：**已审计的现状**（继承自上游 black-ant/Ant-Browser，基线 tag `V1.2.0`）与**计划新增的 accountpool 业务层**。
> 现状部分以 `docs/fork/00-baseline.md` 的审计为准；accountpool 部分以 `docs/fork-ant-browser-plan.md` §3/§4/§5 为准。
> fork 身份（应用名 / 数据目录 / bundle id / 二进制名 / 单例锁）见 Phase 1 改造，记录在仓库根 README 的 “Fork: ProfilePool” 节。

---

## 1. 总体形态

```text
┌──────────────────────────────────────────────────────────┐
│  GUI（Wails v2 + 现有 React 前端）                          │
│   - 实例列表（CRUD / 克隆 / 分组 / 标签 / 批量启停）          │
│   - 代理池 / 内核管理 / 自动化脚本 / 实例导入导出            │
│   - 账号池面板（计划 P2，可选）                             │
└───────────────────────────┬──────────────────────────────┘
                            │ Wails bindings（前端直调 Go 方法）
┌───────────────────────────▼──────────────────────────────┐
│  Backend（Go，module ant-chrome —— 模块路径不改）          │
│  ┌─────────────┐ ┌──────────────┐ ┌─────────────┐         │
│  │ instances   │ │ proxies      │ │ cores        │  ←上游 │
│  └──────┬──────┘ └──────┬───────┘ └──────┬──────┘         │
│         │               │                │                │
│  ┌──────▼───────────────▼────────────────▼──────────────┐ │
│  │ accountpool（计划新增，独立包 backend/internal/...）   │ │
│  │  accounts / leases / cooldowns / events               │ │
│  │  API: lease → start → cdp_url → release                │ │
│  └────────────────────────┬─────────────────────────────┘ │
│                           │ 调用上游 Launch / CDP          │
└───────────────────────────┼──────────────────────────────┘
                            │ spawn
              官方 Chromium / Google Chrome / Chrome for Testing（+ --user-data-dir）
                            ▲
              Playwright Worker（外置，独立仓库/目录）
              xhs_worker / x_worker …
```

设计约定（来自 plan §3）：

```text
Account ──绑定──► Instance ──绑定──► Proxy
                  └─ user-data 持久化登录态
                  └─ fingerprint 启动参数尽量固定
```

- **人工使用**：GUI 启动 Instance。
- **爬虫使用**：Worker 调 `lease` → 内部 `start` → 返回 `cdp_url` → 任务结束 `release`。
- **禁止**：两个任务同时 lease 同一账号；人工正在用的实例默认不可被爬虫抢。

---

## 2. 已审计现状（继承上游）

### 2.1 实例模型（Profile = 一个浏览器实例）

实例在内部称 **Profile**（结构 `browser.Profile`，`backend/internal/browser/types.go:11`），持久化于 SQLite 表 `browser_profiles`（`backend/internal/database/sqlite.go:41`，库文件 `data/app.db`，纯 Go 驱动 `modernc.org/sqlite`，无 CGo）。关键字段：`ProfileId`、`ProfileName`、`UserDataDir`、`CoreId`、`FingerprintArgs`、`ProxyId`/`ProxyConfig`、`LaunchArgs`、`Tags`、`GroupId`、`LaunchCode`、`Running`、`DebugPort`、`Pid` 等。DAO `SQLiteProfileDAO` 用 `INSERT ... ON CONFLICT(profile_id) DO UPDATE` upsert；`deleted_at` 软删除。

一个 Profile ≈ 一个账号环境：独立 `--user-data-dir`、固定指纹 seed、绑定代理。

### 2.2 内核模型（Core）

内核在内部称 **Core**（结构 `config.BrowserCore`，`backend/internal/config/config.go:128`），持久化于 SQLite 表 `browser_cores`（`backend/internal/database/sqlite.go:66`）。字段：`CoreId`（UUID）、`CoreName`、`CorePath`（含浏览器可执行文件的目录）、`is_default`、`sort_order`。当 `CoreDAO` 为 nil 时回退到 `config.yaml` 的 `cores:` 列表。

启动时 `ensureDefaultCores()` 调 `scanChromeDir` 扫描内核根（`config.Browser.CoreRoot`，未设则字面量 `chrome/`）注册内核。可执行文件候选名按平台差异（`CoreExecutableCandidates()`，`backend/internal/browser/core_binary.go:11`）。

> 关键（已切换）：启动器以**官方 Chromium / Google Chrome / Chrome for Testing** 为准，**不再注入** fingerprint-chromium 的 `--fingerprint*` 等专有 flag；历史配置中的此类参数在启动时会被剥离。接入方式：登记本机 Chrome，或从 Chrome for Testing 下载官方测试构建到 `chrome/`。

### 2.3 启动链路与 LaunchServer

- **LaunchServer** 是本地 HTTP 服务器（`backend/internal/launchcode/`，原生 `net/http` ServeMux，无 gin/echo），统一端口默认 **`19876`**（常量 `DefaultLaunchServerPort`，`backend/internal/config/config.go:4`），仅绑 `127.0.0.1`；端口 `<=0` 则 OS 随机分配，否则固定端口须空闲否则启动失败。
- 每实例浏览器 `--remote-debugging-port` 由 `nextAvailablePort()` 动态分配（`127.0.0.1:0` 随机临时端口，**无固定 9222 默认**）。`--remote-debugging-address` 不由 app 添加（浏览器仅监听 localhost）。
- `--remote-debugging-port`/`--user-data-dir`/`--proxy-server` 等是“受管”参数，调用方在 `LaunchRequestParams.launchArgs` 提供的值会被 `sanitizeManagedLaunchArgs` 剥离并由 app 覆盖。

### 2.4 CDP 反向代理

LaunchServer 的 catch-all `/`（`handleCDPProxy`，`backend/internal/launchcode/server_http.go`）是一个 `ReverseProxy`：把非 `/api` 路径（`/json`、`/json/version`、`/devtools/*` 等）转发到当前活跃实例的 debug 端口。客户端拿到的 `cdpUrl` 是 **HTTP base `http://127.0.0.1:{launchServerPort}`**（不是 `ws://`），需自行追加 `/devtools/page/{id}` 或 `/json` 访问原始 WS 端点。仅当 LaunchServer 无端口时才回退到实例直连 `directDebugUrl`（`http://127.0.0.1:{debugPort}`）。

### 2.5 Launch / CDP API 路由

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/launch/{code}` | 按 launch code 启动 |
| POST | `/api/launch` | 按 `LaunchSelector`（code/key/profileId/profileName/keyword/tag/groupId/matchMode）启动 |
| POST | `/api/runtime/session` | 启动并等待 ready（默认 45s），返回 `cdpUrl` |
| POST | `/api/runtime/stop` | 停止 |
| POST | `/api/runtime/status` | 状态 |
| GET | `/api/runtime/active` | 活跃实例 |
| GET/POST | `/api/profiles` | 列表/创建 |
| GET/PUT/DELETE | `/api/profiles/{id}` | 读/改/删（含子动作 `/status`、`/stop`） |
| — | `/api/automation/*` | 自动化脚本 |
| GET | `/api/health` | 健康检查 |
| * | `/`（catch-all） | CDP 反向代理 |

启动成功 payload（`launchSuccessPayload`）含：`ok, profileId, profileName, launchCode, pid, debugPort, debugReady, runtimeWarning, cdpPort, cdpUrl`。**承载 CDP 调试 URL 的字段是 `cdpUrl`**（小写 `c`）。

### 2.6 鉴权

两层（`backend/internal/launchcode/auth.go`）：

1. `localhostMiddleware`：仅接受来自 `127.0.0.1` 的请求，否则 403。
2. `apiAuthMiddleware`：仅 `/api/*`；**OPT-IN**，仅当 `config.LaunchServer.Auth.Enabled=true` 且 `api_key` 非空时激活；激活时要求 header `X-Ant-Api-Key`（默认，可经 `auth.header` 配置）常量时间比较匹配。默认关闭。

### 2.7 代理桥（proxy bridge）

实例经 `Profile.ProxyId`(+`ProxyConfig`) 绑定代理。`ResolveProxyKernel`（`backend/internal/proxy/kernel_resolver.go:122`）决定两条路径：

1. **直连 (native)**：无凭据的 `http`/`https`/`socks5` 与 `direct://` —— 原始 `ProxyConfig` 直接作 `--proxy-server`（Chromium `--proxy-server` 无法携带账密）。
2. **本地桥接 (bridge)**：带账密的 `http`/`https`/`socks5`、`vmess`/`vless`/`trojan`/`ss`/`chain` → `[xray, mihomo]`；`hysteria`/`hysteria2`/`tuic`/`anytls` → `[sing-box, mihomo]`；`mieru` → `[mihomo]`。启动一个本地连接器进程监听 `127.0.0.1:<port>`，Chrome 的 `--proxy-server` 指向该本地端口（xray/sing-box 桥 `socks5://127.0.0.1:<port>`，mihomo 桥 `http://127.0.0.1:<port>` mixed-port）。

桥端口经 `nextAvailablePort()` 绑 `127.0.0.1:0` 由 OS 分配，**无固定端口范围**；以 `sha256(proxyConfig+dns)` 为 key 引用计数，多实例共享同一代理复用同一本地端口。连接器配置文件不入库，运行时按桥 key 生成。默认 IP 健康探针 `https://my.ippure.com/v1/info`，测速 `http://www.gstatic.com/generate_204`。代理无 HTTP 路由，前端经 Wails bindings 直调 Go 方法。

### 2.8 数据库

SQLite 单库 `data/app.db`（`modernc.org/sqlite`，纯 Go，无 CGo）。表含 `browser_profiles`、`browser_cores`、代理、分组、书签、自动化脚本等。账号池计划在此基础上**新增**表（见 §3.2），不推翻上游 `browser_profiles`。

### 2.9 单例与身份（fork 改造后）

- 单实例锁文件 `profilepool.lock` + TCP listener（`127.0.0.1:0` 随机端口），位于各平台 state dir 内（`single_instance.go`）。
- state 目录（fork 改名后）：
  - Linux：`~/.local/share/profile-pool/`
  - macOS：`~/Library/Application Support/profile-pool/`
  - Windows：`%LOCALAPPDATA%\ProfilePool\`（`single_instance_state_windows.go` 硬编码目录名 `ProfilePool`）
- 应用名 / 窗口标题 / bundle id / 二进制名 见仓库根 README “Fork: ProfilePool” 节。

---

## 3. 计划新增：accountpool 业务层

> 来源：`docs/fork-ant-browser-plan.md` §3（目标架构）、§4（数据模型）、§5（API 设计）。
> 原则：**业务改动集中在新增包（`backend/internal/...` 下，建议 `accountpool/`），少改启动链路核心；能包装上游 Launch/CDP API 就不改启动原语。**

### 3.1 概念

- **Instance（实例）** = 浏览器环境（上游已有 Profile）。
- **Account（账号）** = 业务身份，挂在 Instance 上的业务视图。两者绑定，概念分开。
- **Lease（租约）** = 爬虫任务领取账号 → 加锁 → 用完释放；失败进冷却。

核心规则（plan §3，“比浏览器选型更重要”）：

1. 固定绑定：`account_id → fingerprint_seed → proxy_id → user_data_dir`。
2. 租约：任务领取账号 → 加锁 → 用完释放；失败进冷却。
3. 代理与指纹一致：时区/语言/geo 跟代理出口走。
4. 频率与行为：指纹只是门槛；限速、会话时长、拟人操作同样关键。
5. 双通道：人工用 GUI 开 Profile；爬虫只走 API + CDP，不共用正在人工操作的实例。

### 3.2 数据模型（在 `app.db` 上扩展，迁移脚本单独做）

**`accounts`**：`id`、`platform`(`xhs`/`x`/`other`)、`username`/`display_name`、`instance_id`（绑定实例）、`proxy_id`（冗余，以 instance 绑定为准）、`status`(`active`/`cooldown`/`banned`/`need_login`/`disabled`)、`fingerprint_seed`（写入 instance 配置）、`notes`/`tags`、`last_used_at`/`cooldown_until`、`meta_json`（cookie 校验时间、风控备注等）。

**`account_leases`**：`id`、`account_id`、`worker_id`、`purpose`(`manual`/`scrape`/`warmup`)、`status`(`held`/`released`/`expired`/`stolen`)、`cdp_endpoint`（可选缓存）、`leased_at`/`expires_at`/`heartbeat_at`。

**`account_events`**（可选审计）：登录成功、验证码、403、封禁、释放原因。

> 原则：不推翻上游 `instances`/`browser_profiles` 表；账号是**挂在 instance 上的业务视图**。

### 3.3 API（在上游 Launch API 之上包一层）

| 方法 | 路径 | 作用 |
|---|---|---|
| `POST` | `/api/v1/pool/accounts` | 创建账号并可选自动建 instance |
| `GET` | `/api/v1/pool/accounts?platform=&status=` | 列表 |
| `POST` | `/api/v1/pool/lease` | 租一个可用账号 |
| `POST` | `/api/v1/pool/lease/:id/heartbeat` | 续租 |
| `POST` | `/api/v1/pool/lease/:id/release` | 释放（可带 result: ok/risk/ban） |
| `POST` | `/api/v1/pool/accounts/:id/start` | 启动绑定实例，返回 CDP |
| `POST` | `/api/v1/pool/accounts/:id/stop` | 停止 |

`lease` 请求示例：

```json
{
  "platform": "xhs",
  "worker_id": "scraper-mac-01",
  "ttl_sec": 900,
  "auto_start": true,
  "tags_any": ["pool-a"]
}
```

### 3.4 调用链（爬虫侧）

```text
Worker: POST /api/v1/pool/lease {platform, worker_id, ttl_sec, auto_start}
  └─ accountpool: 选一个 active 且未被锁定的 account → 写 account_leases(status=held)
     └─ 若 auto_start: 调上游 POST /api/runtime/session（或 /api/launch by profileId）启动绑定 instance
        └─ 返回 { account_id, lease_id, cdp_url }
Worker: 用 cdp_url 接管 CDP（经 LaunchServer 反向代理 / catch-all）
Worker: 任务结束 POST /api/v1/pool/lease/:id/release { result }
  └─ accountpool: 释放锁，按 result 决定 status（ok→active / risk→cooldown / ban→banned）
```

---

## 4. 工程约束（与上游同步配合）

- **新增业务集中在 `accountpool/` 等新包**，最小化对启动链路（`launchcode/`、`app_instance_start_*`、`proxy/`）的改动，便于跟进上游（见 `docs/fork/03-upstream-sync.md`）。
- **不改 Go 模块路径** `ant-chrome`（约 200 个 import path 派生自它，rename 模块路径超出 Phase 1 范围）。
- 身份/路径改造在 Phase 1 一次性完成（应用名、数据目录、bundle id、二进制名、单例锁），避免后期迁移 user-data。