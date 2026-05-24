# API 与字段契约

## 1. 通用约定

所有 API 使用 JSON 请求和 JSON 响应。

成功响应：

```json
{
  "code": 0,
  "message": "ok",
  "data": {}
}
```

失败响应：

```json
{
  "code": 40001,
  "message": "component not found",
  "data": null
}
```

分页参数：

| 参数 | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| page | integer | 1 | 页码 |
| page_size | integer | 20 | 每页数量 |

分页响应：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "items": [],
    "total": 0,
    "page": 1,
    "page_size": 20
  }
}
```

## 2. 认证接口

### 2.1 查询当前会话

```http
GET /api/auth/me
```

成功时返回当前登录用户信息。

### 2.2 登录

```http
POST /api/auth/login
```

请求体：

```json
{
  "username": "admin",
  "password": "admin"
}
```

### 2.3 退出登录

```http
POST /api/auth/logout
```

### 2.4 会话心跳

```http
POST /api/auth/heartbeat
```

作用：

- 刷新会话空闲时间
- 如果会话已失效，返回 `401`

## 3. 组件接口

### 3.1 查询组件列表

```http
GET /api/components?page=1&page_size=20&keyword=protobuf&enabled=true
```

响应字段：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| id | number | 组件 ID |
| name | string | 组件名称 |
| repo_url | string | GitHub 仓库地址，创建后不可修改 |
| current_version | string | 当前内部使用版本，必须能在 GitHub Release 或 Tag 历史中找到 |
| latest_version | string | 最近检查到的上游版本 |
| check_strategy | string | 检查策略 |
| enabled | boolean | 是否启用 |
| last_check_status | string | 最近检查状态 |
| last_checked_at | string | 最近检查时间 |
| security_status | string | 最近漏洞检查状态 |
| security_suggested_version | string | 最近建议升级版本 |
| security_checked_at | string | 最近漏洞检查时间 |
| updated_at | string | 更新时间 |

### 3.2 新增组件

```http
POST /api/components
```

请求体：

```json
{
  "name": "protobuf",
  "repo_url": "https://github.com/protocolbuffers/protobuf",
  "current_version": "3.20.1",
  "check_strategy": "release_first",
  "enabled": true,
  "notes": "C++ runtime dependency"
}
```

`current_version` 需要满足两个约束：

- 只能向前升级，不允许回退。
- 必须能在 GitHub Release 或 Tag 历史中找到。

`repo_url` 在组件创建后不可修改。

### 3.3 更新组件

```http
PUT /api/components/{id}
```

请求体字段与新增组件一致。

### 3.4 手动检查单个组件

```http
POST /api/components/{id}/check
```

该接口会创建一轮组件检查运行：先执行版本检查并返回版本检查记录，再异步执行漏洞检查和聚合通知。

响应示例：

```json
{
  "id": 101,
  "run_id": 301,
  "component_id": 1,
  "source": "release",
  "previous_version": "3.20.1",
  "latest_version": "3.21.0",
  "has_update": true,
  "status": "success",
  "checked_at": "2026-04-28T10:00:00Z"
}
```

## 4. 订阅人接口

订阅分两种：

- 订阅人可以选择订阅全部组件。
- 订阅人也可以只选择部分组件。

### 4.1 查询组件订阅人

```http
GET /api/components/{id}/subscribers
```

响应字段：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| id | number | 订阅人 ID |
| component_id | number | 组件 ID |
| name | string | 订阅人名称 |
| email | string | 订阅人邮箱 |
| enabled | boolean | 是否启用 |
| created_at | string | 创建时间 |

### 4.2 新增订阅人

```http
POST /api/components/{id}/subscribers
```

请求体：

```json
{
  "name": "张三",
  "email": "zhangsan@example.com",
  "enabled": true
}
```

### 4.3 更新订阅人

```http
PUT /api/subscribers/{id}
```

请求体：

```json
{
  "name": "张三",
  "email": "zhangsan@example.com",
  "enabled": true
}
```

### 4.4 删除订阅人

```http
DELETE /api/subscribers/{id}
```

### 4.5 查询订阅人

```http
GET /api/global-subscribers
```

响应字段：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| id | number | 订阅人 ID |
| name | string | 订阅人名称 |
| email | string | 订阅人邮箱 |
| enabled | boolean | 是否启用 |
| all_components | boolean | 是否订阅全部组件 |
| component_ids | number[] | 选择订阅的组件 ID 列表 |
| created_at | string | 创建时间 |
| updated_at | string | 更新时间 |

### 4.6 新增订阅人

```http
POST /api/global-subscribers
```

请求体：

```json
{
  "name": "张三",
  "email": "zhangsan@example.com",
  "enabled": true,
  "all_components": false
}
```

### 4.7 更新订阅人

```http
PUT /api/global-subscribers/{id}
```

请求体：

```json
{
  "name": "张三",
  "email": "zhangsan@example.com",
  "enabled": true,
  "all_components": false
}
```

### 4.8 删除订阅人

```http
DELETE /api/global-subscribers/{id}
```

### 4.9 更新订阅模块

```http
PUT /api/global-subscribers/{id}/components
```

请求体：

```json
{
  "all_components": false,
  "component_ids": [1, 3, 8]
}
```

## 5. 检查记录接口

### 5.1 查询全量检查运行记录

```http
GET /api/system-runs?page=1&page_size=20
```

响应字段：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| id | number | 运行记录 ID |
| trigger_type | string | `scheduler` 或 `manual` |
| status | string | `running`、`success` 或 `failed` |
| total_count | number | 计划检查组件数 |
| success_count | number | 成功数量 |
| failed_count | number | 失败数量 |
| started_at | string | 开始时间 |
| finished_at | string | 结束时间 |
| error_message | string | 全局失败原因 |

### 5.2 查询检查记录

```http
GET /api/check-records?page=1&page_size=20&component_id=1&status=success&has_update=true
```

响应字段：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| id | number | 检查记录 ID |
| run_id | number | 组件检查运行 ID |
| component_id | number | 组件 ID |
| component_name | string | 组件名称 |
| source | string | `release` 或 `tag` |
| previous_version | string | 检查前版本 |
| latest_version | string | 最新版本 |
| release_title | string | Release 标题 |
| release_url | string | Release 或 Tag URL |
| release_published_at | string | 发布时间 |
| release_note_summary | string | 摘要 |
| has_update | boolean | 是否存在更新 |
| status | string | 检查状态 |
| error_message | string | 失败原因 |
| checked_at | string | 检查时间 |

## 6. 通知记录接口

### 6.1 查询通知记录

```http
GET /api/notification-records?page=1&page_size=20&component_id=1&status=sent
```

可选筛选参数：

| 参数 | 类型 | 说明 |
| --- | --- | --- |
| component_id | number | 按组件筛选 |
| run_id | number | 按组件检查运行筛选 |
| check_record_id | number | 按版本检查记录筛选 |
| recipient_email | string | 按收件人筛选 |
| status | string | 按通知状态筛选 |

响应字段：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| id | number | 通知记录 ID |
| run_id | number | 组件检查运行 ID |
| component_id | number | 组件 ID |
| component_name | string | 组件名称 |
| check_record_id | number | 版本检查记录 ID，可能为空 |
| notification_type | string | 通知类型，当前为 `component_check_summary` |
| fingerprint | string | 通知去重指纹 |
| version | string | 通知对应的最新版本或当前版本 |
| recipient_email | string | 收件人邮箱 |
| subject | string | 邮件标题 |
| status | string | `sent` 或 `failed` |
| error_message | string | 失败原因 |
| sent_at | string | 发送成功时间 |
| created_at | string | 创建时间 |

### 6.2 查询通知详情

```http
GET /api/notification-records/{id}
```

详情接口需要额外返回 `body` 字段，用于查看邮件正文快照。

### 6.3 发送测试邮件

```http
POST /api/notification-records/test
```

请求体：

```json
{
  "recipient": "name@example.com"
}
```

响应：

```json
{
  "sent": true
}
```

测试邮件只用于验证当前邮件发信配置，不写入通知记录。

### 6.4 查询邮件授权状态

```http
GET /api/mail/status
```

响应：

```json
{
  "configured": true,
  "connected": true
}
```

### 6.5 查询运行状态

```http
GET /api/system/status
```

该接口只返回后端缓存的运行状态，不会在每次请求时实时访问 GitHub。服务启动后会立即异步检测一次，之后每 2 小时刷新一次。

响应：

```json
{
  "proxy_status": "正常",
  "proxy_message": "",
  "github_token_status": "正常",
  "github_token_message": "",
  "checked_at": "2026-05-05T08:00:00Z"
}
```

## 7. 枚举值

### 7.1 check_strategy

| 值 | 说明 |
| --- | --- |
| release_first | 优先查询 Release，无 Release 时回退 Tag |
| tag_only | 只查询 Tag |

### 7.2 check status

| 值 | 说明 |
| --- | --- |
| success | 检查成功 |
| failed | 检查失败 |
| partial_failed | 部分流程失败 |
| skipped | 跳过检查 |

### 7.3 notification status

| 值 | 说明 |
| --- | --- |
| sent | 发送成功 |
| failed | 发送失败 |
