# PRD: CloudSync CLI - 本地磁盘到 S3/OSS 同步工具

## 1. Introduction/Overview

CloudSync CLI 是一个面向企业数据备份和灾难恢复场景的命令行工具，支持将本地磁盘数据单向同步到 AWS S3 或阿里云 OSS。工具支持灵活的定时策略、目录级压缩配置、混合保留策略，可长期作为守护进程后台运行。

## 2. Goals

- 提供可靠、高效的本地上云备份能力
- 支持多种云存储后端（S3、OSS）统一接口
- 实现灵活的定时调度（Cron 表达式）
- 支持细粒度的压缩策略配置（按目录）
- 实现智能的备份保留策略（天数 + 数量混合）
- 提供完整的运行状态监控和日志记录
- 支持长期后台守护进程运行

## 3. User Stories

### US-001: 初始化配置文件
**Description:** 作为运维人员，我希望通过简单命令生成配置文件模板，以便快速开始配置备份任务。

**Acceptance Criteria:**
- [ ] `cloudsync init` 命令生成默认配置文件 `cloudsync.yaml`
- [ ] 配置文件包含所有必要字段的注释说明
- [ ] 自动检测并提示配置云存储凭证
- [ ] Typecheck/lint passes

### US-002: 配置云存储后端
**Description:** 作为运维人员，我希望在配置文件中指定 S3 或 OSS 作为目标存储，以便灵活选择云服务商。

**Acceptance Criteria:**
- [ ] 支持配置 AWS S3（accessKey, secretKey, region, bucket）
- [ ] 支持配置阿里云 OSS（accessKeyId, accessKeySecret, endpoint, bucket）
- [ ] 支持从环境变量或凭证文件读取敏感信息
- [ ] 配置验证命令 `cloudsync validate` 检查凭证有效性
- [ ] Typecheck/lint passes

### US-003: 配置同步任务（Source → Target）
**Description:** 作为运维人员，我希望配置多个本地目录同步到不同的云端路径，以便管理多源备份。

**Acceptance Criteria:**
- [ ] 支持配置多个同步任务，每个任务包含 source 和 target 路径
- [ ] Source 支持绝对路径，支持通配符匹配多个目录
- [ ] Target 支持自定义云端路径前缀
- [ ] 云端目录按日期结构组织：`{prefix}/{YYYY}/{MM}/{DD}/{HHmmss}/`，每次备份独立目录避免命名冲突
- [ ] 支持包含/排除规则（glob 模式）
- [ ] Typecheck/lint passes

### US-004: 配置定时调度（Cron 表达式）
**Description:** 作为运维人员，我希望使用 Cron 表达式配置备份频率，以便精确控制备份时间。

**Acceptance Criteria:**
- [ ] 支持标准 Cron 表达式（5 字段格式：分 时 日 月 周）
- [ ] 支持特殊字符：`*` `,` `-` `/` `L` `W` `#`
- [ ] 提供常用预设：`@daily` `@hourly` `@weekly` `@monthly`
- [ ] 支持任务级别的独立调度配置
- [ ] 启动时显示下次执行时间预览
- [ ] Typecheck/lint passes

### US-005: 配置压缩策略（按目录）
**Description:** 作为运维人员，我希望对不同目录配置不同的压缩策略，以便平衡存储成本和备份速度。

**Acceptance Criteria:**
- [ ] 支持全局默认压缩配置（启用/禁用、算法、级别）
- [ ] 支持为每个同步任务覆盖压缩配置
- [ ] 支持压缩算法：gzip、zlib、snappy（优先速度）、lz4、zstd（优先压缩率）
- [ ] 支持按文件大小阈值决定是否压缩（如大于 10MB 才压缩）
- [ ] 支持按文件类型黑白名单（如只压缩 .log .txt，不压缩 .zip .gz）
- [ ] Typecheck/lint passes

### US-006: 配置保留策略（天数 + 数量混合）
**Description:** 作为运维人员，我希望同时按备份天数和版本数量控制保留，以便在满足合规要求的同时控制存储成本。

**Acceptance Criteria:**
- [ ] 支持配置最大保留天数（如 90 天）
- [ ] 支持配置最大版本数量（如 30 个版本）
- [ ] 混合策略：同时满足两个条件，任一超限即触发清理
- [ ] 支持按任务配置独立的保留策略
- [ ] 清理前支持 dry-run 预览将被删除的文件
- [ ] Typecheck/lint passes

### US-007: 执行手动同步
**Description:** 作为运维人员，我希望手动触发一次同步，以便测试配置或执行紧急备份。

