package jira

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/algebananazzzzz/Corum/internal/config"
)

type IssueState struct {
	Key         string   `json:"key"`
	Type        string   `json:"type"`
	Summary     string   `json:"summary"`
	Status      string   `json:"status"`
	Due         *string  `json:"due,omitempty"`
	Labels      []string `json:"labels,omitempty"`
	Description *string  `json:"description,omitempty"`
	UpdatedAt   *string  `json:"updated_at,omitempty"`
}

type JiraState struct {
	ReconciledAt *string      `json:"reconciled_at,omitempty"`
	Issues       []IssueState `json:"issues"`
}

func statePath(root string, course config.Course) (string, error) {
	if !planIdentifierRE.MatchString(course.Code) || filepath.Base(course.Code) != course.Code || course.Code == "." || course.Code == ".." {
		return "", fmt.Errorf("invalid course code")
	}
	rootPath, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	rootPath, err = filepath.EvalSymlinks(rootPath)
	if err != nil {
		return "", fmt.Errorf("resolve vault root: %w", err)
	}
	coursesPath := filepath.Join(rootPath, "courses")
	coursePath := filepath.Join(coursesPath, course.Code)
	resolvedCourse, err := filepath.EvalSymlinks(coursePath)
	if err != nil {
		return "", fmt.Errorf("resolve course directory: %w", err)
	}
	if !pathWithin(coursesPath, resolvedCourse) || resolvedCourse == coursesPath {
		return "", fmt.Errorf("course directory resolves outside vault")
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
	return filepath.Join(stateDir, "jira.json"), nil
}

func pathWithin(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func atomicWriteJSON(target string, value any) error {
	temporary, err := os.CreateTemp(filepath.Dir(target), "."+filepath.Base(target)+"-*")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	clean := true
	defer func() {
		if clean {
			_ = os.Remove(temporaryName)
		}
	}()
	encoder := json.NewEncoder(temporary)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
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
	if err := os.Rename(temporaryName, target); err != nil {
		return err
	}
	clean = false
	directory, err := os.Open(filepath.Dir(target))
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

func writeJiraState(path string, state JiraState) error {
	if state.Issues == nil {
		state.Issues = []IssueState{}
	}
	sort.Slice(state.Issues, func(i, j int) bool { return issueKeyLess(state.Issues[i].Key, state.Issues[j].Key) })
	return atomicWriteJSON(path, state)
}

func issueKeyLess(a, b string) bool {
	separatorA := strings.LastIndexByte(a, '-')
	separatorB := strings.LastIndexByte(b, '-')
	if separatorA < 0 || separatorB < 0 || a[:separatorA] != b[:separatorB] {
		return a < b
	}
	numberA, _ := strconv.Atoi(a[separatorA+1:])
	numberB, _ := strconv.Atoi(b[separatorB+1:])
	return numberA < numberB
}
