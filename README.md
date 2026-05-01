# opensource-release-watcher

`opensource-release-watcher` 是一个用于监控开源组件版本发布的 Web 服务。

它定期检查 GitHub 开源仓库的 Release、Tag，以及后续可扩展的安全更新信息；当发现组件存在新版本或重要修复时，系统会通知对应订阅人。

## 背景

团队在项目开发中通常会依赖大量开源组件，例如：

- protobuf
- OpenCV
- libevent
- Eigen
- zlib
- OpenSSL

这些组件会不定期发布新版本，用于修复 Bug、增加功能、修复安全漏洞，或者调整接口行为。

如果完全依赖人工关注，常见问题包括：

- 组件发布新版本后无人感知
- 安全修复没有及时同步
- 多个项目使用不同版本，缺少统一管理
- 升级动作缺少记录
- 组件订阅关系不明确
- Release Note 无人跟踪

因此，本项目希望提供一个简单、可落地的机制，对开源组件版本变化进行持续监控。

## 功能目标

### 核心能力

- 维护开源组件清单
- 定期查询组件的最新 Release
- 当项目没有 Release 时，自动回退查询 Tag
- 记录上一次检查到的版本
- 避免重复发送相同版本通知
- 只支持邮件订阅与邮件通知
- 支持订阅人按模块订阅，也支持订阅全部组件
- 支持为不同组件配置不同订阅人
- 支持生成版本更新摘要

### 扩展方向

- 支持 GitHub Security Advisory
- 支持 OSV 漏洞数据库查询
- 支持 Release Note 关键字分析
- 支持通知优先级
- 支持月度开源组件状态报告

## 核心流程

```text
定时调度触发检查任务
      ↓
启动检查流程
      ↓
从数据库读取组件清单与订阅信息
      ↓
查询 GitHub Release
      ↓
如果没有 Release，则查询 Tag
      ↓
和本地记录的版本进行比较
      ↓
发现新版本
      ↓
生成通知内容
      ↓
发送邮件给订阅人
      ↓
更新本地状态
```

## 使用场景

### 1. 组件版本发布提醒

当某个组件发布新版本时，系统自动通知订阅人。

例如：

```text
protobuf 3.20.1 -> 3.21.0
opencv 4.8.0 -> 4.9.0
```

### 2. 安全修复提醒

当 Release Note 中包含安全相关关键字时，可以提高提醒优先级。

例如：

```text
security
CVE
vulnerability
fix
patch
```

### 3. 开源组件治理

团队可以通过该系统维护内部使用的组件清单，包括：

- 当前使用版本
- 最新上游版本
- 检查时间
- 通知记录
- 升级状态

## 数据存储

组件清单、订阅人、检查状态和通知记录统一存储在 SQLite 中，由后台管理界面维护。

典型数据包括：

- 组件名称
- GitHub 仓库地址
- 当前使用版本
- 邮件订阅人
- 检查策略
- 最近一次检查结果
- 最近一次通知记录

服务端只需要少量基础配置，例如 Microsoft Graph 发信参数和监听端口；组件监控数据本身不通过外部配置文件维护。

## 通知示例

邮件标题：

```text
[开源组件更新] protobuf 3.20.1 -> 3.21.0
```

邮件正文：

```text
组件名称：protobuf
仓库地址：protocolbuffers/protobuf
当前使用版本：3.20.1
最新发布版本：3.21.0
发布时间：2026-xx-xx

Release Note 摘要：
- 修复若干 C++ runtime 问题
- 改进 generated code 行为
- 调整部分接口兼容性

建议动作：
- 请订阅人评估是否需要升级
- 检查当前项目是否受到影响
- 如涉及安全修复，建议优先处理
```

## 推荐架构

推荐采用前后端分离的 Web 服务方式：

```text
Frontend (React + TypeScript + Vite + Ant Design)
        +
Go HTTP API Server + Scheduler + SQLite + Microsoft Graph
```

## 模块设计

```text
opensource-release-watcher/
├── frontend/
│   ├── src/
│   ├── public/
│   ├── package.json
│   └── vite.config.ts
├── backend/
│   ├── cmd/
│   │   └── server/
│   │       └── main.go
│   ├── internal/
│   │   ├── github/
│   │   ├── checker/
│   │   ├── scheduler/
│   │   ├── storage/
│   │   ├── notifier/
│   │   ├── service/
│   │   ├── api/
│   │   └── version/
│   └── go.mod
├── templates/
│   └── release_email.md
├── data/
│   └── watcher.db
└── README.md
```

## 模块说明

### frontend

前端管理界面，推荐使用 React + TypeScript + Vite + Ant Design。

主要用于：

- 组件清单管理
- 订阅人管理
- 检查结果展示
- 通知记录查看
- 手动触发检查任务

### backend

后端服务，负责提供 HTTP API、执行定时检查任务、保存状态以及发送通知。

### github

负责调用 GitHub API，查询 Release 和 Tag。

### checker

负责组件检查逻辑，包括：

- 查询最新版本
- 判断是否有更新
- 解析 Release Note
- 判断通知优先级

### storage

负责保存检查状态，避免重复通知。

使用 SQLite 保存组件检查状态、版本记录和通知记录。

### notifier

负责通知发送，当前由邮件 notifier 实现。

### version

负责版本号比较。

例如：