**Acceptance Criteria:**
- [ ] `cloudsync sync [task-name]` 执行指定任务同步
- [ ] `cloudsync sync --all` 执行所有任务同步
- [ ] 支持 `--dry-run` 模式预览变更而不实际执行
- [ ] 实时显示同步进度（文件数、字节数、速度）
- [ ] 同步完成输出统计报告（成功/失败/跳过数量）
- [ ] Typecheck/lint passes

### US-008: 启动守护进程
**Description:** 作为运维人员，我希望将工具作为守护进程长期运行，以便按计划自动执行备份。

**Acceptance Criteria:**
- [ ] `cloudsync daemon start` 启动守护进程
- [ ] `cloudsync daemon stop` 停止守护进程
- [ ] `cloudsync daemon status` 查看运行状态和任务计划
- [ ] 支持生成 PID 文件和日志文件
- [ ] 支持信号处理：SIGTERM 优雅退出、SIGHUP 重载配置
- [ ] 崩溃后自动重启（可配置）
- [ ] Typecheck/lint passes

### US-009: 查看同步状态和日志
**Description:** 作为运维人员，我希望查看同步历史和当前状态，以便监控备份健康状况。

**Acceptance Criteria:**
- [ ] `cloudsync status` 显示所有任务最后执行状态和下次计划时间
- [ ] `cloudsync logs [task-name]` 查看指定任务日志
- [ ] `cloudsync history [task-name]` 显示历史执行记录
- [ ] 日志支持 JSON 格式便于机器解析
- [ ] 支持日志轮转（按大小或时间）
- [ ] Typecheck/lint passes

### US-010: 执行备份清理
**Description:** 作为运维人员，我希望手动执行过期备份清理，以便释放存储空间。

**Acceptance Criteria:**
- [ ] `cloudsync cleanup [task-name]` 清理指定任务的过期备份
- [ ] `cloudsync cleanup --all` 清理所有任务
- [ ] 支持 `--dry-run` 预览将被删除的文件
- [ ] 清理前确认提示（可 `--force` 跳过）
- [ ] 输出清理统计（释放空间、删除文件数）
- [ ] Typecheck/lint passes

### US-011: 配置 MeoW 推送通知
**Description:** 作为运维人员，我希望在备份任务完成或失败时通过 MeoW 推送接收通知，以便及时了解备份状态。

**Acceptance Criteria:**
- [ ] 支持配置 MeoW 推送的 API 端点和认证参数
- [ ] 备份任务成功/失败时自动发送推送通知
- [ ] 推送内容包含任务名称、执行时间、结果摘要（成功/失败文件数）
- [ ] 支持配置推送级别：always、on-error、never
- [ ] 支持自定义推送消息模板（标题和正文）
- [ ] 推送失败不影响备份任务执行，仅记录警告日志
- [ ] Typecheck/lint passes

## 4. Functional Requirements

### 核心功能

- **FR-1: 配置管理**
  - FR-1.1: 配置文件使用 YAML 格式，默认路径 `~/.cloudsync/config.yaml`
  - FR-1.2: 支持通过 `--config` 参数指定自定义配置文件路径
  - FR-1.3: 配置变更后支持热重载（SIGHUP 信号）
  - FR-1.4: 配置验证命令检查语法和必填字段

- **FR-2: 存储后端支持**
  - FR-2.1: AWS S3 支持：标准存储、服务端加密（SSE-S3/SSE-KMS）
  - FR-2.2: 阿里云 OSS 支持：标准存储、服务端加密
  - FR-2.3: 统一抽象接口，便于后续扩展其他存储（如 GCS、MinIO）

- **FR-3: 同步机制**
  - FR-3.1: 云端目录按日期结构组织：`{prefix}/{YYYY}/{MM}/{DD}/{HHmmss}/`，每次备份独立目录
  - FR-3.2: 同一任务多次备份按时间戳分目录存储，避免文件命名冲突
  - FR-3.3: 断点续传：大文件分片上传，支持失败重传
  - FR-3.4: 文件校验：上传后验证 MD5/SHA256 校验和

- **FR-4: 压缩功能**
  - FR-4.1: 支持压缩算法：gzip、zlib、snappy、lz4、zstd
  - FR-4.2: 支持压缩级别配置（1-9，数值越大压缩率越高）
  - FR-4.3: 支持目录级压缩策略覆盖全局配置
  - FR-4.4: 压缩后的文件保持元数据（原始文件名、压缩算法）

