# Fork Ant-Browser 实施计划

> 状态：已定稿，待实现  
> 创建日期：2026-07-19  
> 上游：https://github.com/black-ant/Ant-Browser  
> 推荐内核：https://github.com/adryfish/fingerprint-chromium  
> 原则：**先跑通上游能力，再加业务层；少改内核，多改编排与数据模型。**

---

## 背景与双目标

| # | 目标 | 真正要的能力 |
|---|------|----------------|
| 1 | **多账号日常使用** | 一账号一 Profile，GUI 切换，独立 Cookie/存储，代理绑定，不串号 |
| 2 | **爬取 + 账号池** | 固定指纹/代理，API 启停，Playwright/CDP 接管，降风控 |

**已选定方案：Fork Ant-Browser**，引擎使用 fingerprint-chromium；爬取侧 worker 外置；可选后续加 Camoufox 作第二引擎。

---

## 0. 目标与非目标

### 目标（MVP → 可用）

| 优先级 | 能力 |
|--------|------|
| P0 | 本地 fork 能编译、能装 fingerprint-chromium、能多实例启动 |
| P0 | 沿用上游 Launch API / CDP，外部 Playwright 可接管 |
| P0 | 实例维度：平台标签、账号备注、固定代理绑定 |
| P1 | 账号池：租约/加锁、冷却、状态机（idle/leased/cooldown/banned） |
| P1 | 账号池 API：`lease → start → cdp_url → release` |
| P2 | 任务侧适配（小红书/X 的 worker，可先放独立仓库） |
| P2 | GUI：按平台筛选、账号状态展示、批量导入账号 |
| P3 | 可选第二引擎 Camoufox、同步/多机、更细风控策略 |

### 非目标（第一期不做）

- 不 fork / 重编 Chromium 源码
- 不自研完整代理协议栈（用上游 xray/sing-box/mihomo）
- 不把爬虫业务逻辑写死进桌面壳（worker 外置）
- 不追求一上来替代 AdsPower 全部企业功能

---

## 1. 仓库与分支策略

### 1.1 建立 fork

```text
github.com/<you>/ant-browser   # 或私有名，如 profile-forge
upstream: black-ant/Ant-Browser
```

```bash
# 1) GitHub 上 Fork black-ant/Ant-Browser
# 2) 本地
git clone git@github.com:<you>/Ant-Browser.git
cd Ant-Browser
git remote add upstream https://github.com/black-ant/Ant-Browser.git
git fetch upstream
git checkout -b develop   # 日常开发
```

### 1.2 分支模型

| 分支 | 用途 |
|------|------|
| `upstream-sync` | 只跟进上游，定期 merge，**尽量零业务改动** |
| `develop` | 主开发线 |
| `feat/*` | 单功能分支（账号池、API、UI） |
| `release/*` | 可打包发布 |

**硬规则：**

- 业务改动尽量集中在 `backend/internal/...` 新增包，例如 `accountpool/`
- 少改启动链路核心；能包装 API 就不改启动原语
- 每次同步上游：`upstream/master → upstream-sync → develop`，冲突优先保住上游启动/代理稳定性

### 1.3 品牌与路径（建议尽早做）

避免和官方安装包抢数据目录：

- 应用名：`AntBrowser` → 例如 `MyAnt` / `ProfilePool`
- 数据目录：`ant-browser` → 你的 app id（macOS `Application Support/...` 等）
- 端口/单例锁：与官方默认错开，避免并装冲突

第一周可以先不改名，**验证能跑之后立刻改**，否则后期迁移 user-data 很痛苦。

---

## 2. 现状资产（fork 后直接继承）

| 模块 | 已有能力 | 对你的价值 |
|------|----------|------------|
| 实例管理 | CRUD、克隆、分组、标签、批量启停 | 多账号环境 |
| 代理池 | 导入/测速/健康/绑定实例 | 一账号一出口 |
| 内核管理 | 多 Chrome、默认内核 | 接 fingerprint-chromium |
| Launch API (1.2+) | 按 code/selector 启停、session、**统一 CDP** | 爬虫接入点 |
| 自动化脚本 (1.3) | 脚本包、目标实例、执行记录 | 轻量编排 |
| 实例导入导出 | 含完整 user-data | 备份/迁移账号环境 |
| 连接栈 | xray / mihomo 规则明确 | 代理稳定性 |

