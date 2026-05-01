# 数据字段与数据库设计

## 1. 设计原则

系统使用 SQLite 保存组件清单、订阅人、检查记录、通知记录和运行状态。组件监控数据通过后台管理界面维护，不依赖外部配置文件。

核心原则：

- 组件是主实体。
- 订阅人是主实体。
- 每个订阅人可以选择全部组件或指定组件集合。
- 每次检查都生成检查记录。
- 每次邮件发送都生成通知记录。
- 通过唯一约束避免同一组件同一版本重复通知。

## 2. 表结构

### 2.1 components

保存开源组件基础信息和最近状态。

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| id | INTEGER | 是 | 主键 |
| name | TEXT | 是 | 组件名称 |
| repo_url | TEXT | 是 | GitHub 仓库完整地址 |
| current_version | TEXT | 是 | 当前内部使用版本 |
| latest_version | TEXT | 否 | 最近检查到的上游版本 |
| last_seen_version | TEXT | 否 | 最近已处理版本，用于去重 |
| check_strategy | TEXT | 是 | 检查策略，`release_first` 或 `tag_only` |
| enabled | INTEGER | 是 | 是否启用，1 是，0 否 |
| last_check_status | TEXT | 否 | 最近检查状态，`success`、`failed`、`skipped` |
| last_check_error | TEXT | 否 | 最近检查失败原因 |
| last_checked_at | DATETIME | 否 | 最近检查时间 |
| notes | TEXT | 否 | 备注 |
| created_at | DATETIME | 是 | 创建时间 |
| updated_at | DATETIME | 是 | 更新时间 |

建议约束：

```sql
UNIQUE(repo_url)
```

### 2.2 subscribers

保存组件邮件订阅人。

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| id | INTEGER | 是 | 主键 |
| component_id | INTEGER | 是 | 组件 ID |
| name | TEXT | 是 | 订阅人名称 |
| email | TEXT | 是 | 订阅人邮箱 |
| enabled | INTEGER | 是 | 是否启用 |
| created_at | DATETIME | 是 | 创建时间 |
| updated_at | DATETIME | 是 | 更新时间 |

建议约束：

```sql
UNIQUE(component_id, email)
FOREIGN KEY(component_id) REFERENCES components(id)
```

### 2.3 global_subscribers

保存订阅人主表。

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| id | INTEGER | 是 | 主键 |
| name | TEXT | 是 | 订阅人名称 |
| email | TEXT | 是 | 订阅人邮箱 |
| enabled | INTEGER | 是 | 是否启用 |
| created_at | DATETIME | 是 | 创建时间 |
| updated_at | DATETIME | 是 | 更新时间 |

建议约束：

```sql
UNIQUE(email)
```

### 2.4 check_records

保存每次版本检查结果。

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| id | INTEGER | 是 | 主键 |
| component_id | INTEGER | 是 | 组件 ID |
| source | TEXT | 否 | 数据来源，`release` 或 `tag` |
| previous_version | TEXT | 否 | 检查前记录的版本 |
| latest_version | TEXT | 否 | 本次检查到的最新版本 |
| release_title | TEXT | 否 | Release 标题 |
| release_url | TEXT | 否 | Release 或 Tag URL |
| release_published_at | DATETIME | 否 | 上游发布时间 |
| release_note | TEXT | 否 | Release Note 原文 |
| release_note_summary | TEXT | 否 | Release Note 摘要 |
| has_update | INTEGER | 是 | 是否存在更新 |
| status | TEXT | 是 | 检查状态，`success` 或 `failed` |
| error_message | TEXT | 否 | 失败原因 |
| checked_at | DATETIME | 是 | 检查时间 |

建议索引：

```sql
CREATE INDEX idx_check_records_component_id ON check_records(component_id);
CREATE INDEX idx_check_records_checked_at ON check_records(checked_at);
```

### 2.5 notification_records

