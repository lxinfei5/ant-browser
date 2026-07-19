# Fork 文档索引

Ant-Browser fork（`github.com/lxinfei5/Ant-Browser`，基线上游 tag `V1.2.0`）的 Phase 0 文档集。

## 分支模型

- `develop` — fork 日常开发分支
- `upstream-sync` — 跟踪上游的同步分支
- `master` — 主干

上游同步流程: `fetch upstream → upstream-sync → develop`（先在 `upstream-sync` 上合并上游，再合入 `develop`，避免污染 `develop`）。

## 文档

| 文件 | 说明 |
|---|---|
| `00-baseline.md` | Phase 0 基线：仓库/分支、工具链版本、启动命令、内核管理、实例模型、代理、Launch/CDP API、鉴权、Playwright 接入、运行时待验证清单、人工验证 checklist |
| `scripts/cdp_probe.py` | 最小可运行 Playwright CDP 探针脚本（Python），调 `/api/runtime/session` 取 `cdpUrl`，经 CDP 打开页面并打印标题/指纹提示，再调 `/api/runtime/stop` |
| `data-model-sketch.md` | accounts / account_leases / account_events 表结构草图（待评审，先不写 migration） |
| `SECURITY-AUDIT.md` | 对抗式安全审计报告（78 条 confirmed：1 critical / 14 high / …；含二进制来源人工核验与 fork 修复建议） |

## 相关

- 计划文档: `docs/fork-ant-browser-plan.md`（data-model-sketch.md 依据其 §4）