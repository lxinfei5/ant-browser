# 账号数据模型草图 (accounts / account_leases / account_events)

> 状态: **待评审，先不写 migration**。本草案仅描述表结构，尚未落为 SQLite migration。
>
> 依据: `docs/fork-ant-browser-plan.md` §4.1（accounts）、§4.2（account_leases）、§4.3（account_events）。

## 与现有存储层一致的约定 (来自 db-storage 审计)

新表须与 `backend/internal/database/sqlite.go` 现有风格一致，以便以 migration v13 追加：

- **主键**: TEXT（UUID 字符串，`github.com/google/uuid` 生成）。仅 `browser_bookmarks` 用 INTEGER AUTOINCREMENT，新表沿用 TEXT/UUID 主流约定。
- **时间戳**: DATETIME 列，DAO 写 `time.Now().Format(time.RFC3339)`（profile/group/proxy 约定）；DB 默认 `CURRENT_TIMESTAMP`。注意 `launch_codes` 用 `YYYY-MM-DD HH:MM:SS` UTC——代码库不一致，新表**建议统一用 RFC3339**（`待评审`）。
- **布尔**: INTEGER `0`/`1`。
- **JSON 列**: TEXT，默认 `'[]'`（数组）或 `'{}'`（对象）；Go 侧 `json.Marshal`/`json.Unmarshal`。
- **软删除**: `deleted_at TEXT NOT NULL DEFAULT ''`（`''` = 在用），非可空时间戳。
- **新列向后兼容**: 后加列须 `NOT NULL` + 字面 `DEFAULT`，SELECT 读时用 `COALESCE(col, <default>)`（见 `profile_dao.go:38`）。
- **外键**: `PRAGMA foreign_keys=ON` 已开，但现有表**均未声明 FOREIGN KEY**，级联在 Go 代码中手动处理。新表可选: 沿用手动级联（现有约定）或引入显式 `FOREIGN KEY ... ON DELETE CASCADE`（`待评审`）。
- **无 ORM**: raw SQL + `INSERT ... ON CONFLICT DO UPDATE` upsert，DAO 为 `*sql.DB` 薄封装。

---

## accounts（账号表，plan §4.1）

> 一个 account 代表一个被托管的外部账号（如某平台的登录账号），与浏览器实例/Profile 配合使用。

> 与 plan §4 的字段映射: plan 的 `instance_id` → `bound_profile_id`（Ant-Browser 中“实例”= `browser_profiles` 的 `Profile`）；plan 的 `fingerprint_seed` 不存于 account——审计确认种子由 `ProfileId` 哈希确定性派生并注入 `--fingerprint=<seed>`（`app_instance_start_prepare.go:282`），即指纹固定随绑定 Profile 走，account 不重复存；plan 的 `meta_json` → `metadata_json`；plan 的 `username`/`display_name` → `account_ref` + `account_name`。

| column | type | purpose |
|---|---|---|
| `account_id` | TEXT PRIMARY KEY | UUID 主键（plan `id`） |
| `account_name` | TEXT NOT NULL | 人类可读名称（plan `display_name`） |
| `platform` | TEXT NOT NULL DEFAULT '' | 账号所属平台标识，如 `xhs`/`x`/`other`（plan `platform`） |
| `account_ref` | TEXT NOT NULL DEFAULT '' | 平台内账号标识，如用户名/邮箱/UID（plan `username`） |
| `bound_profile_id` | TEXT NOT NULL DEFAULT '' | 绑定的实例 ProfileId（plan `instance_id`）；冗余于 lease，便于直查 |
| `proxy_id` | TEXT NOT NULL DEFAULT '' | 绑定代理 ID（plan `proxy_id`；可冗余，以 instance 绑定为准） |
| `status` | TEXT NOT NULL DEFAULT 'active' | 账号状态（plan §4.1 枚举: `active`/`cooldown`/`banned`/`need_login`/`disabled`） |
| `cooldown_until` | TEXT NOT NULL DEFAULT '' | 冷却到期时间 RFC3339（plan `cooldown_until`；release result=risk 时写入，调度用） |
| `notes` | TEXT NOT NULL DEFAULT '' | 备注（plan `notes`） |
| `tags` | TEXT NOT NULL DEFAULT '[]' | 标签（JSON 数组，沿用 profiles.tags 约定；plan `tags`） |
| `group_id` | TEXT NOT NULL DEFAULT '' | 分组（复用 browser_groups；`待评审` 是否共用） |
| `credential_json` | TEXT NOT NULL DEFAULT '{}' | 凭据/会话快照（JSON；`待评审`：是否加密存储） |
| `metadata_json` | TEXT NOT NULL DEFAULT '{}' | 任意扩展元数据（JSON；plan `meta_json`，如 cookie 校验时间、风控备注） |
| `last_used_at` | TEXT NOT NULL DEFAULT '' | 最近一次使用时间 RFC3339（plan `last_used_at`） |
| `created_at` | DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP | 创建时间（RFC3339 写入） |
| `updated_at` | DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP | 更新时间 |
| `deleted_at` | TEXT NOT NULL DEFAULT '' | 软删除（'' = 在用） |

