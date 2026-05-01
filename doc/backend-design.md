# 后端实现设计

## 1. 技术栈

后端推荐采用：

- Go。
- 标准库 `net/http` 或轻量路由框架。
- SQLite。
- Outlook / Microsoft Graph 邮件发送。
- GitHub REST API。
- 后台 scheduler 定时任务。

推荐目录：

```text
backend/
├── cmd/
│   └── server/
│       └── main.go
├── internal/
│   ├── api/
│   ├── checker/
│   ├── github/
│   ├── notifier/
│   ├── scheduler/
│   ├── service/
│   ├── storage/
│   └── version/
└── go.mod
```

## 2. 模块职责

| 模块 | 职责 |
| --- | --- |
| api | HTTP 路由、请求参数解析、响应封装 |
| service | 业务编排，连接 API、storage、checker、notifier |
| storage | SQLite 数据访问，负责组件、订阅人、检查记录、通知记录持久化 |
| github | 调用 GitHub API 查询 Release 和 Tag |
| checker | 执行版本检查、版本比较、Release Note 摘要、通知判定 |
| notifier | 邮件通知发送 |
| scheduler | 定时触发全量检查任务 |
| version | 版本号标准化和比较 |

## 3. 配置项

服务端基础配置通过环境变量或配置文件提供。

| 配置项 | 必填 | 说明 |
| --- | --- | --- |
| SERVER_ADDR | 否 | HTTP 监听地址，默认 `:8080` |
| DB_PATH | 否 | SQLite 文件路径，默认 `data/watcher.db` |
| GITHUB_TOKEN | 否 | GitHub API Token，用于提高限流额度 |
| CHECK_INTERVAL | 否 | 定时检查间隔，例如 `6h` |
| GRAPH_CLIENT_ID | 否 | Azure App Registration client ID |
| GRAPH_CLIENT_SECRET | 否 | Azure App Registration client secret；公共客户端可不填 |
| GRAPH_ACCESS_TOKEN | 否 | `tools/outlook_tokens.py` 生成的 Microsoft Graph access token |
| GRAPH_REFRESH_TOKEN | 否 | `tools/outlook_tokens.py` 生成的 Microsoft Graph refresh token |

## 4. API 设计

### 4.1 组件 API

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/components` | 查询组件列表 |
| POST | `/api/components` | 新增组件 |
| GET | `/api/components/{id}` | 查询组件详情 |
| PUT | `/api/components/{id}` | 更新组件 |
| DELETE | `/api/components/{id}` | 删除组件 |
| POST | `/api/components/{id}/check` | 手动检查单个组件 |

新增组件请求：

```json
{
  "name": "protobuf",
  "repo_url": "https://github.com/protocolbuffers/protobuf",
  "current_version": "3.20.1",
  "check_strategy": "release_first",
  "enabled": true,
  "notes": "C++ protobuf runtime"
}
```

### 4.2 订阅人 API

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/global-subscribers` | 查询订阅人 |
| POST | `/api/global-subscribers` | 新增订阅人 |
| PUT | `/api/global-subscribers/{id}` | 更新订阅人 |
| PUT | `/api/global-subscribers/{id}/components` | 更新订阅模块 |
| DELETE | `/api/global-subscribers/{id}` | 删除订阅人 |
| GET | `/api/components/{id}/subscribers` | 查询组件订阅人 |
| POST | `/api/components/{id}/subscribers` | 新增订阅人 |
| PUT | `/api/subscribers/{id}` | 更新订阅人 |
| DELETE | `/api/subscribers/{id}` | 删除订阅人 |

### 4.3 检查 API

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/system-runs` | 查询全量检查运行记录 |
| GET | `/api/check-records` | 查询检查记录 |
| GET | `/api/check-records/{id}` | 查询检查详情 |

### 4.4 通知 API

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/notification-records` | 查询通知记录 |
| POST | `/api/notification-records/test` | 发送测试邮件 |
| GET | `/api/notification-records/{id}` | 查询通知详情 |

### 4.5 仪表盘 API

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/dashboard/summary` | 查询仪表盘汇总数据 |

响应示例：

```json
{
  "component_total": 32,
  "enabled_component_total": 28,
  "components_with_update": 5,
  "last_check_failed_total": 2,
  "notification_failed_total": 1,
  "last_full_check_at": "2026-04-28T10:00:00Z"
}
```

## 5. 检查逻辑

单组件检查流程：

```text
读取组件信息
      ↓
判断组件是否启用
      ↓
按检查策略查询 GitHub Release 或 Tag
      ↓
标准化版本号
      ↓
比较 latest_version 与 last_seen_version
      ↓
写入 check_records
      ↓
如果有更新且未通知过该版本，发送邮件
      ↓
写入 notification_records
      ↓
更新 components 最近状态字段
```

版本判断规则：

- Release 优先策略下，优先使用 GitHub latest release。
- 如果 latest release 不存在或仓库未使用 Release，则读取最新 Tag。
- 版本号比较前移除常见前缀 `v`。
- 无法解析为语义化版本时，可先按发布时间判断新旧。
- 同一组件同一版本已经存在成功通知记录时，不再重复发送。

## 6. 邮件通知

邮件标题：

```text
[开源组件更新] protobuf 3.20.1 -> 3.21.0
```

邮件正文需要包含：

- 组件名称。
- GitHub 仓库。
- 当前内部使用版本。
- 最新上游版本。
- 发布时间。
- Release Note 摘要。
- GitHub 链接。
- 建议动作。

收件人规则：

- 订阅人可选择接收全部组件通知。
- 组件订阅人只接收对应组件通知。
- 启用状态的订阅人邮箱需要接收。
- 相同邮箱去重。

## 7. 错误处理

- GitHub API 请求失败时，检查记录状态为 `failed`，并记录错误信息。
- 单个组件检查失败不影响其他组件检查。
- 邮件发送失败时，写入失败通知记录，不回滚检查记录。
- 数据库写入失败需要返回 API 错误，并记录日志。
- GitHub Rate Limit 需要识别并记录明确错误。
