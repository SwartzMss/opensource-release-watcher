# 前端实现设计

## 1. 技术栈

前端推荐采用：

- React。
- TypeScript。
- Vite。
- Ant Design。
- React Router。
- Axios 或基于 `fetch` 的轻量 API client。

前端目录建议：

```text
frontend/
├── src/
│   ├── api/
│   ├── components/
│   ├── pages/
│   ├── routes/
│   ├── types/
│   ├── utils/
│   ├── App.tsx
│   └── main.tsx
├── public/
├── package.json
└── vite.config.ts
```

## 2. 页面规划

### 2.1 仪表盘

用于展示系统整体状态。

核心信息：

- 组件总数。
- 启用监控组件数。
- 检查异常数。
- 通知异常数。
- 最近检查时间。
- 下次检查时间。

主要操作：

- 查看系统健康状态。
- 查看最近通知记录。
- 查看运行指标。

### 2.2 组件管理

用于维护开源组件清单。

列表字段：

| 字段 | 说明 |
| --- | --- |
| name | 组件名称 |
| repo_url | GitHub 仓库地址 |
| current_version | 当前内部使用版本 |
| latest_version | 最近检查到的上游版本 |
| enabled | 是否启用检查 |
| last_check_status | 最近检查状态 |
| last_checked_at | 最近检查时间 |

支持操作：

- 新增组件。
- 编辑组件。
- 启用或禁用检查。
- 删除组件。
- 手动检查单个组件。
- 查看检查历史。
- 查看通知历史。

表单字段：

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| name | string | 是 | 组件展示名称 |
| repo_url | string | 是 | GitHub 仓库完整地址 |
| current_version | string | 是 | 当前内部使用版本 |
| check_strategy | string | 是 | `release_first` 或 `tag_only` |
| enabled | boolean | 是 | 是否启用定时检查 |
| notes | string | 否 | 备注 |

### 2.3 订阅人管理

用于维护订阅人以及其订阅的组件模块。

列表字段：

| 字段 | 说明 |
| --- | --- |
| name | 订阅人名称 |
| email | 邮箱地址 |
| subscribe_scope | 订阅范围，全部组件或指定组件 |
| enabled | 是否启用 |
| created_at | 创建时间 |

支持操作：

- 编辑订阅人。
- 选择或调整订阅组件。
- 一键全选组件。
- 启用或禁用订阅。
- 删除订阅人。

### 2.4 检查记录

用于查看每次版本检查结果。

列表字段：

| 字段 | 说明 |
| --- | --- |
| component_name | 组件名称 |
| source | 数据来源，Release 或 Tag |
| previous_version | 检查前记录版本 |
| latest_version | 本次检查到的最新版本 |
| has_update | 是否存在更新 |
| status | 检查状态 |
| error_message | 失败原因 |
| checked_at | 检查时间 |

支持操作：

- 查看 Release Note 摘要。
- 打开 GitHub Release 或 Tag 链接。
- 按组件、状态、是否有更新筛选。

### 2.5 通知记录

用于查看邮件发送记录。

列表字段：

| 字段 | 说明 |
| --- | --- |
| component_name | 组件名称 |
| version | 通知版本 |
| recipient_email | 收件人 |
| status | 发送状态 |
| error_message | 失败原因 |
| sent_at | 发送时间 |

支持操作：

- 按组件筛选。
- 按通知状态筛选。
- 查看邮件正文快照。

## 3. 前端类型定义

```ts
export interface Component {
  id: number;
  name: string;
  repoOwner: string;
  repoName: string;
  repoUrl: string;
  currentVersion: string;
  latestVersion?: string;
  checkStrategy: 'release_first' | 'tag_only';
  enabled: boolean;
  lastCheckStatus?: 'success' | 'failed' | 'skipped';
  lastCheckedAt?: string;
  notes?: string;
  createdAt: string;
  updatedAt: string;
}

export interface Subscriber {
  id: number;
  componentId: number;
  name: string;
  email: string;
  enabled: boolean;
  createdAt: string;
  updatedAt: string;
}

export interface CheckRecord {
  id: number;
  componentId: number;
  source: 'release' | 'tag';
  previousVersion?: string;
  latestVersion?: string;
  releaseTitle?: string;
  releaseUrl?: string;
  releasePublishedAt?: string;
  releaseNoteSummary?: string;
  hasUpdate: boolean;
  status: 'success' | 'failed';
  errorMessage?: string;
  checkedAt: string;
}

export interface NotificationRecord {
  id: number;
  componentId: number;
  checkRecordId: number;
  version: string;
  recipientEmail: string;
  subject: string;
  body: string;
  status: 'sent' | 'failed';
  errorMessage?: string;
  sentAt?: string;
  createdAt: string;
}
```

## 4. API 调用约定

前端统一通过 `/api` 前缀访问后端。

通用响应格式：

```json
{
  "code": 0,
  "message": "ok",
  "data": {}
}
```

分页响应格式：

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

错误响应格式：

```json
{
  "code": 40001,
  "message": "component not found",
  "data": null
}
```

## 5. 交互规则

- 手动检查组件时，按钮进入 loading 状态，接口返回后刷新组件状态和检查记录。
- 删除组件前需要二次确认。
- 禁用组件后，该组件不再进入定时检查任务。
- 新增组件时需要校验 GitHub 仓库地址和当前版本。
- 列表默认按更新时间或检查时间倒序。