**要新增的核心：账号与账号池语义（Account Pool）。**  
实例（Instance）= 浏览器环境；账号（Account）= 业务身份。两者绑定，概念分开。

### 上游参考版本

- 建议 baseline：`v1.3.0`（或 fork 时最新 release）
- 连接栈文档：`docs/proxy-connector-stacks.md`
- 开发：Windows `bat\dev.bat`；Linux/macOS 见上游 README
- 内核目录示例：`chrome/chrom-142/chrome.exe`（macOS/Linux 为对应二进制名）

---

## 3. 目标架构

```text
┌──────────────────────────────────────────────────────────┐
│  GUI (Wails + 现有前端)                                    │
│  - 实例列表（增强：平台/账号状态）                           │
│  - 账号池面板（可选 P2）                                    │
└────────────────────────────┬─────────────────────────────┘
                             │
┌────────────────────────────▼─────────────────────────────┐
│  Ant Backend (Go)                                         │
│  ┌─────────────┐  ┌──────────────┐  ┌─────────────────┐  │
│  │ instances   │  │ proxies      │  │ kernels         │  │  ← 上游已有
│  └──────┬──────┘  └──────┬───────┘  └────────┬────────┘  │
│         │                │                    │           │
│  ┌──────▼────────────────▼────────────────────▼────────┐  │
│  │ accountpool (新增)                                    │  │
│  │  accounts / leases / cooldowns / events               │  │
│  │  API: lease / release / heartbeat / start+cdp         │  │
│  └───────────────────────┬──────────────────────────────┘  │
│                          │ 调用现有 Launch/CDP              │
└──────────────────────────┼───────────────────────────────┘
                           │ spawn
              fingerprint-chromium (+ user-data-dir)
                           ▲
              Playwright Worker (独立仓库/目录)
              xhs_worker / x_worker
```

**设计约定：**

```text
Account 1 ──绑定──► Instance 1 ──绑定──► Proxy 1
                 └─ user-data 持久化登录态
                 └─ fingerprint 启动参数尽量固定
```

- **人工使用**：GUI 启动 Instance
- **爬虫使用**：Worker 调 `lease` → 内部 start → 返回 `cdp_url` → 任务结束 `release`
- **禁止**：两个任务同时 lease 同一账号；人工正在用的实例默认不可被爬虫抢

### 账号池核心规则（比浏览器选型更重要）

1. **固定绑定**：`account_id → fingerprint_seed → proxy_id → user_data_dir`
2. **租约**：任务领取账号 → 加锁 → 用完释放；失败进冷却
3. **代理与指纹一致**：时区/语言/geo 跟代理出口走
4. **频率与行为**：指纹只是门槛；限速、会话时长、拟人操作同样关键
5. **双通道**：人工用 GUI 开 Profile；爬虫只走 API + CDP，不共用正在人工操作的实例

---

## 4. 数据模型（建议新增表）

在现有 `app.db` 上扩展（迁移脚本单独做）。

### 4.1 `accounts`

| 字段 | 说明 |
|------|------|
| `id` | 主键 |
| `platform` | `xhs` / `x` / `other` |
| `username` / `display_name` | 展示用 |
| `instance_id` | 绑定的浏览器实例 |
| `proxy_id` | 可冗余，以 instance 绑定为准 |
| `status` | `active` / `cooldown` / `banned` / `need_login` / `disabled` |
| `fingerprint_seed` | 若启动参数支持，固定写入 instance 配置 |
| `notes` / `tags` | 备注 |
| `last_used_at` / `cooldown_until` | 调度用 |
| `meta_json` | 扩展（cookie 校验时间、风控备注等） |

### 4.2 `account_leases`

