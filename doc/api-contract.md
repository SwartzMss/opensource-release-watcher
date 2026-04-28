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

## 2. 组件接口

### 2.1 查询组件列表

```http
GET /api/components?page=1&page_size=20&keyword=protobuf&enabled=true
```

响应字段：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| id | number | 组件 ID |
| name | string | 组件名称 |
| repo_owner | string | GitHub owner |
| repo_name | string | GitHub repo |
| repo_url | string | GitHub 仓库地址 |
| current_version | string | 当前内部使用版本 |
| latest_version | string | 最近检查到的上游版本 |
| owner_name | string | 负责人 |
| owner_email | string | 负责人邮箱 |
| check_strategy | string | 检查策略 |
| enabled | boolean | 是否启用 |
| last_check_status | string | 最近检查状态 |
| last_checked_at | string | 最近检查时间 |
| updated_at | string | 更新时间 |

### 2.2 新增组件

```http
POST /api/components
```

请求体：

```json
{
  "name": "protobuf",
  "repo_owner": "protocolbuffers",
  "repo_name": "protobuf",
  "current_version": "3.20.1",
  "owner_name": "platform-team",
  "owner_email": "platform@example.com",
  "check_strategy": "release_first",
  "enabled": true,
  "notes": "C++ runtime dependency"
}
```

### 2.3 更新组件

```http
PUT /api/components/{id}
```

请求体字段与新增组件一致。

### 2.4 手动检查单个组件

```http
POST /api/components/{id}/check
```

响应示例：

```json
{
  "id": 101,
  "component_id": 1,
  "source": "release",
  "previous_version": "3.20.1",
  "latest_version": "3.21.0",
  "has_update": true,
  "status": "success",
  "checked_at": "2026-04-28T10:00:00Z"
}
```

## 3. 订阅人接口

### 3.1 查询组件订阅人

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

### 3.2 新增订阅人

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

### 3.3 更新订阅人

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

### 3.4 删除订阅人

```http
DELETE /api/subscribers/{id}
```

## 4. 检查记录接口

### 4.1 手动触发全量检查

```http
POST /api/checks/run
```

响应示例：

```json
{
  "run_id": 10,
  "status": "running",
  "started_at": "2026-04-28T10:00:00Z"
}
```

### 4.2 查询全量检查运行记录

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

### 4.3 查询检查记录

```http
GET /api/check-records?page=1&page_size=20&component_id=1&status=success&has_update=true
```

响应字段：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| id | number | 检查记录 ID |
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

## 5. 通知记录接口

### 5.1 查询通知记录

```http
GET /api/notification-records?page=1&page_size=20&component_id=1&status=sent
```

响应字段：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| id | number | 通知记录 ID |
| component_id | number | 组件 ID |
| component_name | string | 组件名称 |
| check_record_id | number | 检查记录 ID |
| version | string | 通知版本 |
| recipient_email | string | 收件人邮箱 |
| subject | string | 邮件标题 |
| status | string | `sent` 或 `failed` |
| error_message | string | 失败原因 |
| sent_at | string | 发送成功时间 |
| created_at | string | 创建时间 |

### 5.2 查询通知详情

```http
GET /api/notification-records/{id}
```

详情接口需要额外返回 `body` 字段，用于查看邮件正文快照。

## 6. 枚举值

### 6.1 check_strategy

| 值 | 说明 |
| --- | --- |
| release_first | 优先查询 Release，无 Release 时回退 Tag |
| tag_only | 只查询 Tag |

### 6.2 check status

| 值 | 说明 |
| --- | --- |
| success | 检查成功 |
| failed | 检查失败 |
| skipped | 跳过检查 |

### 6.3 notification status

| 值 | 说明 |
| --- | --- |
| sent | 发送成功 |
| failed | 发送失败 |
