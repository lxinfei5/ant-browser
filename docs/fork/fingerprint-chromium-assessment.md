# fingerprint-chromium 安全与量级评估

- 日期: 2026-07-19
- 评估版本: 148.0.7778.215 (arm64 / macOS DMG)
- 使用场景: 用户已安装 arm64 预编译版本,在 ProfilePool 托管实例内运行(每个账号 = 一个独立 user-data-dir;二进制在本地 Mac 上运行)。

---

## 项目量级 — hard numbers

### adryfish/fingerprint-chromium(被评估项目)

| 指标 | 数值 | 来源 |
|---|---|---|
| Stars | 2,816 | `gh api repos/adryfish/fingerprint-chromium` |
| Forks | 411 | 同上(forks_count / network_count) |
| Watchers/subscribers | 33(subscribers) | 同上 |
| Open issues | 22 | `gh api search/issues` |
| Closed issues | 58 | 同上(**合计 80 条 issue**,非此前流传的 85) |
| Open PRs | 0 | 同上 |
| Closed PRs(历史累计) | 2 | 同上 |
| 贡献者 | **1**(仅 adryfish) | `/contributors` 数组长度 1 |
| 累计 commits | 13(全部由 adryfish 提交) | `/commits?per_page=100` |
| Releases | 13,从 v129.0.6668.100(2024-12-09)到 v148.0.7778.215(2026-06-21) | `/releases` |
| 发版节奏 | ~每月 1 次,跟随 Chromium 版本号 | 同上 |
| 仓库创建 | 2024-12-09 | `created_at` |
| 最近 push | 2026-06-21 | `pushed_at` |
| License | BSD-3-Clause | `license.spdx_id` |
| 是否 GitHub fork | 否(parent/source = null) | repo metadata |
| 仓库内容 | 仅 LICENSE / README.md / README-ZH.md / qqgroup.png(Linguist 语言统计为空 `{}`) | `/contents` + `/languages` |
| 维护者 adryfish | GitHub 账号 2024-05-17 注册(~2.17 年),5 个公开仓库,88 followers,无 name/bio/company | `gh api users/adryfish` |
| adryfish 其他仓库 | llm-web-api(133★)、reclaude(86★)、recodex(59★)、reclaude-code(35★),均为 LLM/代理工具,体量远小于本项目 | `users/adryfish/repos` |

**定性**: 高 stargazer 采纳度(2.8k),但**实质上是一人维护的小众项目**——1 贡献者、13 commits、0 open PR、历史仅 2 个 closed PR,提交节奏稀疏(2026-02-27 到 2026-06-21 之间有 ~4 个月空档)。Star 主要来自终端用户对二进制 release 的下载,而非开发者社区。tracked 仓库本身只托管文档与二进制,真正的源码以 patch 形式藏在 release tag 中。

### ungoogled-software/ungoogled-chromium(底层引擎)

| 指标 | 数值 | 来源 |
|---|---|---|
| Stars | 27,173 | `gh api repos/ungoogled-software/ungoogled-chromium` |
| Forks | 1,223 | 同上 |
| Subscribers | 303 | 同上 |
| Open issues | 176 | 同上 |
| 贡献者 | 97(top: Eloston 1,327 commits、Ahrotahn 347、Zoraver 76、PF4Public 57、LeFroid 52) | `/contributors` |
| Releases | 30 | `/releases` |
| 创建时间 | 2015-06-12(约 11 年) | `created_at` |
| 最近 push | 2026-07-17 | `pushed_at` |
| 发版节奏 | 每周多次(最近 5 个 release 全在 2026-07:07-17/07-15/07-09/07-08/07-03) | `/releases` |

**量级裁定**: fingerprint-chromium 是**一人维护的小众项目**,而其底层 ungoogled-chromium 才是**成熟的多维护者社区项目**(27k★、97 贡献者、11 年历史、每周多次 release)。fingerprint-chromium 的浏览器引擎完全依附于后者,自身只加了一层指纹定制 patch。

---

## 构建来源与二进制可信度

核心信任点: **预编译二进制需要完全信任维护者 adryfish 的构建机与发布流程。** 评估发现该流程的可审计性显著弱于同类项目:

- **无 CI/CD 构建流水线**。仓库唯一的 GitHub Actions workflow(`release-on-tag.yml`,在 tag 144 tree 中可见)在 tag push 时只做一件事:用 `actions/create-release@latest` 创建一个**空 release**,完全没有 checkout/编译/upload 步骤;且最近 5 次运行(2026-06-21、2026-02-27、2025-12-23、2025-09-14、2025-08-11)**全部 conclusion=failure**。来源: `gh api repos/.../actions/runs` + `.github/workflows/release-on-tag.yml`。
- **Cirrus 配置只做校验,不编译**。继承自 ungoogled-chromium 的 `.cirrus.yml` 只跑 `code_check` / `validate_config` / `validate_with_source`(validate_patches + validate_lists),没有任何产出 DMG/exe/tar.xz/AppImage 的构建任务。`new_version_check.yml` 被 `if: github.repository == ungoogled-software/ungoogled-chromium` 硬性门控,在此 fork 中永不运行。
- **源码延迟发布且当前缺失**。main 分支不含任何源码或构建脚本(仅 4 个文档/图片文件);源码以 ungoogled-chromium patch 仓形式存在于 release tag 中。README 称 "patch files will be released when the next version is published (typically one month later)"。截至 2026-07-19,**148.0.7778.215 tag 仍只有 4 个 README 文件**,距 2026-06-21 二进制发布已约 4 周,"一个月后放源码"的承诺已接近临界。来源: `git/trees/148.0.7778.215?recursive=1` + README。
- **校验和: 有 SHA-256(经核验修正)**。最初声称"无 checksum 无签名"被核验推翻: `gh api releases/latest` 显示 5 个二进制 asset **各自携带 `digest` 字段 = `sha256:...`**(GitHub asset 元数据形式),例如:
  - windows zip: `sha256:9ef3f471b7a6641b4224532522b29141ce3746e27d55788d88e2fd951f362579`
  - macos dmg: `sha256:b72f091e2e1a7583eed389c4b8e3534ed355e568af8c8bbf8fc30a25e23ca679`
  - 这能校验下载完整性,但**不能证明二进制与源码对应**。
- **签名/出处证明: 无**。release 只有 5 个二进制 asset,**没有任何** `.sig` / `.asc` / `.dgst` / provenance-attestation 文件,无 GPG 签名,无 SLSA 出处。release notes 仅描述 Chrome 148 指纹特性,无任何 provenance 声明。
- **无可复现构建文档**。README build 章节只指向 `ungoogled-chromium/docs/building.md`;grep `reproduc|sign|checksum|gpg|verify` 仅命中延迟源码/自建章节。第三方无法重建并比对哈希。
- **Chromium 版本偏旧**。148.0.7778.215 落后上游 Chromium stable **两个大版本**(mac stable 当前 150.0.7871.129,2026-07-19),约 **2 个月安全补丁缺失**(覆盖 149、150)。来源: Chrome versionhistory API。
- **商业化动机**。README 通过商业 affiliate/服务链接变现: Shared/Dedicated Membership(Claude Code 账号共享/托管)、RapidProxy(折扣码 'adryfish')、OKKproxy(`utm_source=fingerprint&sharecode=43783634`)等。README 中**未提及任何 telemetry/update-check**,且底层 ungoogled-chromium 已剥离 Google 回连。来源: README:59-78, 263-280。

---

## 安全声誉