- **FR-5: 定时调度**
  - FR-5.1: 使用 Cron 表达式解析器，支持标准 5 字段格式
  - FR-5.2: 支持特殊字符解析和预设别名
  - FR-5.3: 支持时区配置（默认系统时区）
  - FR-5.4: 错过执行时间后的处理策略（立即执行/跳过/等待下次）

- **FR-6: 保留策略**
  - FR-6.1: 支持按天数保留：删除超过指定天数的备份
  - FR-6.2: 支持按数量保留：只保留最近 N 个备份版本
  - FR-6.3: 混合策略：同时检查天数和数量，任一条件触发清理
  - FR-6.4: 清理操作记录日志，支持审计

- **FR-7: 守护进程**
  - FR-7.1: 支持前台/后台运行模式
  - FR-7.2: 生成 PID 文件防止重复启动
  - FR-7.3: 支持系统服务集成（systemd、launchd）
  - FR-7.4: 优雅退出：接收到 SIGTERM 后完成当前任务再退出

- **FR-8: 日志和监控**
  - FR-8.1: 支持多级别日志：debug、info、warn、error
  - FR-8.2: 支持日志输出到文件和标准输出
  - FR-8.3: 支持结构化日志（JSON 格式）
  - FR-8.4: 日志轮转：按大小（如 100MB）或时间（如每天）
  - FR-8.5: 任务执行状态持久化，支持查询历史记录

- **FR-9: MeoW 推送通知**
  - FR-9.1: 支持配置 MeoW API 地址，默认 `https://api.chuckfang.com/JohnDoe`
  - FR-9.2: 推送请求格式：`POST {url}?msgType=html&htmlHeight=350`，Content-Type: `application/json`
  - FR-9.3: 推送内容包含：title（标题）、msg（HTML 格式消息）、url（可选链接）
  - FR-9.4: 支持配置推送触发条件：always（每次）、on-error（仅失败）、never（禁用）
  - FR-9.5: 推送失败不影响备份任务，记录 warning 日志

- **FR-10: 错误处理和重试**
  - FR-10.1: 网络错误自动重试（指数退避策略）
  - FR-10.2: 可配置重试次数和退避参数
  - FR-10.3: 失败任务记录详情，支持手动重试

- **FR-11: 安全性**
  - FR-11.1: 云存储凭证支持环境变量、配置文件、凭证文件多种方式
  - FR-11.2: 配置文件中的敏感信息支持加密存储
  - FR-11.3: 上传支持服务端加密
  - FR-11.4: 本地临时文件安全清理

## 5. Non-Goals (Out of Scope)

以下功能不在本期实现范围内：

- **双向同步**：不支持从云端同步到本地
- **数据去重**：不支持跨文件块级去重
- **增量备份**：不支持基于块级的增量（只支持文件级增量）
- **多机协作**：不支持分布式备份或主从架构
- **Web UI**：不提供图形化管理界面
- **实时同步**：不支持文件系统事件触发的实时备份
- **备份恢复**：不提供从云端恢复到本地的自动化工具（需手动下载）
- **压缩加密**：不支持压缩后的加密（可依赖服务端加密）
- **跨区域复制**：不管理存储桶跨区域复制配置
- **Windows 支持**：本期仅支持 Linux 和 macOS
- **云端数据完整性扫描**：不主动扫描云端数据完整性
- **带宽限速**：不支持限制上传/下载速度

## 6. Design Considerations

### 6.1 配置文件结构