| 字段 | 说明 |
|------|------|
| `id` | 租约 ID |
| `account_id` | |
| `worker_id` | 哪个爬虫进程 |
| `purpose` | `manual` / `scrape` / `warmup` |
| `status` | `held` / `released` / `expired` / `stolen` |
| `cdp_endpoint` | 可选缓存 |
| `leased_at` / `expires_at` / `heartbeat_at` | 租约与心跳 |

### 4.3 `account_events`（可选）

审计：登录成功、验证码、403、封禁、释放原因。

**原则：** 不推翻上游 `instances` 表；账号是**挂在 instance 上的业务视图**。

---

## 5. API 设计（账号池层）

在上游 Launch API 之上包一层，路径示例（实现时按仓库现有路由风格调整）：

| 方法 | 路径 | 作用 |
|------|------|------|
| `POST` | `/api/v1/pool/accounts` | 创建账号并可选自动建 instance |
| `GET` | `/api/v1/pool/accounts?platform=xhs&status=active` | 列表 |
| `POST` | `/api/v1/pool/lease` | 租一个可用账号 |
| `POST` | `/api/v1/pool/lease/:id/heartbeat` | 续租 |
| `POST` | `/api/v1/pool/lease/:id/release` | 释放（可带 result: ok/risk/ban） |
| `POST` | `/api/v1/pool/accounts/:id/start` | 启动绑定实例，返回 CDP |
| `POST` | `/api/v1/pool/accounts/:id/stop` | 停止 |

### 5.1 `lease` 请求示例

```json
{
  "platform": "xhs",
  "worker_id": "scraper-mac-01",
  "ttl_sec": 900,
  "auto_start": true,
  "tags_any": ["pool-a"]
}
```

### 5.2 `lease` 响应示例

```json
{
  "lease_id": "l_xxx",
  "account_id": "a_01",
  "instance_code": "xhs-01",
  "cdp_url": "http://127.0.0.1:9222",
  "proxy_summary": "socks5://…",
  "expires_at": "2026-07-19T12:00:00Z"
}
```

Worker 侧只依赖这一层，**不直接操作上游内部表**。

### 5.3 Worker 最小伪代码

```python
lease = post("/pool/lease", {"platform": "xhs", "auto_start": True})
browser = playwright.chromium.connect_over_cdp(lease["cdp_url"])
# ... 业务 ...
post(f"/pool/lease/{lease['lease_id']}/release", {"result": "ok"})
```

### 5.4 release 结果 → 状态

| result | 账号状态 |
|--------|----------|
| `ok` | idle / active |
| `risk` | cooldown 30–120 min |
| `ban` | banned |
| `need_login` | need_login |

---

## 6. 分阶段计划

### Phase 0 — 基线跑通（约 2–4 天）

**产出：** 本机可开发、可启动多实例、CDP 可连。

1. Fork + 装依赖（Go、Node、Wails 版本按仓库要求）
2. `master` 干净启动；Windows 用 `bat\dev.bat`，macOS/Linux 按 README
3. 下载 **fingerprint-chromium**，在「内核管理」登记路径
4. 建 2 个实例 + 2 个代理，验证：
   - 指纹/环境隔离（BrowserLeaks）
   - IP 出口不同
5. 读清上游 **Launch API / CDP** 文档与路由实现（`backend` 内搜 launch/cdp）
6. 用 Playwright 最小脚本：`start instance → connect_over_cdp → 打开页面`
7. 记录：API base URL、鉴权方式、CDP 字段名、默认端口

**验收：**

- [ ] GUI 启停 2 实例正常
- [ ] 外部脚本拿到 CDP 并操作页面
- [ ] 文档笔记：`docs/fork/00-baseline.md`（在 fork 仓库内自建）

---

### Phase 1 — Fork 卫生与可维护性（约 1–2 天）

**产出：** 可长期跟上游的工程形态。

