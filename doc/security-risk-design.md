# 安全风险判断设计

## 1. 目标

本设计用于回答一个更直接的问题：

- 当前正在使用的版本，是否已经存在公开已知的漏洞或缺陷风险？

这里的“风险”不只包含传统安全漏洞，也包含已经被上游明确修复、但当前版本仍可能受影响的已知缺陷。

## 2. 设计原则

### 2.1 版本监控和风险判断分离

版本监控负责回答：

- 有没有新版本。
- 新版本是多少。
- 需不需要通知订阅人。

风险判断负责回答：

- 当前版本是否命中已知漏洞。
- 当前版本是否存在上游已确认的缺陷风险。

这两者不要混成一个字段。

### 2.2 证据优先，不做臆测

风险判断必须基于可追溯证据，而不是根据“看起来像有问题”去推断。

证据可以来自：

- OSV。
- 上游项目 security advisory。
- Release Note / Changelog。

### 2.3 自动判定优先

- 能命中明确版本范围的，自动判定。
- 只写了“fix / hardening / security / vulnerability”的，作为辅助证据。
- 没有公开证据的，保持未识别。

## 3. 风险分类

建议把风险分成三类：

### 3.1 漏洞

已公开披露，且有明确受影响版本范围。

特征：

- 有 OSV 标识，或可映射到 OSV 的来源。
- 有 vulnerable version range。
- 常见有 fixed version。

### 3.2 已知缺陷

上游已经确认存在问题，但不一定被正式归类为安全漏洞。

特征：

- Release Note / Changelog 明确提到修复。
- Issue 或公告明确说明问题影响范围。
- 可能没有 CVE / GHSA 编号。

说明：

- 这一类属于后续扩展。
- 第一版不进入自动判定结果。

### 3.3 未识别

第一版中，未识别表示当前没有足够证据判断风险。

特征：

- 无法解析版本对应的 Git 提交。
- OSV commit 查询成功，但未返回漏洞记录。
- 当前数据源暂不覆盖该组件或该版本。

## 4. 风险来源

### 4.1 OSV 数据源

OSV 是主查询入口，内部会聚合多种来源的漏洞数据。

适合作为 C/C++ 源码库的主查询入口。

用途：

- 做 commit 级漏洞匹配。
- 聚合不同来源的漏洞数据。
- 提供统一的 API 查询能力。

### 4.2 OSV 查询模式

本项目第一版仅使用 OSV 的 `commit hash` 查询能力。

第一版主路径如下：

1. `repo_url + current_version/tag`
2. 解析到 `commit_sha`
3. 使用 `commit_sha` 查询 OSV
4. OSV commit 查询返回漏洞记录，且该漏洞记录确认影响当前 commit 时，标记为“有漏洞”
5. OSV 未返回漏洞记录时，标记为“未识别”
6. commit 解析失败时，标记为“未识别”，并记录原因
7. OSV 查询失败时，标记为“检查失败”，并记录原因

这样做的好处是：

- 对 C/C++ 源码仓库更直接。
- 不依赖 package 生态字段。
- 和 OSV 的 C/C++ commit-range 模型一致。

### 4.3 上游安全公告 / Release Note / Changelog

作为后续扩展的辅助证据来源，第一版不作为主查询链路。

用途：

- 判断当前版本是否包含已知缺陷。
- 给出“建议升级到哪个版本”。

## 5. 判定流程

第一版判定流程如下：

```text
读取组件当前版本和 repo_url
      ↓
是否已缓存 `security_commit_sha`？
      ├── 是 → 使用 `security_commit_sha`
      ↓ 否
根据 `repo_url + current_version/tag` 解析出 `commit_sha`
      ↓
commit_sha 是否解析成功？
      ├── 否 → 标记为“未识别”
      │        原因：无法解析版本对应的 Git 提交
      ↓ 是
使用 `commit_sha` 查询 OSV
      ↓
OSV 查询是否成功？
      ├── 否 → 标记为“检查失败”
      │        原因：OSV 查询失败
      ↓ 是
是否返回影响当前 commit 的漏洞记录？
      ├── 是 → 标记为“有漏洞”
      ↓ 否
标记为“未识别”
原因：OSV commit 查询未命中公开已知漏洞
```

## 6. 建议的数据模型

### 6.1 组件主表保留的信息

- `repo_url`
- `name`
- `current_version`
- `latest_version`
- `check_strategy`
- `enabled`

### 6.2 新增风险记录表

建议增加一张独立表，记录每次风险判断结果：

| 字段 | 说明 |
| --- | --- |
| component_id | 组件 ID |
| version | 被检查的版本 |
| commit_sha | 对应的 Git 提交，若可解析则记录，可选 |
| risk_type | 第一版固定为 `vulnerability` |
| risk_status | `affected` / `unknown` / `check_failed` |
| source | `osv` |
| identifier | OSV ID |
| affected_range | OSV 返回的受影响 commit range 或版本范围 |
| fixed_version | 修复版本，若存在 |
| severity | 严重性 |
| confidence | 置信度 |
| summary | 风险摘要 |
| status_reason | 当前状态原因 |
| raw_payload | OSV 原始响应 |
| checked_at | 检查时间 |

