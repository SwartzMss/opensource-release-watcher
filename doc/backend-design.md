# 后端设计

本文记录后端实现结构和关键业务流程。API 字段细节见 [API 与字段契约](api-contract.md)，数据库结构见 [数据模型](data-model.md)。

## 1. 模块职责

| 模块 | 职责 |
| --- | --- |
| `api` | HTTP 路由、认证、请求解析、统一响应 |
| `service` | 业务编排，串联检查、漏洞、通知和运行状态 |
| `storage` | SQLite 数据访问和记录保留策略 |
| `github` | GitHub REST API client，支持 token、代理、重试和日志 |
| `gitrepo` | 解析组件版本对应的 Git tag / commit |
| `checker` | 版本检查，读取 Release / Tag 并比较版本 |
| `security` | 当前版本漏洞检查和修复版本建议 |
| `osv` | OSV API client |
| `notifier` | 邮件发送，支持 Microsoft Graph 和 SMTP / Exchange relay |
| `scheduler` | 周期性触发全量组件检查 |
| `version` | 版本标准化和比较 |

## 2. 组件检查流程

单个组件检查会创建一条 `component_check_runs`，用于关联版本检查、漏洞检查和通知结果。

```text
读取组件
  ↓
创建 component_check_runs
  ↓
执行版本检查
  ↓
写入 check_records
  ↓
更新 components 最近检查状态
  ↓
异步执行漏洞检查
  ↓
写入 component_security_profiles / component_security_records
  ↓
生成检查结果 fingerprint
  ↓
按订阅人判断是否需要发送聚合邮件
  ↓
写入 notification_records
  ↓
结束 component_check_runs
```

触发来源：

- `scheduler`：周期检查。
- `manual_check`：用户手动检查。
- `create_component`：新增组件后立即检查。
- `update_component`：更新组件版本后立即检查。

## 3. 版本检查

检查策略：

- `release_first`：优先读取 latest release，无 release 时回退到 tag。
- `tag_only`：只读取 tag。

版本判断：

- 版本比较使用 `version` 模块。
- 常见 `v` 前缀会被标准化。
- 当前内部版本必须能在上游 Release / Tag 中解析。
- 组件编辑时当前版本只允许向前升级。

## 4. 漏洞检查

漏洞检查由异步 worker 执行，避免组件检查接口长时间阻塞。

主要步骤：

1. 根据 `repo_url + current_version` 解析 commit。
2. 使用 commit 查询 OSV。
3. 对返回漏洞按 ID 去重。
4. 为每个漏洞解析建议升级版本。
5. 组件级建议升级版本取所有漏洞建议中的最高版本。
6. 保存当前组件最新一次漏洞明细。

状态含义：

| 状态 | 含义 |
| --- | --- |
| `affected` | 当前版本命中 OSV 漏洞 |
| `unknown` | 未命中漏洞或无法解析到足够证据 |
| `check_failed` | OSV 或 GitHub 查询失败 |

## 5. 通知策略

通知是组件检查的最终聚合结果。

通知触发条件：

- 发现订阅人尚未收到的新版本。
- 当前检查命中漏洞风险。

通知去重字段：

```text
component_id + recipient_email + notification_type + fingerprint
```

`fingerprint` 由以下内容生成：

- 最新版本。
- 版本检查状态。
- 漏洞检查状态。
- 组件级建议升级版本。
- 命中的漏洞 ID 列表。

版本通知进度：

- 每个订阅人与组件维护独立 `last_notified_version`。
- 邮件发送成功后推进该订阅人的版本进度。
- 漏洞风险使用 fingerprint 去重，不依赖版本进度。

## 6. 运行状态检测

系统概览中的代理和 GitHub Token 状态由后台定时检测。

策略：

- 服务启动后立即异步检测一次。
- 后续每 2 小时刷新一次。
- `/api/system/status` 只返回缓存结果。
- 探测 GitHub rate limit API，失败时最多重试 3 次。

## 7. 日志

后端日志写入：

```text
log/server.log
```

关键日志点：

- API 请求。
- GitHub API 请求、响应和重试。
- 组件检查运行创建、版本检查完成、漏洞检查完成。
- 修复版本解析过程。
- 通知发送、跳过和失败原因。
- 运行状态后台检测结果。

## 8. 错误处理

- 单个组件检查失败不影响其他组件。
- 版本检查失败会写入失败检查记录。
- 漏洞检查失败会结束检查运行并标记部分失败。
- 邮件发送失败会写入失败通知记录。
- 通知记录引用的检查记录不会被保留策略删除。