1. 改应用 id / 数据目录 / 窗口标题（避免与官方冲突）
2. 加 `docs/fork/`：架构、分支、同步流程
3. 明确 License 风险：上游暂无 LICENSE → **仅私用或联系作者**；对外分发前务必处理
4. CI：至少 `go test` / 前端 build（能加则加）
5. 固定上游版本 tag（如 `v1.3.0`）作为 baseline，写在 README

**验收：**

- [ ] 与官方 Ant 可并存安装
- [ ] `upstream-sync` 流程写清楚

---

### Phase 2 — 账号模型 + GUI 轻量增强（约 3–5 天）

**产出：** 「实例」能表达「某平台某账号」。

1. DB migration：`accounts` 表
2. 创建实例时可填：`platform`、`username`、`fingerprint_seed`
3. 实例列表列：平台、账号状态、最后使用时间
4. 筛选：按 `platform` / `status`
5. 规范命名：`xhs-acc-03`、`x-acc-01` 等 code 规则

**暂不实现**完整租约，先把元数据挂上。

**验收：**

- [ ] 10 个假账号实例可管理、可筛选
- [ ] 每个账号绑定固定代理

---

### Phase 3 — 账号池 API（约 5–7 天）**【核心】**

**产出：** 爬虫可安全租用账号。

1. 实现 `lease / heartbeat / release`
2. 锁：DB 事务或 `SELECT … FOR UPDATE`，防止双租
3. `auto_start=true` 时串起上游 start + 返回 CDP
4. 租约超时回收（后台 ticker）
5. release 结果驱动状态（见 5.4）
6. 互斥：GUI 手动启动时可标记 `manual_hold`，pool 不可抢

**验收：**

- [ ] 两个 worker 并发 lease，不会拿到同一 account
- [ ] 租约过期后账号自动回到可租
- [ ] Playwright 全流程：lease → cdp → 打开站点 → release

---

### Phase 4 — 爬取侧独立 Worker（约 3–7 天，可并行）

**产出：** 业务与桌面壳解耦。

建议**另建仓库** `ant-pool-workers`：

```text
workers/
  common/pool_client.py
  xhs/scrape_notes.py
  x/scrape_timeline.py
```

规范：

- 只调 pool API
- 限速、重试、验证码/登录失效上报
- 日志带 `account_id` / `lease_id`
- 不在浏览器里清 cookie 换号（换号 = 换 lease）

**验收：**

- [ ] 至少 1 个平台的 smoke 任务跑通
- [ ] 账号异常会写回 pool 状态

---

### Phase 5 — 体验与稳态（约 3–5 天）

1. GUI「账号池」页：状态、冷却倒计时、强制释放
2. 批量导入：CSV（platform, username, proxy_name, notes）→ 批量建 instance+account
3. 健康检查：代理失败自动 cooldown
4. 备份：定时导出实例 ZIP / DB
5. 可选：Camoufox 作为实验引擎（P3，不阻塞主线）

---

## 7. 里程碑时间表（单人全职粗估）

| 周次 | 里程碑 |
|------|--------|
| W1 | Phase 0–1：跑通 + fork 卫生 + CDP 验证 |
| W2 | Phase 2：账号模型 + 列表增强 |
| W3 | Phase 3：lease/CDP 闭环 |
| W4 | Phase 4：1 个平台 worker + 稳态修补 |
| W5+ | Phase 5 + 第二平台 + 可选第二引擎 |

兼职可按 ×1.5–2 拉长。

---

## 8. 目录级改动建议（减少与上游冲突）

```text
backend/
  internal/
    accountpool/          # 新增：领域逻辑
      model.go
      service.go
      lease.go
      api.go
    ...                   # 少改现有包
frontend/
  src/
    pages/account-pool/   # 新增页面，少动老页面
docs/
  fork/
    00-baseline.md
    01-architecture.md
    02-pool-api.md
    03-upstream-sync.md
migrations/               # 或沿用项目现有迁移方式
  00x_accounts.sql
```

**禁止一上来大改：** 代理连接栈、单例锁、Wails 生命周期——这些是上游最易踩坑的区域。

---

## 9. 技术风险与对策

