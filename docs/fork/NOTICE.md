# NOTICE — ProfilePool fork

> 基线上游 tag：`V1.2.0`（2026-07-19 fork 时点）

## 上游许可状态

上游仓库 `black-ant/Ant-Browser`（https://github.com/black-ant/Ant-Browser）在 fork 基线 tag `V1.2.0` 下 **没有 LICENSE 文件**。本 fork 据此假定上游代码**未以任何开源许可证授权**，默认保留全部权利归原作者所有。

因此本 fork **不附带、也不追加任何 LICENSE 文件**：不创建会授予我们不具备之权利的许可证文件。

## 使用范围

- 本 fork（“ProfilePool”）仅供 **私人在本机自用**（multi-account 浏览器环境 / 代理绑定 / 账号池编排）。
- **不要公开再分发本 fork 的二进制构建**（Windows 安装包 / 便携包、macOS `.app`/`.zip`、Linux deb/tar 等）；如确有公开发布需求，**先联系上游作者 black-ant 取得明确许可**。
- 不得移除或篡改上游原有版权 / 作者署名信息（`wails.json` 的 author/company、`README.md` 上游内容等）。

## 第三方运行时

本 fork 在运行时会下载/调用以下第三方二进制，它们**各自保留其原始许可证**，与本 fork 的许可状态无关：

- **xray**（https://github.com/XTLS/Xray-core）— Apache License 2.0
- **sing-box**（https://github.com/SagerNet/sing-box）— GPL-3.0
- **mihomo**（https://github.com/MetaCubeX/mihomo）— GPL-3.0
- **fingerprint-chromium**（https://github.com/adryfish/fingerprint-chromium，推荐内核）— 见上游仓库 LICENSE
- **Playwright** / Node 自动化运行时 — 见各自 Apache-2.0 / MIT 许可

> 运行时二进制由用户自行管理、放在可写 state root 下的 `chrome/`/`bin/` 目录；本 fork 不再分发这些二进制，仅作为本地代理桥 / 内核调用。

## 联系

如上游作者对本 fork 的存在或使用有异议，请联系 fork 维护者下架/调整。