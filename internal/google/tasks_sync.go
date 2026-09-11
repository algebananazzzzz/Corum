package google

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/algebananazzzzz/Corum/internal/config"
)

type TaskState struct {
	ReconciledAt *string         `json:"reconciled_at,omitempty"`
	Tasks        []TaskStateItem `json:"tasks"`
}

type TaskStateItem struct {
	ID          string  `json:"id"`
	Title       string  `json:"title"`
	Status      string  `json:"status"`
	Due         *string `json:"due,omitempty"`
	Notes       *string `json:"notes,omitempty"`
	Parent      *string `json:"parent,omitempty"`
	CompletedAt *string `json:"completed_at,omitempty"`
	UpdatedAt   *string `json:"updated_at,omitempty"`
	WebViewLink *string `json:"web_view_link,omitempty"`
}

type SyncClient interface {
	ListTasks(context.Context, string) ([]map[string]any, error)
}

type SyncOptions struct {
	Client SyncClient
	Now    func() time.Time
}

type SyncResult struct {
	Course    string `json:"course"`
	ListID    string `json:"list_id"`
	TaskCount int    `json:"task_count"`
	CachePath string `json:"cache_path"`
}

func SyncTasks(ctx context.Context, root, courseCode, listID string, options SyncOptions) (SyncResult, error) {
	if options.Client == nil {
		return SyncResult{}, fmt.Errorf("Google Tasks sync client is required")
	}
	if strings.TrimSpace(listID) == "" {
		return SyncResult{}, fmt.Errorf("Google Tasks list ID is required")
	}
	cachePath, err := taskStatePath(root, courseCode)
	if err != nil {
		return SyncResult{}, err
	}
	raw, err := options.Client.ListTasks(ctx, listID)
	if err != nil {
		return SyncResult{}, fmt.Errorf("could not fetch Google Tasks: %w", err)
	}
	tasks, err := normalizeTasks(raw)
	if err != nil {
		return SyncResult{}, err
	}
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].ID < tasks[j].ID })
	now := time.Now
	if options.Now != nil {
		now = options.Now
	}
	reconciledAt := now().Format(time.RFC3339Nano)
	if err := writeTaskState(cachePath, TaskState{ReconciledAt: &reconciledAt, Tasks: tasks}); err != nil {
		return SyncResult{}, fmt.Errorf("could not write Google Tasks state: %w", err)
	}
	return SyncResult{Course: courseCode, ListID: listID, TaskCount: len(tasks), CachePath: cachePath}, nil
}

func normalizeTasks(raw []map[string]any) ([]TaskStateItem, error) {
	if raw == nil {
		return nil, fmt.Errorf("Google Tasks response is invalid")
	}
	seen := map[string]bool{}
	result := make([]TaskStateItem, 0, len(raw))
	for index, value := range raw {
		id, ok := value["id"].(string)
		if !ok || strings.TrimSpace(id) == "" {
			return nil, fmt.Errorf("Google Tasks item %d is missing an ID", index)
		}
		if seen[id] {
			return nil, fmt.Errorf("Google Tasks contains a duplicate ID")
		}
		seen[id] = true
		title, ok := value["title"].(string)
		if !ok || strings.TrimSpace(title) == "" {
			return nil, fmt.Errorf("Google Tasks item %d has an invalid title", index)
		}
		status, ok := value["status"].(string)
		if !ok || (status != "needsAction" && status != "completed") {
			return nil, fmt.Errorf("Google Tasks item %d has an invalid status", index)
		}
		item := TaskStateItem{ID: id, Title: title, Status: status}
		if err := optionalDate(value, "due", &item.Due); err != nil {
			return nil, fmt.Errorf("Google Tasks item %d: %w", index, err)
		}
		if err := optionalText(value, "notes", &item.Notes); err != nil {
			return nil, fmt.Errorf("Google Tasks item %d: %w", index, err)
		}
		if err := optionalText(value, "parent", &item.Parent); err != nil {
			return nil, fmt.Errorf("Google Tasks item %d: %w", index, err)
		}
		if err := optionalTimestamp(value, "completed", &item.CompletedAt); err != nil {
			return nil, fmt.Errorf("Google Tasks item %d: %w", index, err)
		}
		if err := optionalTimestamp(value, "updated", &item.UpdatedAt); err != nil {
			return nil, fmt.Errorf("Google Tasks item %d: %w", index, err)
		}
		if err := optionalText(value, "webViewLink", &item.WebViewLink); err != nil {
			return nil, fmt.Errorf("Google Tasks item %d: %w", index, err)
		}
		result = append(result, item)
	}
	return result, nil
}

func optionalText(value map[string]any, key string, target **string) error {
	raw, exists := value[key]
	if !exists || raw == nil {
		return nil
	}
	text, ok := raw.(string)
	if !ok {
		return fmt.Errorf("%s must be a string", key)
	}
	if strings.TrimSpace(text) != "" {
		*target = &text
	}
	return nil
}

func optionalDate(value map[string]any, key string, target **string) error {
	raw, exists := value[key]
	if !exists || raw == nil {
		return nil
	}
	text, ok := raw.(string)
	if !ok {
		return fmt.Errorf("%s must be a timestamp", key)
	}
	parsed, err := time.Parse(time.RFC3339Nano, text)
	if err != nil {
		return fmt.Errorf("%s is invalid", key)
	}
	date := parsed.Format("2006-01-02")
	*target = &date
	return nil
}

func optionalTimestamp(value map[string]any, key string, target **string) error {
	raw, exists := value[key]
	if !exists || raw == nil {
		return nil
	}
	text, ok := raw.(string)
	if !ok {
		return fmt.Errorf("%s must be a timestamp", key)
	}
	parsed, err := time.Parse(time.RFC3339Nano, text)
	if err != nil {
		return fmt.Errorf("%s is invalid", key)
	}
	normalized := parsed.Format(time.RFC3339Nano)
	*target = &normalized
	return nil
}

func taskStatePath(root, courseCode string) (string, error) {
	if err := config.ValidateCourse(config.Course{Code: courseCode}); err != nil {
		return "", err
	}
	rootPath, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	rootPath, err = filepath.EvalSymlinks(rootPath)
	if err != nil {
		return "", fmt.Errorf("resolve vault root: %w", err)
	}
	coursePath := filepath.Join(rootPath, "courses", courseCode)
	resolvedCourse, err := filepath.EvalSymlinks(coursePath)
	if err != nil {
		return "", fmt.Errorf("resolve course directory: %w", err)
	}
	if resolvedCourse != coursePath {
		return "", fmt.Errorf("course directory must not be a symlink")
	}
	info, err := os.Stat(resolvedCourse)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("invalid course directory")
	}
	stateDir := filepath.Join(resolvedCourse, "state")
	if info, err := os.Lstat(stateDir); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("state directory must not be a symlink")
	} else if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(stateDir, "google-tasks.json"), nil
}

func writeTaskState(path string, state TaskState) error {
	if state.Tasks == nil {
		state.Tasks = []TaskStateItem{}
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".google-tasks-*")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	encoder := json.NewEncoder(temporary)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(state); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryName, path)
}