| 风险 | 对策 |
|------|------|
| 上游无明确 License | 私有自用优先；公开分发前确认授权 |
| 上游快速演进，merge 冲突 | 业务隔离在 `accountpool`；定期小步同步 |
| CDP 与指纹内核兼容 | Phase 0 用 fingerprint-chromium 实测；记录可用版本 |
| 代理带认证 | 走 Ant 连接栈，不要假设 chrome `--proxy-server` 支持账密 |
| 账号被风控 | 固定四元组 + 冷却 + 降频；指纹只是一环 |
| 人工与爬虫抢同一实例 | `manual_hold` / lease 互斥 |
| macOS unsigned / 签名 | 开发机先跑通；发布再处理 notarize |

---

## 10. 验收清单（Definition of Done）

### 需求 1：多账号日常

- [ ] 同平台 ≥5 账号，各有独立实例
- [ ] 切换 = 启停不同实例，无共用 Cookie
- [ ] 每实例固定代理，IP 检测一致
- [ ] 可按平台筛选、Ctrl+K 快速打开

### 需求 2：爬取账号池

- [ ] `lease → CDP → 操作 → release` 稳定
- [ ] 并发不双开同一账号
- [ ] 失败进入 cooldown / need_login
- [ ] Worker 与 GUI 数据一致

---

## 11. 第一周具体 To-Do

1. GitHub Fork `black-ant/Ant-Browser`
2. 本机编译启动 + 安装 fingerprint-chromium
3. 创建 `xhs-demo-1`、`xhs-demo-2` 两实例并绑代理
4. 找到 Launch/CDP API 的真实路径与鉴权，写进 `docs/fork/00-baseline.md`
5. Playwright 10 行脚本验证 CDP
6. 画 `accounts` / `leases` 表最终字段（评审后再写 migration）
7. 建分支 `feat/account-model`，只加表 + 只读 API，不碰启动逻辑

---

## 12. 成功形态（约 4 周后）

> **一个改名后的 Ant 系桌面端** + **账号池 HTTP API** + **外置 Playwright Worker**  
> 人工多开靠 GUI；爬虫靠 lease/CDP；指纹靠 fingerprint-chromium；代理靠上游连接栈。

而不是：一个难以合并上游的巨型魔改 monorepo。

---

## 13. 相关开源参考（选型上下文）

| 角色 | 项目 | 说明 |
|------|------|------|
| 编排基座（本计划 fork 对象） | [Ant-Browser](https://github.com/black-ant/Ant-Browser) | 实例/代理/内核/Launch API/CDP |
| 引擎 | [fingerprint-chromium](https://github.com/adryfish/fingerprint-chromium) | CLI 指纹 seed，自动化友好 |
| 爬取加强（可选 P3） | [Camoufox](https://github.com/daijro/camoufox) | Firefox 源码级反检测 + Playwright |
| 产品参考（不 fork） | [Donut Browser](https://github.com/zhom/donutbrowser) | Profile/API/MCP 完整，AGPL |
| 即用参考（不 fork） | [VirtualBrowser](https://github.com/Virtual-Browser/VirtualBrowser) | Windows 多开产品型 |

---

## 14. 合规提醒

- 多账号隔离、自动化采集可能违反平台 ToS，请只在合法授权范围内使用。
- 指纹浏览器降低关联概率，不能保证不封号；IP 质量与行为模式往往更关键。
- 小红书、X 对异常频率、设备突变、新环境登录敏感——**账号与环境固定**比「每次换马甲」更有效。

---

## 15. 后续实现入口（给其他对话用）

实现时建议按顺序：

1. **Phase 0 体检**：clone fork，对照仓库真实 Launch/CDP API 路径补全 `docs/fork/00-baseline.md`
2. **数据模型**：`accounts` / `account_leases` 的 migration + Go 结构体
3. **Pool API**：OpenAPI/路由 + Playwright 客户端骨架
4. 再进入 GUI 增强与 worker 仓库

本计划文档路径：

```text
docs/fork-ant-browser-plan.md
```
