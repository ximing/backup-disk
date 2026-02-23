// Package scheduler provides task scheduling functionality using cron expressions
package scheduler

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

// TaskConfig defines the interface for task configuration
// This avoids importing the config package and creating a circular dependency
type TaskConfig struct {
	Name        string
	Schedule    string
	Source      SourceConfig
	Target      TargetConfig
	Compression CompressionConfig
	Retention   *RetentionPolicy
	Notify      *NotifySettings
}

// SourceConfig represents source file configuration
type SourceConfig struct {
	Path    string
	Include []string
	Exclude []string
}

// TargetConfig represents target storage configuration
type TargetConfig struct {
	Prefix     string
	DateFormat string
}

// CompressionConfig represents compression settings
type CompressionConfig struct {
	Enabled           bool
	Type              string
	Mode              string
	Level             int
	MinSize           int64
	IncludeExtensions []string
	ExcludeExtensions []string
	ArchiveName       string
}

// RetentionPolicy represents retention settings
type RetentionPolicy struct {
	MaxDays     int
	MaxVersions int
}

// NotifySettings represents notification settings
type NotifySettings struct {
	Enabled string
}

// Task represents a scheduled sync task
type Task struct {
	Config   TaskConfig
	Schedule cron.Schedule
	EntryID  cron.EntryID
}

// NextRun returns the next scheduled run time for the task
func (t *Task) NextRun() time.Time {
	return t.Schedule.Next(time.Now())
}

// TaskHandler is the function type for task execution
type TaskHandler func(ctx context.Context, task TaskConfig) error

// TaskScheduler manages scheduled tasks
type TaskScheduler struct {
	cron    *cron.Cron
	parser  cron.Parser
	tasks   map[string]*Task
	mu      sync.RWMutex
	handler TaskHandler
}

// NewTaskScheduler creates a new task scheduler
func NewTaskScheduler(handler TaskHandler) *TaskScheduler {
	// Create cron with standard parser that supports both standard cron and presets
	parser := cron.NewParser(
		cron.SecondOptional | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
	)

	return &TaskScheduler{
		cron:    cron.New(cron.WithParser(parser)),
		parser:  parser,
		tasks:   make(map[string]*Task),
		handler: handler,
	}
}

// AddTask adds a single task to the scheduler
func (s *TaskScheduler) AddTask(taskCfg TaskConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.addTaskLocked(taskCfg)
}

// addTaskLocked adds a task (must be called with lock held)
func (s *TaskScheduler) addTaskLocked(taskCfg TaskConfig) error {
	// Check for duplicate task names
	if _, exists := s.tasks[taskCfg.Name]; exists {
		return fmt.Errorf("task %s already exists", taskCfg.Name)
	}

	// Parse the schedule expression
	schedule, err := s.parser.Parse(taskCfg.Schedule)
	if err != nil {
		return fmt.Errorf("invalid schedule expression %q: %w", taskCfg.Schedule, err)
	}

	task := &Task{
		Config:   taskCfg,
		Schedule: schedule,
	}

	// Create a wrapper function that captures the task config
	wrapper := func() {
		ctx := context.Background()
		if s.handler != nil {
			_ = s.handler(ctx, taskCfg)
		}
	}

	// Schedule the task
	entryID := s.cron.Schedule(schedule, cron.FuncJob(wrapper))
	task.EntryID = entryID

	s.tasks[taskCfg.Name] = task
	return nil
}

// RemoveTask removes a task from the scheduler
func (s *TaskScheduler) RemoveTask(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, exists := s.tasks[name]
	if !exists {
		return fmt.Errorf("task %s not found", name)
	}

	s.cron.Remove(task.EntryID)
	delete(s.tasks, name)
	return nil
}

// GetTask returns a task by name
func (s *TaskScheduler) GetTask(name string) (*Task, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	task, exists := s.tasks[name]
	return task, exists
}

// GetAllTasks returns all scheduled tasks
func (s *TaskScheduler) GetAllTasks() []*Task {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tasks := make([]*Task, 0, len(s.tasks))
	for _, task := range s.tasks {
		tasks = append(tasks, task)
	}
	return tasks
}

// Start starts the scheduler
func (s *TaskScheduler) Start() {
	s.cron.Start()
}

// Stop stops the scheduler
func (s *TaskScheduler) Stop() {
	s.cron.Stop()
}

// ValidateSchedule validates a cron schedule expression
func ValidateSchedule(schedule string) error {
	parser := cron.NewParser(
		cron.SecondOptional | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
	)
	_, err := parser.Parse(schedule)
	if err != nil {
		return fmt.Errorf("invalid schedule: %w", err)
	}
	return nil
}

// GetNextRuns returns the next N scheduled run times for a schedule expression
func GetNextRuns(schedule string, n int) ([]time.Time, error) {
	parser := cron.NewParser(
		cron.SecondOptional | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
	)

	s, err := parser.Parse(schedule)
	if err != nil {
		return nil, fmt.Errorf("invalid schedule: %w", err)
	}

	var times []time.Time
	t := time.Now()
	for i := 0; i < n; i++ {
		t = s.Next(t)
		times = append(times, t)
	}
	return times, nil
}

// FormatDuration formats a duration in a human-readable way
func FormatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	if hours == 0 {
		return fmt.Sprintf("%dd", days)
	}
	return fmt.Sprintf("%dd%dh", days, hours)
}
