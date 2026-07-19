# 03 — 上游同步流程（ProfilePool fork）

> 上游：`https://github.com/black-ant/Ant-Browser`
> 基线 tag：`V1.2.0`（fork 时的起点；后续上游 release 按需跟进）
> 相关：`docs/fork/README.md`（分支模型）、`docs/fork-ant-browser-plan.md` §1.2（分支策略）

本文给出可复制粘贴的上游同步命令，以及冲突解决与安全加固回合后的再同步规则。

---

## 1. 分支模型

| 分支 | 用途 |
|---|---|
| `upstream-sync` | 只跟进上游，定期 merge，**尽量零业务改动** |
| `develop` | fork 主开发线（业务改动落地处） |
| `feat/*` | 单功能分支（账号池、API、UI） |
| `release/*` | 可打包发布 |

硬规则（plan §1.2）：

- **业务改动尽量集中在 `accountpool/` 等新增包**（`backend/internal/...` 下）。
- **少改启动链路核心**（`launchcode/`、`app_instance_start_*`、`proxy/`）；能包装上游 Launch/CDP API 就不改启动原语。
- 每次同步上游：`upstream/master → upstream-sync → develop`，冲突优先保住上游启动/代理稳定性。

---

## 2. 一次性：配置 upstream remote

```bash
cd /path/to/Ant-Browser
git remote add upstream https://github.com/black-ant/Ant-Browser.git
git fetch upstream
# 仅需做一次
```

> 仓库本地 clone 的 origin 指向 fork（`github.com/lxinfei5/Ant-Browser`），upstream 指向原作者。

---

## 3. 标准上游同步流程（可复制粘贴）

前置：工作区干净；`develop` 与 `upstream-sync` 已存在。

```bash
# 0. 拉取所有远端最新
git fetch --all --prune

# 1. 在 upstream-sync 上合并上游 master
git checkout upstream-sync
git merge upstream/master
# → 如有冲突，按 §4 规则解决；解决后 git add + git commit 完成 merge

# 2. 推送 upstream-sync（备份同步分支）
git push origin upstream-sync

# 3. 把同步结果合入 develop（--no-ff 保留合并节点，便于追溯上游回合）
git checkout develop
git merge --no-ff upstream-sync -m "chore(sync): merge upstream/master into develop"

# 4. 推送 develop
git push origin develop
```

> 若 `upstream-sync` 或 `develop` 不存在，先从基线 tag 建一次：
> ```bash
> git fetch upstream
> git checkout -b upstream-sync upstream/master   # 或 git checkout V1.2.0 后再建
> git checkout -b develop upstream-sync
> git push -u origin upstream-sync develop
> ```

---

## 4. 冲突解决优先级

合并 `upstream/master` 时如遇冲突，按以下优先级取舍：

1. **上游启动 / 代理稳定性优先**：`launchcode/`、`app_instance_start_*`、`browser_launch_args`、`proxy/`（xray/mihomo/sing-box 桥、运行时校验）、`netguard`、`runtime_verify` 等核心 —— **采纳上游版本**，除非上游改动与 fork 的安全加固（`docs/fork/SECURITY-AUDIT.md` 中 confirmed 的修复）冲突，此时按 §5 处理。
2. **fork 身份改造保留**：`wails.json`、`config.yaml`、`build/darwin/Info.plist`、`single_instance*`、`backend/internal/apppath/apppath.go`（`appStateDirName`）、`backend/app.go`（`appName`）、`backend/internal/config/config_defaults.go`（`AppConfig.Name`）、`backend/internal/tray/tray.go`、`publish/**`（bundle/binary/installer 名）、`frontend/index.html`、`frontend/src/config/projectBase.config.ts`、`frontend/src/modules/settings/types.ts` 等 —— **保留 fork 的 ProfilePool 身份**，不要被上游的 “Ant Browser” 字面量覆盖。
3. **业务层（accountpool/）保留 fork**：fork 新增的 `accountpool/` 包、`/api/v1/pool/*` 路由、`accounts`/`account_leases`/`account_events` 表与迁移 —— **保留 fork**，上游无此文件时无冲突。
4. **Go 模块路径 `ant-chrome` 保留**：`go.mod` 第 1 行 `module ant-chrome` 与所有 `ant-chrome/backend/...` import 路径 —— **保持不变**（rename 模块路径超出 Phase 1 范围，会触及每个文件）。
5. 其余（UI 细节、文档、工具脚本）—— 按需，倾向采纳上游以减少长期分歧。

---

## 5. 安全加固回合后的再同步

fork 在 Phase 0 已做对抗式安全审计（`docs/fork/SECURITY-AUDIT.md`，78 条 confirmed：1 critical / 14 high / …）并按计划合入安全加固。这些改动可能触及上游启动/代理核心（如 runtime 校验 `runtime_verify.go`、SSRF `netguard.go`、单例锁、CDP 鉴权等）。再同步上游时：

1. **先在 `upstream-sync` 上 merge `upstream/master`**（同 §3 步骤 1）。
2. **冲突比对 fork 的安全加固清单**：逐项确认上游是否已修复同一问题或引入回归。fork 安全加固的文件清单以 `docs/fork/SECURITY-AUDIT.md` 的 “fork 修复建议” 为准。
3. **上游已修复同一问题** → 采纳上游实现，删除 fork 重复加固，更新 `SECURITY-AUDIT.md` 条目状态。
4. **上游未修复 / 上游引入新风险** → 保留 fork 加固；若上游改动与之同文件冲突，按以下方式收敛：
   - 优先在 `accountpool/` 或包装层隔离，避免直接改上游核心文件；
   - 必须改核心文件时，在改动处加注释标记 `// ProfilePool fork: <audit-id> — 见 docs/fork/SECURITY-AUDIT.md`，便于下次同步快速定位。
5. **解决后**，在 `develop` 上 `merge --no-ff upstream-sync`（同 §3 步骤 3），并跑一遍 CI（`.github/workflows/ci.yml`：`go vet` + `go test ./backend/... ./backend/internal/...` + 前端 `npm ci && npm run build`）确认未回归。

---

## 6. 速查（一行版）

```bash
git fetch --all --prune && \
git checkout upstream-sync && git merge upstream/master && \
git push origin upstream-sync && \
git checkout develop && git merge --no-ff upstream-sync -m "chore(sync): merge upstream/master into develop" && \
git push origin develop
```

> 冲突时停下，按 §4 优先级解决；安全加固相关冲突按 §5 处理。