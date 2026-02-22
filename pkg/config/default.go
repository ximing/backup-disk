package config

// DefaultConfig is the default configuration template
const DefaultConfig = `# CloudSync 配置文件
# 文档: https://github.com/ximing/cloudsync

# 全局配置
global:
  # 日志级别: debug, info, warn, error
  log_level: info

  # 日志格式: text, json
  log_format: text

# 存储后端配置
storage:
  # 存储类型: s3 或 oss
  type: s3

  # AWS S3 配置 (当 type=s3 时使用)
  s3:
    region: us-east-1
    bucket: my-backup-bucket
    # 支持环境变量语法: ${AWS_ACCESS_KEY_ID}
    access_key: ${AWS_ACCESS_KEY_ID}
    secret_key: ${AWS_SECRET_ACCESS_KEY}
    # 可选: S3 兼容服务的服务端点
    # endpoint: https://s3.amazonaws.com
    # 可选: 服务器端加密方式
    # encryption: AES256

  # 阿里云 OSS 配置 (当 type=oss 时使用)
  oss:
    endpoint: oss-cn-hangzhou.aliyuncs.com
    bucket: my-backup-bucket
    # 支持环境变量语法
    access_key_id: ${OSS_ACCESS_KEY_ID}
    access_key_secret: ${OSS_ACCESS_KEY_SECRET}

# 同步任务配置
tasks:
  # 任务名称 (必需，唯一)
  - name: documents-backup

    # Cron 表达式 (必需)
    # 支持标准 5 字段格式: 分 时 日 月 周
    # 或预设别名: @hourly, @daily, @weekly, @monthly
    schedule: "0 2 * * *"  # 每天凌晨 2 点

    # 源文件配置
    source:
      # 本地路径 (必需)
      path: ~/Documents

      # 包含模式 (可选, glob 语法)
      include:
        - "**/*.pdf"
        - "**/*.docx"

      # 排除模式 (可选, glob 语法)
      exclude:
        - "**/temp/**"
        - "**/.DS_Store"

    # 目标配置
    target:
      # 存储前缀 (必需)
      prefix: backups/documents

      # 日期格式 (可选, 默认: 2006/01/02/150405)
      # 使用 Go 时间格式: https://pkg.go.dev/time#pkg-constants
      date_format: "2006/01/02/150405"

    # 压缩配置 (可选)
    compression:
      # 是否启用压缩
      enabled: true

      # 压缩类型: gzip 或 zstd
      type: gzip

      # 压缩级别: 1-9 (gzip) 或 1-22 (zstd)
      level: 6

      # 最小压缩文件大小 (字节)
      # 小于此大小的文件不压缩
      min_size: 1024

      # 只压缩这些扩展名 (可选)
      # include_extensions:
      #   - ".txt"
      #   - ".log"

      # 排除这些扩展名 (可选)
      # exclude_extensions:
      #   - ".zip"
      #   - ".gz"

    # 任务级保留策略 (可选, 覆盖全局配置)
    retention:
      # 最大保留天数
      max_days: 30
      # 最大保留版本数
      max_versions: 10

    # 任务级通知设置 (可选, 覆盖全局配置)
    notify:
      # 通知级别: always, on-error, never
      enabled: on-error

  # 可以配置多个任务
  - name: photos-backup
    schedule: "0 3 * * 0"  # 每周日凌晨 3 点
    source:
      path: ~/Pictures
      include:
        - "**/*.jpg"
        - "**/*.png"
    target:
      prefix: backups/photos
    compression:
      enabled: false

# 全局保留策略 (适用于所有任务，可被任务级配置覆盖)
retention:
  # 最大保留天数 (0 表示不限制)
  max_days: 30

  # 最大保留版本数 (0 表示不限制)
  max_versions: 10

# MeoW 推送通知配置
notify:
  # 是否启用通知
  enabled: true

  # MeoW API 地址
  api_url: https://api.meow.example.com/push

  # 消息类型
  msg_type: html

  # HTML 消息高度
  html_height: 350
`
