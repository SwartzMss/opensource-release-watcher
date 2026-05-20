# opensource-release-watcher

`opensource-release-watcher` 是一个用于持续关注开源组件版本变化和已知漏洞风险的 Web 服务。

它维护一份团队正在使用的开源组件清单，定期检查 GitHub Release / Tag 和 OSV 漏洞数据；当发现新版本或当前版本存在已知漏洞时，系统会给对应订阅人发送邮件提醒。

## 项目意图

团队依赖的开源组件通常会不定期发布新版本，用于修复 Bug、修复安全漏洞、增加功能或调整接口行为。如果完全依赖人工关注，容易出现：

- 新版本发布后无人感知。
- 安全修复没有及时同步。
- 多个项目使用不同版本，缺少统一管理。
- 组件订阅关系不明确。
- Release Note 和通知记录缺少留痕。

本项目提供一个轻量的开源组件版本和风险感知服务，帮助团队持续跟踪组件发布、漏洞风险和通知状态。

## 核心能力

- 维护开源组件清单和当前内部使用版本。
- 查询 GitHub Release，仓库没有 Release 时回退到 Tag。
- 使用 OSV 查询当前版本对应 commit 的公开漏洞。
- 将版本更新和漏洞风险聚合成一封邮件通知。
- 支持全局订阅人和按组件订阅。
- 避免相同组件、相同收件人重复接收相同检查结果通知。
- 提供仪表盘、组件管理、漏洞检查、检查记录和通知记录页面。

## 快速开始

### 1. 准备配置

复制示例配置：

```bash
cp .env.example .env
```

常用配置：

| 配置项 | 说明 |
| --- | --- |
| `SERVER_ADDR` | 后端监听地址，例如 `127.0.0.1:8000` |
| `DB_PATH` | SQLite 数据库路径 |
| `CHECK_INTERVAL` | 组件定时检查间隔，例如 `6h` |
| `STATIC_DIR` | 前端静态资源目录，standalone 部署时通常为 `./frontend/dist` |
| `GITHUB_TOKEN` | GitHub API Token，建议配置以提高 API 限额 |
| `HTTP_PROXY` / `HTTPS_PROXY` | 可选代理配置 |
| `NO_PROXY` | 不走代理的地址，例如 `localhost,127.0.0.1` |
| `ADMIN_USERNAME` | 登录用户名 |
| `ADMIN_PASSWORD` | 登录密码 |
| `SESSION_SECRET` | 登录 cookie 签名密钥，生产环境必须修改 |
| `GRAPH_*` | Microsoft Graph 邮件发送配置 |
| `DEPLOY_MODE` | 部署模式，默认 `standalone`，只运行 Go 服务 |

`.env` 包含真实密钥和本机路径，不应提交到仓库。

### 2. 本地开发

```bash
scripts/deploy.sh dev
```

默认地址：

- 后端 API：`http://127.0.0.1:8000`
- 前端页面：`http://127.0.0.1:5173`

默认登录账号以 `.env` 为准；如果未修改示例配置，通常是 `admin/admin`。

### 3. 构建

```bash
scripts/build.sh
```

构建产物：

- 后端二进制：`bin/opensource-release-watcher-server`
- 前端静态资源：`frontend/dist`

### 4. 发布打包

生成 Linux 和 Windows release 包：

```bash
scripts/release.sh v0.1.0
```

如果不传版本号，脚本会优先使用当前 Git tag / commit：

```bash
scripts/release.sh
```

默认产物输出到 `release/`：

```text
opensource-release-watcher-0.1.0-linux-amd64.tar.gz
opensource-release-watcher-0.1.0-linux-arm64.tar.gz
opensource-release-watcher-0.1.0-windows-amd64.zip
SHA256SUMS
```

每个包内包含：

```text
bin/opensource-release-watcher-server      # Linux 包
bin/opensource-release-watcher-server.exe  # Windows 包
frontend/dist/
.env.example
README.md
LICENSE
scripts/deploy.sh
```