```text
v1.2.3 > v1.2.2
v2.0.0 > v1.9.9
```

## 运行与部署

项目脚本集中在 `scripts/` 目录。

### 环境配置

复制示例配置后按实际环境修改：

```bash
cp .env.example .env
```

常用配置项：

| 配置项 | 说明 |
| --- | --- |
| `SERVER_ADDR` | 后端监听地址，例如 `127.0.0.1:8000` |
| `DB_PATH` | SQLite 数据库文件路径 |
| `GITHUB_TOKEN` | GitHub API Token，可空 |
| `CHECK_INTERVAL` | 定时检查间隔，例如 `6h` |
| `ADMIN_USERNAME` | 登录用户名，默认 `admin` |
| `ADMIN_PASSWORD` | 登录密码，默认 `admin` |
| `SESSION_SECRET` | 登录 cookie 签名密钥，生产环境应设置为随机长字符串 |
| `GRAPH_*` | 个人 Outlook / Microsoft Graph 发信配置 |
| `DOMAIN` | nginx 对外域名 |
| `EXTERNAL_PORT` | nginx HTTPS 对外端口 |
| `CERT_PATH` | TLS 证书路径 |
| `KEY_PATH` | TLS 私钥路径 |
| `CLIENT_MAX_BODY_SIZE` | nginx 请求体大小限制 |

`.env` 包含真实域名、证书路径和密钥，不应提交；仓库只提交脱敏后的 `.env.example`。

个人 Outlook / Hotmail 邮箱配置：

```env
GRAPH_CLIENT_ID=你的应用客户端 ID
GRAPH_CLIENT_SECRET=可选，按应用注册类型填写
GRAPH_ACCESS_TOKEN=脚本生成的 access_token
GRAPH_REFRESH_TOKEN=脚本生成的 refresh_token
```

在 Microsoft 应用注册中启用个人 Microsoft 账户登录，添加 delegated 权限 `Mail.Send`、`User.Read`、`offline_access`，并添加重定向 URI `https://login.microsoftonline.com/common/oauth2/nativeclient`。然后运行：

```bash
python3 -m pip install selenium requests
python3 tools/outlook_tokens.py
```

先编辑 `tools/outlook_tokens.py` 顶部的 `CLIENT_ID`，需要密钥时再填写 `CLIENT_SECRET`。脚本会打开浏览器让你登录 Outlook，并输出可写入 `.env` 的 token。`GRAPH_TENANT_ID` 和 `GRAPH_REDIRECT_URL` 不需要配置，后端默认使用 `common`，脚本固定使用 `https://login.microsoftonline.com/common/oauth2/nativeclient`。重启服务后，测试邮件和更新通知会使用这些 token 发信。

默认登录账号是 `admin/admin`。生产部署前建议至少修改：

```env
ADMIN_USERNAME=admin
ADMIN_PASSWORD=替换成强密码
SESSION_SECRET=替换成随机长字符串
```

### 本地开发

```bash
scripts/deploy.sh dev
```

默认会启动：

- 后端：`127.0.0.1:8000`
- 前端：`http://127.0.0.1:5173`

可通过环境变量覆盖：

```bash
SERVER_ADDR=127.0.0.1:18080 DEV_PORT=5174 scripts/deploy.sh dev
```

### 编译构建

```bash
scripts/build.sh
```

或使用统一入口：

```bash
scripts/deploy.sh build
```

构建产物：

- 后端二进制：`bin/opensource-release-watcher-server`
- 前端静态资源：`frontend/dist`

### 生产部署

部署脚本会构建后端和前端、写入 systemd service、同步静态资源到 nginx 目录，并生成 nginx HTTPS 配置。

```bash
sudo scripts/deploy.sh start
```

常用命令：

```bash
sudo scripts/deploy.sh restart
sudo scripts/deploy.sh stop
sudo scripts/deploy.sh status
sudo scripts/deploy.sh clean-static
sudo scripts/deploy.sh uninstall
```

查看后端服务日志：

```bash
sudo journalctl -u opensource-release-watcher.service -f
```

查看最近 100 行日志：

```bash
sudo journalctl -u opensource-release-watcher.service -n 100 --no-pager
```

查看 nginx 状态和错误日志：

```bash
sudo systemctl status nginx --no-pager
sudo tail -n 100 /var/log/nginx/error.log
```

可选覆盖项：

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `ENV_FILE` | `.env` | 要加载的环境变量文件 |
| `STATIC_DEST` | `/var/www/opensource-release-watcher` | nginx 静态资源目录 |
| `SERVICE_NAME` | `opensource-release-watcher` | systemd/nginx 配置名 |
| `SERVICE_USER` | 当前 sudo 发起用户 | systemd 运行用户 |
| `SERVICE_GROUP` | 同 `SERVICE_USER` | systemd 运行用户组 |
| `NGINX_SERVICE` | `nginx` | nginx systemd 服务名 |

## 项目定位

本项目不是包管理器，也不是自动升级工具。

它的核心定位是：

```text
开源组件版本变化感知服务
```

它当前解决的问题是：

```text
我们依赖的开源组件，什么时候发布了新版本？
这个新版本是否值得关注？
应该给谁发邮件？
是否已经通知过？
后续是否需要升级评估？
```

## License

本项目采用 [MIT 许可](LICENSE)。