保存邮件通知记录。

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| id | INTEGER | 是 | 主键 |
| component_id | INTEGER | 是 | 组件 ID |
| check_record_id | INTEGER | 是 | 检查记录 ID |
| version | TEXT | 是 | 通知对应的最新版本 |
| recipient_email | TEXT | 是 | 收件人邮箱 |
| subject | TEXT | 是 | 邮件标题 |
| body | TEXT | 是 | 邮件正文快照 |
| status | TEXT | 是 | 发送状态，`sent` 或 `failed` |
| error_message | TEXT | 否 | 失败原因 |
| sent_at | DATETIME | 否 | 发送成功时间 |
| created_at | DATETIME | 是 | 创建时间 |

建议约束：

```sql
UNIQUE(component_id, version, recipient_email)
FOREIGN KEY(component_id) REFERENCES components(id)
FOREIGN KEY(check_record_id) REFERENCES check_records(id)
```

### 2.6 system_runs

保存全量检查任务运行记录。

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| id | INTEGER | 是 | 主键 |
| trigger_type | TEXT | 是 | 触发方式，`scheduler` 或 `manual` |
| status | TEXT | 是 | 运行状态，`running`、`success`、`failed` |
| total_count | INTEGER | 是 | 本次计划检查组件数 |
| success_count | INTEGER | 是 | 成功数量 |
| failed_count | INTEGER | 是 | 失败数量 |
| started_at | DATETIME | 是 | 开始时间 |
| finished_at | DATETIME | 否 | 结束时间 |
| error_message | TEXT | 否 | 全局失败原因 |

## 3. 初始化 SQL 草案

```sql
CREATE TABLE components (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  repo_url TEXT NOT NULL,
  current_version TEXT NOT NULL,
  latest_version TEXT,
  last_seen_version TEXT,
  check_strategy TEXT NOT NULL DEFAULT 'release_first',
  enabled INTEGER NOT NULL DEFAULT 1,
  last_check_status TEXT,
  last_check_error TEXT,
  last_checked_at DATETIME,
  notes TEXT,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  UNIQUE(repo_url)
);

CREATE TABLE subscribers (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  component_id INTEGER NOT NULL,
  name TEXT NOT NULL,
  email TEXT NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  UNIQUE(component_id, email),
  FOREIGN KEY(component_id) REFERENCES components(id)
);

CREATE TABLE global_subscribers (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  email TEXT NOT NULL UNIQUE,
  enabled INTEGER NOT NULL DEFAULT 1,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL
);

CREATE TABLE check_records (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  component_id INTEGER NOT NULL,
  source TEXT,
  previous_version TEXT,
  latest_version TEXT,
  release_title TEXT,
  release_url TEXT,
  release_published_at DATETIME,
  release_note TEXT,
  release_note_summary TEXT,
  has_update INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL,
  error_message TEXT,
  checked_at DATETIME NOT NULL,
  FOREIGN KEY(component_id) REFERENCES components(id)
);

CREATE TABLE notification_records (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  component_id INTEGER NOT NULL,
  check_record_id INTEGER NOT NULL,
  version TEXT NOT NULL,
  recipient_email TEXT NOT NULL,
  subject TEXT NOT NULL,
  body TEXT NOT NULL,
  status TEXT NOT NULL,
  error_message TEXT,
  sent_at DATETIME,
  created_at DATETIME NOT NULL,
  UNIQUE(component_id, version, recipient_email),
  FOREIGN KEY(component_id) REFERENCES components(id),
  FOREIGN KEY(check_record_id) REFERENCES check_records(id)
);

CREATE TABLE system_runs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  trigger_type TEXT NOT NULL,
  status TEXT NOT NULL,
  total_count INTEGER NOT NULL DEFAULT 0,
  success_count INTEGER NOT NULL DEFAULT 0,
  failed_count INTEGER NOT NULL DEFAULT 0,
  started_at DATETIME NOT NULL,
  finished_at DATETIME,
  error_message TEXT
);

CREATE INDEX idx_check_records_component_id ON check_records(component_id);
CREATE INDEX idx_check_records_checked_at ON check_records(checked_at);
CREATE INDEX idx_notification_records_component_id ON notification_records(component_id);
CREATE INDEX idx_notification_records_created_at ON notification_records(created_at);
```