发布到 GitHub Release 时，建议先打 tag：

```bash
git tag v0.1.0
git push origin v0.1.0
```

仓库提供了 GitHub Actions workflow：`.github/workflows/release.yml`。推送 `v*` tag 后会自动：

1. 构建前端静态资源。
2. 运行 Go 测试。
3. 构建 Linux amd64、Linux arm64、Windows amd64 release 包。
4. 生成 `SHA256SUMS`。
5. 创建或更新对应 GitHub Release，并上传压缩包。

也可以在 GitHub Actions 页面手动运行 `Release` workflow，输入版本号，例如 `v0.1.0`。

### 5. 生产部署

默认部署方式是不使用 nginx，让 Go 后端直接提供 API 和前端静态资源。在 `.env` 中使用：

```env
DEPLOY_MODE=standalone
SERVER_ADDR=0.0.0.0:8000
STATIC_DIR=./frontend/dist
```

然后执行：

```bash
sudo scripts/deploy.sh start
```

访问地址为 `http://服务器IP:8000/`。

Windows release 包可以直接运行：

```powershell
Copy-Item .env.example .env
.\bin\opensource-release-watcher-server.exe
```

然后访问 `http://127.0.0.1:8000/`。如果要让局域网访问，将 `.env` 中的 `SERVER_ADDR` 设置为 `0.0.0.0:8000`。

常用命令：

```bash
sudo scripts/deploy.sh restart
sudo scripts/deploy.sh stop
sudo scripts/deploy.sh status
sudo scripts/deploy.sh uninstall
```

## 邮件通知配置

当前邮件发送使用个人 Outlook / Hotmail 的 Microsoft Graph delegated token。

需要在 `.env` 中配置：

```env
GRAPH_CLIENT_ID=你的应用客户端 ID
GRAPH_CLIENT_SECRET=可选，按应用注册类型填写
GRAPH_ACCESS_TOKEN=脚本生成的 access_token
GRAPH_REFRESH_TOKEN=脚本生成的 refresh_token
```

获取 token 的辅助脚本：

```bash
python3 -m pip install selenium requests
python3 tools/outlook_tokens.py
```

运行前先编辑 `tools/outlook_tokens.py` 顶部的 `CLIENT_ID`，需要密钥时再填写 `CLIENT_SECRET`。Microsoft 应用注册需要 delegated 权限 `Mail.Send`、`User.Read`、`offline_access`，重定向 URI 使用：

```text
https://login.microsoftonline.com/common/oauth2/nativeclient
```

## 使用方式

1. 登录后台。
2. 在“组件管理”中新增 GitHub 组件，填写当前内部使用版本。
3. 在“订阅人管理”中配置收件人和订阅范围。
4. 手动检查组件，或等待定时任务自动检查。
5. 在“漏洞检查”查看当前版本命中的 OSV 漏洞。
6. 在“通知记录”查看邮件发送结果。

## 日志

后端默认写入仓库下的日志目录：

```bash
tail -f log/server.log
tail -n 100 log/server.log
```

如果使用 systemd 部署，也可以查看服务日志：

```bash
sudo journalctl -u opensource-release-watcher.service -n 100 --no-pager
```

## 文档索引

更多设计和实现细节放在 `doc/` 目录：

- [文档索引](doc/README.md)
- [需求文档](doc/requirements.md)
- [后端设计](doc/backend-design.md)
- [前端设计](doc/frontend-design.md)
- [API 与字段契约](doc/api-contract.md)
- [数据模型](doc/data-model.md)
- [安全风险设计](doc/security-risk-design.md)

## 关注范围

本项目关注：

- 当前使用的开源组件是否有新版本。
- 当前使用版本是否命中公开已知漏洞。
- 哪些订阅人需要收到提醒。
- 是否已经通知过。

## License

本项目采用 [MIT 许可](LICENSE)。