- **无 CVE、无 GitHub Security Advisory**(`security-advisories` API 返回 `[]`)、无 SECURITY.md(404)。来源: `gh api repos/.../security-advisories`。
- **issue 共 80 条(22 open + 58 closed,经核验修正: 此前 "85 条" 说法不准确)**。抽样 issue 标题(#73 x.com 登录、#39 跨机复制 Google 账号 cookie、#68 proxy 用户名密码)均为功能性需求,未发现恶意/后门/木马类报告。关键词 `malware/virus/backdoor/steal/unsafe/木马/后门` 在 issue 中无安全相关命中。
- **维护者明确拒收款项**,README 第 7 行: "Anyone who contacts you asking for payment is a scammer — do not be fooled.",无 FUNDING.yml(404),issue #74 用户想捐赠但无渠道,issue #84 为垃圾外联。这降低了"为牟利投毒"的动机面,但不构成安全保证。
- **双重用途(anti-detect)特征**: 篡改 canvas/WebGL/audio/fonts/WebRTC/timezone/CPU/deviceMemory、设 `navigator.webdriver=false`、隐藏 CDP 检测。这类反检测特性**可能被部分 AV/EDR 泛化标记为 riskware/PUP**,即便并非恶意。**目前无证据表明其实际恶意**;亦无任何公开数据采集/telemetry 代码披露。
- **声誉基本空白**: WebSearch 对 "fingerprint-chromium review/security/malware/reddit/juejin/v2ex" 无结果;未检索到任何第三方安全审计、VirusTotal 报告或社区评测(待确认: VT 检出比率未能获取,需上传安装包)。
- **AV-flag vs malicious 区分**: 即便某杀软报毒,也极可能是对 anti-detect 行为的泛化启发式告警,而非确凿恶意判定;**当前无任何 reputable 来源将其定性为恶意软件**。

---

## 与同类对比

| 项目 | 引擎 | 维护 | CI 构建/可审计 | 签名/出处 | 可复现 | 版本新旧 |
|---|---|---|---|---|---|---|
| **官方 Chrome** | Chromium | Google 大团队 | 是(内部) | Google 签名 + 自动更新 | 否(闭源构建) | 当前(150) |
| **ungoogled-chromium(官方)** | Chromium | 多维护者(CODEOWNERS: @networkException/@rany2/@clickot/@emilylange 等),97 贡献者 | 有 Cirrus + 公开构建文档 | 无 GPG 签名但有公开 build docs 与较长公开记录 | 部分(有 build docs) | 紧跟上游(每周多次 release) |
| **Camoufox** | Firefox | 团队 | GitHub Actions CI 构建日志公开 + 文档化 deterministic/reproducible builds(Camoufox 侧未在本评估中独立复核,待确认) | 待确认 | 文档化 | 跟随 Firefox |
| **fingerprint-chromium(本项目)** | ungoogled-chromium | **1 人**(adryfish,匿名,2024-05 注册) | **无**(CI 只创建空 release 且每次失败) | **无**(仅 SHA-256 asset digest,无签名/attestation) | **无文档** | 落后 2 个大版本(~2 月补丁) |

**排序(信任由高到低)**: 官方 Chrome > ungoogled-chromium 官方 > Camoufox > fingerprint-chromium。

在同类开源 anti-detect Chromium 项目中,fingerprint-chromium 属**中量级**: CloakHQ/CloakBrowser(28,614★)远大于它;itbrowser-net/undetectable-fingerprint-browser(821★)、ProxyShard/ShardBrowser(534★)、LoseNine/Chromium_FingerPrint_Tutorial(525★)为同量级。来源: `gh search repos fingerprint-chromium`。

---

## 现实风险

**最坏情况(若二进制被植入恶意代码)**: 由于这是自定义编译的 Chromium,对页面内容、存储密码、cookie、token、以及代理凭据有**完整明文访问权**,且所有托管账号的会话都经过它——

- 跨所有 ProfilePool 托管 profile 的**凭据、cookie、token 全量外泄**(Google、社交、广告账号等);
- 代理用户名/密码外泄(issue #68 证实真实使用 proxy 凭据);
- 跨机复制 cookie 的使用模式(issue #39)进一步放大账号接管面;
- 可在任意登录页面注入/篡改内容、静默安装扩展、持久化后门。

**爆炸半径**: 一个恶意二进制 = 所有经其登录的托管账号集体沦陷。每个账号 = 一个独立 user-data-dir 的隔离**仅限 profile 层面**,不能阻止二进制本身跨 profile 读取数据——**隔离的是账号互相之间的数据,不是隔离二进制对每个账号的访问**。

缓解因素(降低但非消除风险):
- 本地运行在用户自己的 Mac(非远端服务器),网络可观测;
- 仓库无 telemetry 证据,且 ungoogled-chromium 基座已剥离 Google 回连,异常外连更易被发现;
- 维护者公开拒收款项(降低直接牟利投毒动机);
- 无任何已知 incident / 恶意报告。

---

## 结论与建议

**裁定**:
- **量级**: 一人维护的小众 anti-detect 浏览器,2.8k★ 主要是终端用户采纳而非开发者社区;引擎完全依附于成熟的 ungoogled-chromium(27k★/97 贡献者)。
- **安全**: 无已知 CVE/恶意报告,anti-detect 特性可能触发 AV 泛化告警但无恶意证据。**核心风险是结构性而非证据性**: 匿名单人维护、无可审计 CI、无签名、无可复现构建、源码延迟且当前缺失、Chromium 落后 2 个大版本。使用预编译 DMG = 完全信任 adryfish 的构建机与发布流程。

**建议(按优先级)**:

1. **优先自行从源码构建**: 等待 148 源码 tag 发布(已逾期临界)后,基于 ungoogled-chromium `docs/building.md` 在受控环境自建,而非使用预编译 DMG。这是唯一能消除"信任维护者构建机"这一核心风险的方式。
2. **若继续使用预编译版**(用户当前情形),采取以下限制:
   - **仅在 ProfilePool 托管的沙箱实例内运行**,绝不登录个人主账号/支付账号/主邮箱;隔离爆炸半径(限制为"可牺牲的运营账号")。
   - **固定版本**: 不要自动升级;每次升级前等源码 tag 发布并 diff patch,再决定是否跟进。
   - **校验 SHA-256**: 下载后比对 GitHub asset digest(`b72f091e2e1a7583eed389c4b8e3534ed355e568af8c8bbf8fc30a25e23ca679` for dmg)。
   - **运行时网络监控**: 用 Little Snitch / lsof 监控异常外连(基座已去 Google 回连,异常外连更显眼)。
   - **macOS 签名检查**: 跑 `codesign -dv --verbose=4 /Applications/Chromium.app` 与 `spctl -a -vv` 确认 DMG 是 ad-hoc 签名还是完全未签名(待确认)。
3. **若 provenance 重要**: 优先迁移到 **ungoogled-chromium 官方构建** 或 **Camoufox**(有公开 CI 构建日志/可复现文档),代价是失去 fingerprint-chromium 的指纹定制能力。
4. **绝对避免**: 用此二进制登录高价值账号(主 Google 账号、支付账号、主邮箱);爆炸半径过大。

**底线**: 当前无证据表明已安装的 arm64 DMG 恶意,但**信任链薄弱**。短期可继续在 ProfilePool 隔离实例内用于可牺牲的运营账号;中期应转向自建或迁移到 provenance 更强的替代品。

---

## 来源

- 仓库与量级: `gh api repos/adryfish/fingerprint-chromium`(+ `/contributors`、`/commits`、`/releases`、`/contents`、`/languages`、`git/trees/main`、`git/trees/148.0.7778.215`、`actions/runs`)
- 维护者: `gh api users/adryfish` + `users/adryfish/repos`
- 上游: `gh api repos/ungoogled-software/ungoogled-chromium`(+ `/contributors`、`/releases`)
- Issue/PR 统计: `gh api search/issues?q=repo:adryfish/fingerprint-chromium`
- 安全公告: `gh api repos/adryfish/fingerprint-chromium/security-advisories`(返回 `[]`);`SECURITY.md`(404);`FUNDING.yml`(404)
- 构建流水线: `.github/workflows/release-on-tag.yml`(tag 144 tree)、`.cirrus.yml`(tag 144 tree)、`.github/workflows/new_version_check.yml`
- Release assets + digest: `gh api repos/adryfish/fingerprint-chromium/releases/latest`
- README(延迟源码、变现链接、拒收款项声明): `https://raw.githubusercontent.com/adryfish/fingerprint-chromium/main/README.md`
- Chromium 版本: `https://versionhistory.googleapis.com/v1/chrome/platforms/mac/channels/stable/versions/all/releases`
- 同类对比: `gh search repos fingerprint-chromium`(CloakBrowser 28614、undetectable-fingerprint-browser 821、ShardBrowser 534、Chromium_FingerPrint_Tutorial 525)
- Camoufox 可复现构建声明: camoufox.com / 项目文档(未在本评估中独立复核,**待确认**)

**待确认项**:
- 148 源码 tag 何时发布(已 ~4 周,临界);
- macOS DMG 是 ad-hoc 签名还是完全未签名(Gatekeeper 行为);
- adryfish 是否在任何地方发布过 GPG key / 签名身份;
- VirusTotal 对 release 二进制的检出比率;
- 指纹 patch 是否引入 stock Chromium 之外的运行时网络调用;
- 维护者真实身份与司法管辖域(无法评估国家级行为者/供应链风险);
- Camoufox 的 CI/可复现构建声明(相对排序结论已被 fingerprint-chromium 侧的已确认事实充分支撑)。