```yaml
version: "1"

# 全局配置
global:
  log_level: info
  log_format: json  # 或 text
  log_path: ~/.cloudsync/logs
  retry:
    max_attempts: 3
    backoff: exponential

# 存储后端配置
storage:
  type: s3  # 或 oss
  s3:
    region: us-east-1
    bucket: my-backup-bucket
    access_key: ${AWS_ACCESS_KEY_ID}
    secret_key: ${AWS_SECRET_ACCESS_KEY}
    encryption: AES256
  # 或 oss:
  #   endpoint: oss-cn-beijing.aliyuncs.com
  #   bucket: my-backup-bucket
  #   access_key_id: ${OSS_ACCESS_KEY_ID}
  #   access_key_secret: ${OSS_ACCESS_KEY_SECRET}

# 同步任务配置
tasks:
  - name: app-logs
    schedule: "0 2 * * *"  # 每天凌晨 2 点
    source:
      path: /var/log/myapp
      include:
        - "*.log"
      exclude:
        - "*.tmp"
    target:
      prefix: backups/app-logs
      # 云端目录结构: backups/app-logs/2024/01/15/020000/
      date_format: "2006/01/02/150405"  # Go 时间格式
    compression:
      enabled: true
      algorithm: zstd
      level: 3
      min_size: 1MB  # 小于此值不压缩
    retention:
      max_days: 90
      max_versions: 30
    notify:
      level: on-error  # always | on-error | never

  - name: database
    schedule: "0 3 * * 0"  # 每周日凌晨 3 点
    source:
      path: /data/postgres/backups
    target:
      prefix: backups/database
      date_format: "2006/01/02/150405"
    compression:
      enabled: false  # 数据库已压缩
    retention:
      max_days: 180
      max_versions: 12
    notify:
      level: always

# MeoW 推送配置
notify:
  meow:
    enabled: true
    api_url: "https://api.chuckfang.com/JohnDoe"  # 替换为实际的推送地址
    msg_type: html
    html_height: 350
    # 默认消息模板
    templates:
      success:
        title: "✅ 备份成功: {{.TaskName}}"
        msg: "<p><b>任务:</b> {{.TaskName}}</p><p><b>时间:</b> {{.Duration}}</p><p><b>文件:</b> {{.FileCount}} 个 ({{.TotalSize}})</p>"
        url: ""
      failure:
        title: "❌ 备份失败: {{.TaskName}}"
        msg: "<p><b>任务:</b> {{.TaskName}}</p><p><b>错误:</b> {{.Error}}</p><p><b>已备份:</b> {{.SuccessCount}}/{{.TotalCount}} 个文件</p>"
        url: ""
```

### 6.2 命令行接口

```bash
# 配置管理
cloudsync init [--path /path/to/config.yaml]
cloudsync validate [--config /path/to/config.yaml]

# 手动执行
cloudsync sync <task-name> [--config ...] [--dry-run]
cloudsync sync --all [--config ...] [--dry-run]

# 守护进程
cloudsync daemon start [--config ...] [--foreground]
cloudsync daemon stop
cloudsync daemon status
cloudsync daemon reload  # 发送 SIGHUP 重载配置

# 查看状态
cloudsync status [--config ...]
cloudsync logs <task-name> [--follow] [--lines 100]
cloudsync history <task-name> [--limit 20]

# 清理
cloudsync cleanup <task-name> [--dry-run] [--force]
cloudsync cleanup --all [--dry-run] [--force]
```

### 6.3 本地目录结构

```
~/.cloudsync/
├── config.yaml          # 主配置文件
├── logs/
│   ├── cloudsync.log    # 主日志文件
│   ├── cloudsync.log.1  # 轮转日志
│   └── tasks/
│       ├── app-logs.log
│       └── database.log
├── data/
│   ├── cloudsync.pid    # 守护进程 PID
│   ├── state.db         # 任务状态数据库 (SQLite)
│   └── tmp/             # 临时文件目录
└── credentials/         # 加密存储的凭证（可选）
```

### 6.4 云端目录结构设计

每次备份在云端创建独立的日期目录，避免文件命名冲突：

```
{prefix}/{YYYY}/{MM}/{DD}/{HHmmss}/
```

**示例：**
```
backups/app-logs/
├── 2024/
│   ├── 01/
│   │   ├── 14/
│   │   │   └── 020000/          # 1月14日 02:00:00 的备份
│   │   │       ├── app.log
│   │   │       └── error.log
│   │   └── 15/
│   │       └── 020000/          # 1月15日 02:00:00 的备份
│   │           ├── app.log      # 文件名不变，目录区分版本
│   │           └── error.log
│   └── 02/
│       └── 01/
│           └── 020000/
└── ...
```

**保留策略清理逻辑：**
- 根据目录路径中的日期解析备份时间
- 删除超过 `max_days` 的整份备份（删除整个日期目录）
- 当版本数超过 `max_versions` 时，删除最旧的完整备份目录

### 6.5 MeoW 推送接口规范

**请求格式：**
```
POST /JohnDoe?msgType=html&htmlHeight=350 HTTP/1.1
Host: api.chuckfang.com
Content-Type: application/json

{
  "title": "备份通知",
  "msg": "<p><b>任务:</b> app-logs</p><p><b>结果:</b> 成功</p>",
  "url": "https://example.com"
}
```

**响应格式：**
```json
{
  "status": 200,
  "message": "推送成功"
}
```

