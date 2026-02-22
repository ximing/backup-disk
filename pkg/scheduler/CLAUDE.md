# Scheduler Package

This package provides task scheduling functionality using cron expressions.

## TaskScheduler

The `TaskScheduler` manages scheduled tasks using `robfig/cron/v3`:

```go
scheduler := scheduler.NewTaskScheduler(handler)
scheduler.Start()
```

## Cron Expression Support

Standard 5-field cron expressions are supported:
```
* * * * *
│ │ │ │ │
│ │ │ │ └─── Day of week (0-6, SUN-SAT)
│ │ │ └───── Month (1-12, JAN-DEC)
│ │ └─────── Day of month (1-31, L)
│ └───────── Hour (0-23)
└─────────── Minute (0-59)
```

### Preset Aliases

- `@yearly` / `@annually` - Once a year at midnight on Jan 1st
- `@monthly` - Once a month at midnight on the 1st
- `@weekly` - Once a week at midnight on Sunday
- `@daily` / `@midnight` - Once a day at midnight
- `@hourly` - At the beginning of every hour

### Common Examples

```yaml
schedule: "0 2 * * *"      # Every day at 2:00 AM
schedule: "0 */6 * * *"    # Every 6 hours
schedule: "0 0 * * 0"      # Every Sunday at midnight
schedule: "@daily"          # Daily at midnight (preset)
schedule: "0 0 1 * *"      # First day of every month
```

## TaskConfig

The scheduler defines its own `TaskConfig` struct to avoid import cycles with the config package. When using the scheduler, convert from `config.TaskConfig`:

```go
taskCfg := scheduler.TaskConfig{
    Name:     cfg.Name,
    Schedule: cfg.Schedule,
    Source:   scheduler.SourceConfig{Path: cfg.Source.Path, ...},
    // ... other fields
}
scheduler.AddTask(taskCfg)
```

## TaskHandler

Define a handler function that will be called when a task is triggered:

```go
handler := func(ctx context.Context, task scheduler.TaskConfig) error {
    // Execute the task
    return nil
}
scheduler := scheduler.NewTaskScheduler(handler)
```

## Utility Functions

- `ValidateSchedule(schedule string)` - Validate a cron expression
- `GetNextRuns(schedule string, n int)` - Get next N scheduled times
- `FormatDuration(d time.Duration)` - Format duration in human-readable form
- `DescribeSchedule(schedule string)` - Get human-readable description

## Important Notes

- The scheduler must be started with `Start()` before tasks will execute
- Call `Stop()` to gracefully stop the scheduler
- The scheduler uses the cron parser with `SecondOptional` flag, allowing both 5 and 6 field expressions
