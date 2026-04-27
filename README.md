# opensource-release-watcher

`opensource-release-watcher` 是一个用于监控开源组件版本发布的轻量级工具。

它定期检查 GitHub / GitLab 等开源仓库的 Release、Tag，以及后续可扩展的安全更新信息；当发现组件存在新版本或重要修复时，系统会根据配置，通过邮件通知对应订阅人。

不做即时通讯多渠道分发，不做自动升级，也不做复杂工作流编排。

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
- 组件 owner 不明确
- Release Note 无人跟踪

因此，本项目希望提供一个简单、可落地的机制，对开源组件版本变化进行持续监控，并通过邮件发送提醒。

## 功能目标

### 初版能力

- 维护一份开源组件清单
- 定期查询组件的最新 Release
- 当项目没有 Release 时，自动回退查询 Tag
- 记录上一次检查到的版本
- 避免重复发送相同版本通知
- 只支持邮件订阅与邮件通知
- 支持为不同组件配置不同 owner
- 支持生成版本更新摘要

### 后续可扩展能力

- 支持 GitHub Security Advisory
- 支持 OSV 漏洞数据库查询
- 支持 GitLab Release 查询增强
- 支持 Release Note 关键字分析
- 支持通知优先级
- 支持月度开源组件状态报告

## 核心流程

```text
定时调度触发检查任务
      ↓
启动检查流程
      ↓
读取 components.yaml
      ↓
读取组件清单
      ↓
查询 GitHub / GitLab Release
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

当某个组件发布新版本时，系统自动通过邮件通知负责人。

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

团队可以通过该工具维护内部使用的开源组件清单，包括：

- 当前使用版本
- 最新上游版本
- 组件负责人
- 检查时间
- 通知记录
- 升级状态

## 配置示例

```yaml
components:
  - name: protobuf
    source: github
    repo: protocolbuffers/protobuf
    current_version: "3.20.1"
    owner: "platform-team"
    subscribers:
      - platform@example.com
      - dev-lead@example.com
    watch:
      release: true
      tag: true
      security: true

  - name: opencv
    source: github
    repo: opencv/opencv
    current_version: "4.9.0"
    owner: "media-team"
    subscribers:
      - media@example.com
    watch:
      release: true
      tag: true
      security: true
```

`subscribers` 表示邮件接收人列表。

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
组件负责人：platform-team
发布时间：2026-xx-xx

Release Note 摘要：
- 修复若干 C++ runtime 问题
- 改进 generated code 行为
- 调整部分接口兼容性

建议动作：
- 请组件负责人评估是否需要升级
- 检查当前项目是否受到影响
- 如涉及安全修复，建议优先处理
```

## 推荐架构

推荐采用轻量级 Web 服务方式：

```text
Go HTTP Server + Scheduler + components.yaml + SQLite + SMTP
```

本项目初版不引入复杂通知通道，通知统一通过 SMTP 邮件完成。

## 模块设计

```text
opensource-release-watcher/
├── cmd/
│   └── server/
│       └── main.go
├── internal/
│   ├── config/
│   ├── github/
│   ├── gitlab/
│   ├── checker/
│   ├── scheduler/
│   ├── storage/
│   ├── notifier/
│   ├── service/
│   ├── api/
│   └── version/
├── configs/
│   └── components.yaml
├── templates/
│   └── release_email.md
├── data/
│   └── watcher.db
├── README.md
└── go.mod
```

## 模块说明

### config

负责读取组件配置文件，例如 `components.yaml`。

### github

负责调用 GitHub API，查询 Release 和 Tag。

### gitlab

负责调用 GitLab API，查询 Release 和 Tag。

### checker

负责组件检查逻辑，包括：

- 查询最新版本
- 判断是否有更新
- 解析 Release Note
- 判断通知优先级

### storage

负责保存检查状态，避免重复通知。

可以先使用 SQLite，后续再扩展到 PostgreSQL。

### notifier

负责邮件通知发送。

通知发送由邮件 notifier 负责；如果后续确实有需要，再演进为统一通知接口。

### version

负责版本号比较。

例如：

```text
v1.2.3 > v1.2.2
v2.0.0 > v1.9.9
```

## 运行方式

### 启动服务

服务启动后，按配置的调度策略定期执行组件检查任务。

例如：

```bash
opensource-release-watcher --config configs/components.yaml
```

## 项目定位

本项目不是包管理器，也不是自动升级工具。

它的核心定位是：

```text
开源组件版本变化感知工具
```

它当前解决的问题是：

```text
我们依赖的开源组件，什么时候发布了新版本？
这个新版本是否值得关注？
应该给谁发邮件？
是否已经通知过？
后续是否需要升级评估？
```

## Roadmap

### Phase 1

- [ ] 支持 GitHub Release 查询
- [ ] 支持 GitHub Tag 查询
- [ ] 支持 YAML 配置
- [ ] 支持 SQLite 状态存储
- [ ] 支持邮件通知
- [ ] 支持重复通知去重

### Phase 2

- [ ] 支持 GitLab Release 查询
- [ ] 支持 Release Note 关键字分析
- [ ] 支持通知优先级
- [ ] 支持组件 owner 配置
- [ ] 支持邮件模板

### Phase 3

- [ ] 支持 OSV 漏洞查询
- [ ] 支持 GitHub Security Advisory 查询
- [ ] 支持月度组件状态报告

## License

本项目采用 [MIT 许可](LICENSE)。
