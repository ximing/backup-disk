// Package state provides SQLite-based task execution state persistence
package state

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// ExecutionStatus represents the status of a task execution
type ExecutionStatus string

const (
	StatusRunning  ExecutionStatus = "running"
	StatusSuccess  ExecutionStatus = "success"
	StatusFailed   ExecutionStatus = "failed"
	StatusCanceled ExecutionStatus = "canceled"
)

// ExecutionRecord represents a single task execution record
type ExecutionRecord struct {
	ID           int64           `json:"id"`
	TaskName     string          `json:"task_name"`
	Status       ExecutionStatus `json:"status"`
	StartTime    time.Time       `json:"start_time"`
	EndTime      *time.Time      `json:"end_time,omitempty"`
	Duration     *int64          `json:"duration_ms,omitempty"`
	FilesTotal   int             `json:"files_total"`
	FilesSuccess int             `json:"files_success"`
	FilesFailed  int             `json:"files_failed"`
	FilesSkipped int             `json:"files_skipped"`
	BytesTotal   int64           `json:"bytes_total"`
	BytesSuccess int64           `json:"bytes_success"`
	Error        string          `json:"error,omitempty"`
}

// TaskStatus represents the current status of a task
type TaskStatus struct {
	TaskName      string           `json:"task_name"`
	LastRun       *time.Time       `json:"last_run,omitempty"`
	LastStatus    *ExecutionStatus `json:"last_status,omitempty"`
	NextSchedule  *time.Time       `json:"next_schedule,omitempty"`
	TotalRuns     int              `json:"total_runs"`
	SuccessRuns   int              `json:"success_runs"`
	FailedRuns    int              `json:"failed_runs"`
}

// Store provides SQLite state storage
type Store struct {
	db *sql.DB
}

// New creates a new state store
func New(dbPath string) (*Store, error) {
	// Ensure directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create state directory: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open state database: %w", err)
	}

	store := &Store{db: db}
	if err := store.init(); err != nil {
		db.Close()
		return nil, err
	}

	return store, nil
}

// Close closes the state store
func (s *Store) Close() error {
	return s.db.Close()
}

// init initializes the database schema
func (s *Store) init() error {
	schema := `
	CREATE TABLE IF NOT EXISTS executions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		task_name TEXT NOT NULL,
		status TEXT NOT NULL,
		start_time DATETIME NOT NULL,
		end_time DATETIME,
		duration_ms INTEGER,
		files_total INTEGER DEFAULT 0,
		files_success INTEGER DEFAULT 0,
		files_failed INTEGER DEFAULT 0,
		files_skipped INTEGER DEFAULT 0,
		bytes_total INTEGER DEFAULT 0,
		bytes_success INTEGER DEFAULT 0,
		error TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_executions_task_name ON executions(task_name);
	CREATE INDEX IF NOT EXISTS idx_executions_start_time ON executions(start_time);
	CREATE INDEX IF NOT EXISTS idx_executions_status ON executions(status);
	`

	_, err := s.db.Exec(schema)
	if err != nil {
		return fmt.Errorf("failed to initialize database schema: %w", err)
	}

	return nil
}