索引（建议）: `idx_accounts_platform`、`idx_accounts_status`、`idx_accounts_bound_profile`、`idx_accounts_deleted_at`。

---

## account_leases（账号租约表，plan §4.2）

> 一个 lease 记录某 account 在某时段被某 Profile（实例）占用，用于多账号轮换/防撞。同一账号同时至多一个 active lease。

| column | type | purpose |
|---|---|---|
| `lease_id` | TEXT PRIMARY KEY | UUID 主键（plan `id`） |
| `account_id` | TEXT NOT NULL | 关联 accounts.account_id |
| `profile_id` | TEXT NOT NULL | 关联 browser_profiles.profile_id（占用账号的实例） |
| `worker_id` | TEXT NOT NULL DEFAULT '' | 领取租约的爬虫/worker 进程标识（plan `worker_id`，如 `scraper-mac-01`） |
| `purpose` | TEXT NOT NULL DEFAULT 'scrape' | 用途枚举（plan: `manual`/`scrape`/`warmup`） |
| `status` | TEXT NOT NULL DEFAULT 'held' | 租约状态枚举（plan §4.2: `held`/`released`/`expired`/`stolen`） |
| `cdp_endpoint` | TEXT NOT NULL DEFAULT '' | 缓存的 CDP 入口 URL（plan `cdp_endpoint`，可选；来自启动响应 `cdpUrl`） |
| `leased_at` | DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP | 租约开始时间 RFC3339（plan `leased_at`） |
| `expires_at` | TEXT NOT NULL DEFAULT '' | 预期到期时间 RFC3339（plan `expires_at`；'' = 不限） |
| `heartbeat_at` | TEXT NOT NULL DEFAULT '' | 最近心跳时间 RFC3339（plan `heartbeat_at`；`/lease/:id/heartbeat` 续租时更新） |
| `released_at` | TEXT NOT NULL DEFAULT '' | 释放时间（'' = 仍在用；release 时写） |
| `release_result` | TEXT NOT NULL DEFAULT '' | 释放结果（plan §5.4: `ok`/`risk`/`ban`/`need_login`；驱动 account 状态机） |
| `metadata_json` | TEXT NOT NULL DEFAULT '{}' | 租约扩展元数据（JSON） |
| `created_at` | DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP | 创建时间 |
| `updated_at` | DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP | 更新时间 |

约束/索引（建议）: `idx_leases_account_id`、`idx_leases_profile_id`、`idx_leases_status`、`idx_leases_worker_id`；活跃唯一性（同 account 同时至多一个 `held` lease）`待评审`——由应用层事务/`SELECT ... FOR UPDATE` 保证，或加部分唯一索引 `WHERE status='held'`。

---

## account_events（账号事件表，plan §4.3）

> 追加式事件日志，记录账号生命周期事件（登录/登出/检查/封禁/租约变更等）。

| column | type | purpose |
|---|---|---|
| `event_id` | TEXT PRIMARY KEY | UUID 主键 |
| `account_id` | TEXT NOT NULL | 关联 accounts.account_id |
| `lease_id` | TEXT NOT NULL DEFAULT '' | 关联 account_leases.lease_id（如适用） |
| `profile_id` | TEXT NOT NULL DEFAULT '' | 触发事件的实例（如适用） |
| `event_type` | TEXT NOT NULL DEFAULT '' | 事件类型（如 `login_success`/`captcha`/`http_403`/`banned`/`lease_acquired`/`lease_released`/`cooldown_started`；`待评审` 枚举） |
| `event_status` | TEXT NOT NULL DEFAULT '' | 事件结果状态（success/failure；`待评审`） |
| `payload_json` | TEXT NOT NULL DEFAULT '{}' | 事件载荷（JSON，含错误信息/原始响应等） |
| `occurred_at` | DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP | 事件发生时间（RFC3339） |

索引（建议）: `idx_events_account_id`、`idx_events_occurred_at`、`idx_events_event_type`。事件表只追加，不软删。

---

## 待评审事项

- accounts 的 `credential_json` 是否需要静态加密（db-storage 审计: proxy 凭据是否加密未确认，需查 ProxyDAO schema）。
- account_leases 是否引入首批真正的 FOREIGN KEY 约束（`foreign_keys=ON` 已开）。
- `bound_profile_id` 与 account_leases 的职责划分（绑定关系存 accounts 还是仅靠 leases 表达）。
- 时间戳统一为 RFC3339 还是沿用 launch_codes 的 `YYYY-MM-DD HH:MM:SS`。
- tags 是否复用 browser_groups / 是否需要独立 tags 表（现有 profiles.tags 为 JSON 列，无独立 tags 表）。
- status 字段枚举值。

> 落地前需对照 `docs/fork-ant-browser-plan.md` §4 复核字段语义；确认后再写 migration v13。