**变量模板：**
| 变量 | 说明 |
|------|------|
| `{{.TaskName}}` | 任务名称 |
| `{{.StartTime}}` | 开始时间 |
| `{{.EndTime}}` | 结束时间 |
| `{{.Duration}}` | 执行时长 |
| `{{.Status}}` | 状态：success / failure |
| `{{.FileCount}}` | 文件总数 |
| `{{.SuccessCount}}` | 成功上传数 |
| `{{.FailedCount}}` | 失败数 |
| `{{.TotalSize}}` | 总大小（人类可读） |
| `{{.Error}}` | 错误信息（失败时） |

## 7. Technical Considerations

### 7.1 技术栈

- **语言**: Go（跨平台、单二进制文件部署、丰富的云 SDK）
- **Cron 解析**: github.com/robfig/cron/v3
- **配置解析**: gopkg.in/yaml.v3
- **AWS SDK**: github.com/aws/aws-sdk-go-v2
- **阿里云 SDK**: github.com/aliyun/aliyun-oss-go-sdk
- **压缩库**:
  - gzip/zlib: 标准库
  - snappy: github.com/golang/snappy
  - lz4: github.com/pierrec/lz4
  - zstd: github.com/klauspost/compress/zstd
- **CLI 框架**: github.com/spf13/cobra
- **日志**: github.com/rs/zerolog 或标准库 log/slog

### 7.2 架构设计

```
┌─────────────────────────────────────────────────────────────┐
│                        CLI Layer                            │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌────────────────┐  │
│  │  init    │ │  sync    │ │  daemon  │ │  status/logs   │  │
│  └──────────┘ └──────────┘ └──────────┘ └────────────────┘  │
└─────────────────────────────────────────────────────────────┘
                              │
┌─────────────────────────────────────────────────────────────┐
│                     Scheduler/Cron                          │
│                    (robfig/cron)                            │
└─────────────────────────────────────────────────────────────┘
                              │
┌─────────────────────────────────────────────────────────────┐
│                      Task Engine                            │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────────┐   │
│  │   Scanner    │  │  Compressor  │  │  Uploader        │   │
│  │  (walk dir)  │  │ (gzip/lz4/..)│  │ (S3/OSS client)  │   │
│  └──────────────┘  └──────────────┘  └──────────────────┘   │
└─────────────────────────────────────────────────────────────┘
                              │
┌─────────────────────────────────────────────────────────────┐
│                    Storage Interface                        │
│         ┌──────────┐              ┌──────────┐              │
│         │  S3      │              │   OSS    │              │
│         │ Adapter  │              │ Adapter  │              │
│         └──────────┘              └──────────┘              │
└─────────────────────────────────────────────────────────────┘
```

### 7.3 性能考虑

- **并发上传**: 使用并发上传提高吞吐量，不限制带宽占用
- **内存优化**: 大文件使用流式处理，避免全量加载到内存
- **压缩优化**: 使用流式压缩，边压缩边上传，减少磁盘 I/O
- **索引缓存**: 本地缓存云端目录列表，减少 API 调用（用于清理过期备份）

### 7.4 错误处理策略

| 错误类型 | 处理策略 |
|---------|---------|
| 网络超时 | 指数退避重试（1s → 2s → 4s → 8s）|
| 认证失败 | 立即失败，记录错误，发送告警 |
| 磁盘读取错误 | 跳过该文件，记录警告，继续其他文件 |
| 云端 API 限流 | 退避重试，增加重试间隔 |
| 配置错误 | 启动时校验失败，输出详细错误信息 |

### 7.5 安全考虑

- 凭证优先从环境变量读取，避免硬编码
- 支持配置文件敏感字段加密（AES-256-GCM）
- 临时文件使用 0600 权限创建
- 支持 TLS/SSL 证书验证（可配置跳过，不推荐）

## 8. Success Metrics

- **可靠性**: 备份成功率 > 99.5%（排除配置错误）
- **性能**: 单文件上传速度达到网络带宽的 80% 以上
- **资源占用**: 内存占用 < 500MB（默认配置）
- **易用性**: 新用户可在 10 分钟内完成首次配置并执行备份
- **可观测性**: 所有操作都有日志记录，关键指标可查询

## 9. Open Questions

1. **备份版本标识**: 同一文件的多次备份如何命名？使用时间戳（`file_20240115_143022.log`）还是版本号？（注：由于每次备份在独立日期目录，文件名可保持不变）

2. **告警渠道**: 除 MeoW 外，是否需要支持邮件/钉钉/企业微信等其他告警渠道？(不需要)

3. **失败重试策略**: 任务失败后是否需要在下一次调度时间前自动重试？（失败后需要重试一次，还失败，推送通知）