// StartExecution records the start of a task execution
func (s *Store) StartExecution(taskName string) (int64, error) {
	result, err := s.db.Exec(
		"INSERT INTO executions (task_name, status, start_time) VALUES (?, ?, ?)",
		taskName, StatusRunning, time.Now(),
	)
	if err != nil {
		return 0, fmt.Errorf("failed to record execution start: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("failed to get last insert id: %w", err)
	}

	return id, nil
}

// CompleteExecution marks an execution as completed
func (s *Store) CompleteExecution(id int64, status ExecutionStatus, filesTotal, filesSuccess, filesFailed, filesSkipped int, bytesTotal, bytesSuccess int64, errorMsg string) error {
	endTime := time.Now()

	// Get start time to calculate duration
	var startTime time.Time
	err := s.db.QueryRow("SELECT start_time FROM executions WHERE id = ?", id).Scan(&startTime)
	if err != nil {
		return fmt.Errorf("failed to get start time: %w", err)
	}

	duration := endTime.Sub(startTime).Milliseconds()

	_, err = s.db.Exec(
		`UPDATE executions SET
			status = ?, end_time = ?, duration_ms = ?,
			files_total = ?, files_success = ?, files_failed = ?, files_skipped = ?,
			bytes_total = ?, bytes_success = ?, error = ?
		WHERE id = ?`,
		status, endTime, duration,
		filesTotal, filesSuccess, filesFailed, filesSkipped,
		bytesTotal, bytesSuccess, errorMsg, id,
	)
	if err != nil {
		return fmt.Errorf("failed to complete execution: %w", err)
	}

	return nil
}

// GetExecution retrieves a specific execution record
func (s *Store) GetExecution(id int64) (*ExecutionRecord, error) {
	row := s.db.QueryRow(
		`SELECT id, task_name, status, start_time, end_time, duration_ms,
			files_total, files_success, files_failed, files_skipped,
			bytes_total, bytes_success, error
		FROM executions WHERE id = ?`,
		id,
	)

	return scanExecution(row)
}

// GetTaskHistory retrieves execution history for a specific task
func (s *Store) GetTaskHistory(taskName string, limit int) ([]*ExecutionRecord, error) {
	rows, err := s.db.Query(
		`SELECT id, task_name, status, start_time, end_time, duration_ms,
			files_total, files_success, files_failed, files_skipped,
			bytes_total, bytes_success, error
		FROM executions WHERE task_name = ?
		ORDER BY start_time DESC LIMIT ?`,
		taskName, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query task history: %w", err)
	}
	defer rows.Close()

	return scanExecutions(rows)
}

// GetAllHistory retrieves execution history for all tasks
func (s *Store) GetAllHistory(limit int) ([]*ExecutionRecord, error) {
	rows, err := s.db.Query(
		`SELECT id, task_name, status, start_time, end_time, duration_ms,
			files_total, files_success, files_failed, files_skipped,
			bytes_total, bytes_success, error
		FROM executions
		ORDER BY start_time DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query history: %w", err)
	}
	defer rows.Close()

	return scanExecutions(rows)
}

// GetTaskStatus retrieves the current status summary for a task
func (s *Store) GetTaskStatus(taskName string) (*TaskStatus, error) {
	status := &TaskStatus{TaskName: taskName}

	// Get last run info
	var lastStatusStr string
	var lastRun sql.NullTime
	err := s.db.QueryRow(
		`SELECT status, start_time FROM executions
		WHERE task_name = ?
		ORDER BY start_time DESC LIMIT 1`,
		taskName,
	).Scan(&lastStatusStr, &lastRun)

	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to query last run: %w", err)
	}

	if lastRun.Valid {
		status.LastRun = &lastRun.Time
		lastStatus := ExecutionStatus(lastStatusStr)
		status.LastStatus = &lastStatus
	}

	// Get statistics
	err = s.db.QueryRow(
		`SELECT
			COUNT(*) as total,
			SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END) as success,
			SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END) as failed
		FROM executions WHERE task_name = ?`,
		taskName,
	).Scan(&status.TotalRuns, &status.SuccessRuns, &status.FailedRuns)

	if err != nil {
		return nil, fmt.Errorf("failed to query statistics: %w", err)
	}

	return status, nil
}

// GetAllTaskStatus retrieves status for all tasks
func (s *Store) GetAllTaskStatus() ([]*TaskStatus, error) {
	rows, err := s.db.Query(
		`SELECT task_name FROM executions GROUP BY task_name ORDER BY task_name`,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query tasks: %w", err)
	}
	defer rows.Close()

	var taskNames []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		taskNames = append(taskNames, name)
	}

	var statuses []*TaskStatus
	for _, name := range taskNames {
		status, err := s.GetTaskStatus(name)
		if err != nil {
			return nil, err
		}
		statuses = append(statuses, status)
	}

	return statuses, nil
}

// GetRunningExecutions retrieves all currently running executions
func (s *Store) GetRunningExecutions() ([]*ExecutionRecord, error) {
	rows, err := s.db.Query(
		`SELECT id, task_name, status, start_time, end_time, duration_ms,
			files_total, files_success, files_failed, files_skipped,
			bytes_total, bytes_success, error
		FROM executions WHERE status = ?
		ORDER BY start_time DESC`,
		StatusRunning,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query running executions: %w", err)
	}
	defer rows.Close()

	return scanExecutions(rows)
}

// CleanupOldRecords removes records older than the specified duration
func (s *Store) CleanupOldRecords(olderThan time.Duration) (int64, error) {
	cutoff := time.Now().Add(-olderThan)
	result, err := s.db.Exec(
		"DELETE FROM executions WHERE start_time < ?",
		cutoff,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup old records: %w", err)
	}

	return result.RowsAffected()
}

// scanExecution scans a single execution from a row
func scanExecution(row *sql.Row) (*ExecutionRecord, error) {
	var rec ExecutionRecord
	var endTime sql.NullTime
	var duration sql.NullInt64
	var errorMsg sql.NullString

	err := row.Scan(
		&rec.ID, &rec.TaskName, &rec.Status, &rec.StartTime, &endTime, &duration,
		&rec.FilesTotal, &rec.FilesSuccess, &rec.FilesFailed, &rec.FilesSkipped,
		&rec.BytesTotal, &rec.BytesSuccess, &errorMsg,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if endTime.Valid {
		rec.EndTime = &endTime.Time
	}
	if duration.Valid {
		rec.Duration = &duration.Int64
	}
	if errorMsg.Valid {
		rec.Error = errorMsg.String
	}

	return &rec, nil
}

// scanExecutions scans multiple executions from rows
func scanExecutions(rows *sql.Rows) ([]*ExecutionRecord, error) {
	var records []*ExecutionRecord

	for rows.Next() {
		var rec ExecutionRecord
		var endTime sql.NullTime
		var duration sql.NullInt64
		var errorMsg sql.NullString

		err := rows.Scan(
			&rec.ID, &rec.TaskName, &rec.Status, &rec.StartTime, &endTime, &duration,
			&rec.FilesTotal, &rec.FilesSuccess, &rec.FilesFailed, &rec.FilesSkipped,
			&rec.BytesTotal, &rec.BytesSuccess, &errorMsg,
		)
		if err != nil {
			return nil, err
		}

		if endTime.Valid {
			rec.EndTime = &endTime.Time
		}
		if duration.Valid {
			rec.Duration = &duration.Int64
		}
		if errorMsg.Valid {
			rec.Error = errorMsg.String
		}

		records = append(records, &rec)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return records, nil
}

// GetDBPath returns the default path for the state database
func GetDBPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".cloudsync/data/state.db"
	}
	return filepath.Join(home, ".cloudsync", "data", "state.db")
}