### 6.3 组件风险配置

如果某些组件需要特别处理，可以单独加配置字段：

- `security_mode`
  - 预留字段，第一版不启用
- `security_lookup_mode`
  - `commit_first`
- `security_aliases`
  - 预留字段，第二阶段 Release Note / Changelog 检索时使用
- `security_package_name`
  - 预留字段，第一版不使用
- `security_ecosystem`
  - 预留字段，第一版不使用
- `security_commit_sha`
  - 若组件版本能稳定映射到固定提交，可缓存该提交，用于避免重复解析 tag
- `security_tag_pattern`
  - 用于将 `current_version` 转换为 Git tag，示例：`v{version}`、`{version}`、`release-{version}`，第一版可默认尝试 `{version}` 和 `v{version}`
- `security_notes`
  - 风险备注

默认 tag 解析顺序：

1. `{version}`
2. `v{version}`

如果仍然无法解析，则标记为“未识别”，原因记录为：`无法解析版本对应的 Git 提交`。

## 7. 前端展示建议

第一版建议在仪表盘和组件详情页展示：

- 风险状态：有漏洞 / 未识别 / 检查失败
- 证据来源：OSV
- 受影响版本范围
- 修复版本
- 最近检查时间
- 状态原因

展示文案上要区分：

- `有漏洞`
- `未识别`
- `检查失败`

不要把“未识别”误写成“安全”或“正常”。

## 8. 适用边界

这套方案适合：

- 想判断“当前版本是否存在公开已知漏洞”。
- 想优先自动判断，后续再扩展更复杂的风险证据来源。

不适合：

- 纯源码审计。
- 自动修复。
- 未经过版本映射的随意猜测。

## 9. 推荐演进顺序

### 第一阶段

- 只接入 OSV commit 查询。
- 做 tag/version -> commit_sha 的映射。
- 若已缓存 `security_commit_sha`，优先复用。
- 输出 `有漏洞 / 未识别 / 检查失败`。
- `risk_type` 第一版固定为 `vulnerability`。
- `risk_status` 第一版固定为 `affected / unknown / check_failed`。

### 第二阶段

- 接入上游 Release Note / Changelog / Advisory 作为辅助证据。
- 识别已知缺陷。
- 输出 `有已知缺陷`。
- 第二阶段再引入 `bug` 类型。

### 第三阶段

- 做风险解释优化。
- 做展示和查询优化。
- 做风险责任闭环。

## 10. 结论

安全风险判断第一版不应该再依赖 package 生态模型。

更合理的方式是：

- 版本监控继续负责“有没有新版本”。
- 风险判断负责“当前版本有没有已知风险”。
- 第一版主来源是 OSV commit 查询。
- Release Note / Changelog 作为后续辅助来源。
- 第一版只记录 `vulnerability` 类型。
- 第一版的状态只使用 `affected / unknown / check_failed`。

## 11. 实现落点

### 11.1 后端模块

建议新增一个安全判断子模块，职责包括：

- 从组件表读取 `repo_url`、`current_version`、`security_*` 配置。
- 解析 `current_version/tag` 到 `commit_sha`。
- 查询 OSV commit。
- 写入风险记录和风险摘要。

可以按下面的分层落地：

- `security`：安全查询编排。
- `gitrepo`：tag/version 到 commit_sha 的解析。
- `osv`：OSV API 客户端。
- `storage`：风险记录持久化。

### 11.2 处理时机

风险判断建议在以下时机触发：

- 组件新增后。
- 组件 `current_version` 更新后。
- 定时全量检查完成后。
- 手动触发单个组件检查后。

这样可以保证：

- 新版本进入系统时就能尽快做风险判断。
- 仪表盘始终展示最新风险状态。

### 11.3 前端展示

前端建议分两层展示：

1. 仪表盘卡片级别
   - `风险状态`
   - `有漏洞 / 未识别 / 检查失败`
2. 组件详情页
   - 命中的证据
   - 受影响范围
   - 修复版本
   - 风险摘要

### 11.4 接口建议

如果后续要正式落地，建议补充以下接口：

- `GET /api/components/{id}/security-summary`
- `GET /api/security-records`
- `GET /api/security-records/{id}`

如果只做最小实现，也可以先把风险数据挂在：

- 组件详情
- 检查记录详情
- 仪表盘汇总

### 11.5 设计约束

- OSV 是主查询源。
- Release Note / Changelog 作为后续扩展的辅助证据，第一版不参与自动判定。
- 风险记录必须和具体版本绑定。
- 同一组件同一版本的风险记录可以多条，但要保留最新摘要。
- 不要把“没有命中”直接等同于“安全”，只能说“未识别”。
