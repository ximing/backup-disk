# CloudSync CLI

本地磁盘到 S3/OSS 同步工具 - 支持定时调度、压缩策略、保留策略和 MeoW 推送通知。

[![Release](https://github.com/ximing/cloudsync/actions/workflows/release.yml/badge.svg)](https://github.com/ximing/cloudsync/actions/workflows/release.yml)
[![Go Version](https://img.shields.io/badge/go-1.24+-blue.svg)](https://golang.org)

## 功能特性

- **多存储后端支持**: AWS S3、阿里云 OSS
- **定时调度**: 基于 Cron 表达式的自动备份计划
- **压缩支持**: gzip、zstd 压缩算法
- **保留策略**: 按天数和版本数量自动清理旧备份
- **守护进程模式**: 后台运行，按计划自动执行
- **MeoW 推送通知**: 备份完成/失败时接收通知
- **失败重试**: 指数退避重试策略
- **日志管理**: 分级日志、日志轮转、任务分离日志

## 快速开始

### 安装

#### 从 Release 下载

```bash
# macOS (Intel)
curl -L -o cloudsync.tar.gz https://github.com/ximing/cloudsync/releases/latest/download/cloudsync-darwin-amd64.tar.gz

# macOS (Apple Silicon)
curl -L -o cloudsync.tar.gz https://github.com/ximing/cloudsync/releases/latest/download/cloudsync-darwin-arm64.tar.gz

# Linux (AMD64)
curl -L -o cloudsync.tar.gz https://github.com/ximing/cloudsync/releases/latest/download/cloudsync-linux-amd64.tar.gz

# 解压
tar -xzf cloudsync.tar.gz
chmod +x cloudsync
sudo mv cloudsync /usr/local/bin/
```

#### 从源码构建

```bash
git clone https://github.com/ximing/cloudsync.git
cd cloudsync
go build -o cloudsync ./cmd/cloudsync
```

### 初始化配置

```bash
# 生成默认配置文件
cloudsync init

# 编辑配置文件
vim ~/.cloudsync/config.yaml
```

### 验证配置

```bash
cloudsync validate
```

### 测试存储连接

测试 S3/OSS 存储后端的连接是否正常：

```bash
# 测试连接（默认 10 秒超时）
cloudsync test

# 设置超时时间
cloudsync test --timeout 30
```

输出示例：
```
Storage Type: s3
  Region: us-east-1
  Bucket: my-backup-bucket

Creating storage client...
Testing connection (timeout: 10s)...

Connection test SUCCESS!
  Response time: 245ms

Configuration is ready for sync operations.
```

### 手动执行同步

```bash
# 执行指定任务
cloudsync sync my-backup

# 执行所有任务
cloudsync sync --all

# 干运行模式（预览变更）
cloudsync sync my-backup --dry-run
```

### 启动守护进程

```bash
# 启动守护进程
cloudsync daemon start

# 查看状态
cloudsync daemon status

# 前台运行（调试用）
cloudsync daemon start --foreground

# 停止守护进程
cloudsync daemon stop
```

## 配置文件

配置文件位于 `~/.cloudsync/config.yaml`：

```yaml
# 全局配置
global:
  log_level: info          # debug, info, warn, error
  log_format: text         # text, json
  max_log_size: 100        # MB，日志轮转大小

# 存储后端配置
backends:
  - name: my-s3
    type: s3               # s3 或 oss
    region: us-east-1
    bucket: my-backup-bucket
    access_key: ${AWS_ACCESS_KEY_ID}      # 支持环境变量
    secret_key: ${AWS_SECRET_ACCESS_KEY}
    encryption: AES256     # 可选

  - name: my-oss
    type: oss
    endpoint: oss-cn-beijing.aliyuncs.com
    bucket: my-backup-bucket
    access_key_id: ${OSS_ACCESS_KEY_ID}
    access_key_secret: ${OSS_ACCESS_KEY_SECRET}

# 同步任务配置
tasks:
  - name: my-backup
    schedule: "0 2 * * *"  # Cron 表达式，每天凌晨 2 点
    source:
      path: /home/user/documents
      include:
        - "**/*.pdf"
        - "**/*.docx"
      exclude:
        - "**/temp/**"
        - "**/.git/**"
    target:
      backend: my-s3
      prefix: backups/documents
      date_format: "YYYY/MM/DD/HHmmss"  # 日期目录格式
    compression:
      enabled: true
      type: zstd             # gzip 或 zstd
      mode: file             # file (单文件) 或 archive (打包)
      level: 3               # 1-9
      min_size: 1024         # 最小压缩字节数 (file 模式)
      exclude_extensions:    # (file 模式)
        - ".zip"
        - ".gz"
      archive_name: ""       # 压缩包名 (archive 模式，可选)
    retention:
      max_days: 30           # 保留 30 天
      max_versions: 10       # 最多保留 10 个版本
    notify:
      enabled: true
      on: always             # always, on-error, never
      api_url: https://api.meow.example.com/push
      msg_type: html
      html_height: 350
      template: |
        <h3>CloudSync: {{.TaskName}}</h3>
        <p>状态: {{.Status}}</p>
        <p>文件数: {{.FileCount}}</p>
        <p>耗时: {{.Duration}}</p>
```

## 命令参考

### 全局选项

```bash
cloudsync [command] [flags]
```

### 命令列表

| 命令 | 说明 |
|------|------|
| `init` | 初始化配置目录和默认配置文件 |
| `validate` | 验证配置文件语法 |
| `test` | 测试存储后端连接 |
| `sync <task>` | 手动执行同步任务 |
| `cleanup <task>` | 清理过期备份 |
| `daemon start/stop/status` | 守护进程管理 |
| `status` | 查看任务状态 |
| `logs [task]` | 查看日志 |
| `history [task]` | 查看执行历史 |

### 详细用法

#### test 命令

```bash
# 测试存储连接（默认 10 秒超时）
cloudsync test

# 设置自定义超时时间
cloudsync test --timeout 30
```

连接测试会验证：
- 凭证是否有效
- Bucket 是否存在且可访问
- 网络连接是否正常

#### sync 命令

```bash
# 执行单个任务
cloudsync sync my-backup

# 执行所有任务
cloudsync sync --all

# 干运行模式（不实际上传）
cloudsync sync my-backup --dry-run

# 强制同步（跳过某些检查）
cloudsync sync my-backup --force
```

#### cleanup 命令

```bash
# 清理指定任务的过期备份
cloudsync cleanup my-backup

# 清理所有任务
cloudsync cleanup --all

# 干运行模式（预览将被删除的文件）
cloudsync cleanup my-backup --dry-run

# 强制清理（跳过确认）
cloudsync cleanup my-backup --force
```

#### daemon 命令

```bash
# 后台启动守护进程
cloudsync daemon start

# 前台运行（调试用）
cloudsync daemon start --foreground

# 停止守护进程
cloudsync daemon stop

# 查看守护进程状态
cloudsync daemon status

# 重载配置（发送 SIGHUP）
cloudsync daemon reload
```

#### status 命令

```bash
# 查看所有任务状态
cloudsync status

# 查看指定任务状态
cloudsync status my-backup
```

#### logs 命令

```bash
# 查看最新日志
cloudsync logs

# 查看指定任务日志
cloudsync logs my-backup

# 实时跟踪日志
cloudsync logs my-backup --follow

# 查看最后 N 行
cloudsync logs my-backup --lines 100
```

#### history 命令

```bash
# 查看所有任务历史
cloudsync history

# 查看指定任务历史
cloudsync history my-backup

# 限制结果数量
cloudsync history my-backup --limit 20
```

## 压缩模式

支持两种压缩模式：

### 单文件模式 (file)

每个文件单独压缩，保留原始目录结构：

```yaml
compression:
  enabled: true
  type: zstd
  mode: file
  level: 3
  min_size: 1024
  exclude_extensions: [".jpg", ".png"]
```

云端路径：`prefix/2024/03/15/143022/documents/file.txt.gz`

### 打包模式 (archive)

所有文件打包成一个压缩包：

```yaml
compression:
  enabled: true
  type: zstd        # tar.zst 格式
  mode: archive
  level: 5
  archive_name: "backup"  # 生成 backup.tar.zst
```

云端路径：`prefix/2024-03-15/backup.tar.zst`

### 模式对比

| 特性 | file 模式 | archive 模式 |
|------|-----------|--------------|
| 文件结构 | 保留原结构 | 打包为单个文件 |
| 格式 | .gz / .zst | .tar.gz / .tar.zst |
| 随机访问 | ✅ 可直接下载单个文件 | ❌ 需下载整个压缩包 |
| 压缩率 | 一般 | 更好（有字典优势） |
| 适用场景 | 需要频繁访问单个文件 | 归档备份、完整恢复 |

## Cron 表达式

使用标准 5 字段 Cron 表达式：

```
┌───────────── 分钟 (0-59)
│ ┌───────────── 小时 (0-23)
│ │ ┌───────────── 日期 (1-31)
│ │ │ ┌───────────── 月份 (1-12)
│ │ │ │ ┌───────────── 星期 (0-6, 0=周日)
│ │ │ │ │
* * * * *
```

### 预设别名

| 别名 | 说明 |
|------|------|
| `@yearly` / `@annually` | 每年 1 月 1 日 0:00 |
| `@monthly` | 每月 1 日 0:00 |
| `@weekly` | 每周日 0:00 |
| `@daily` / `@midnight` | 每天 0:00 |
| `@hourly` | 每小时的第 0 分钟 |

### 示例

```yaml
# 每天凌晨 2 点
schedule: "0 2 * * *"

# 每 6 小时
schedule: "0 */6 * * *"

# 每周一和周五的 3:30
schedule: "30 3 * * 1,5"

# 工作日每小时
schedule: "0 * * * 1-5"

# 使用预设别名
schedule: "@daily"
```

## 目录结构

```
~/.cloudsync/
├── config.yaml          # 主配置文件
├── data/
│   ├── cloudsync.pid   # 守护进程 PID 文件
│   └── state.db        # SQLite 状态数据库
└── logs/
    ├── cloudsync.log   # 主日志文件
    └── tasks/
        ├── my-backup.log
        └── *.log       # 各任务独立日志
```

## 环境变量

配置文件支持环境变量替换：

```yaml
backends:
  - name: my-s3
    type: s3
    access_key: ${AWS_ACCESS_KEY_ID}
    secret_key: ${AWS_SECRET_ACCESS_KEY}
```

支持默认值语法：

```yaml
access_key: ${AWS_ACCESS_KEY_ID:-default_value}
```

## 通知模板变量

| 变量 | 说明 |
|------|------|
| `{{.TaskName}}` | 任务名称 |
| `{{.Status}}` | 执行状态 (success/failed) |
| `{{.FileCount}}` | 处理文件数 |
| `{{.TotalSize}}` | 总大小 |
| `{{.Duration}}` | 耗时 |
| `{{.StartTime}}` | 开始时间 |
| `{{.EndTime}}` | 结束时间 |
| `{{.Error}}` | 错误信息（失败时） |

## 编译

### 本地编译

```bash
go build -o cloudsync ./cmd/cloudsync
```

### 交叉编译

```bash
# Linux AMD64
GOOS=linux GOARCH=amd64 go build -o cloudsync-linux-amd64 ./cmd/cloudsync

# Linux ARM64
GOOS=linux GOARCH=arm64 go build -o cloudsync-linux-arm64 ./cmd/cloudsync

# macOS AMD64
GOOS=darwin GOARCH=amd64 go build -o cloudsync-darwin-amd64 ./cmd/cloudsync

# macOS ARM64
GOOS=darwin GOARCH=arm64 go build -o cloudsync-darwin-arm64 ./cmd/cloudsync

# Windows
GOOS=windows GOARCH=amd64 go build -o cloudsync-windows-amd64.exe ./cmd/cloudsync
```

## 许可证

MIT License
