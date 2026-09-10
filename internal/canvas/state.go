package canvas

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/algebananazzzzz/Corum/internal/config"
)

type CanvasState struct {
	Sources map[string]any `json:"sources"`
}
type Change struct {
	ID      string         `json:"id"`
	Source  string         `json:"source"`
	ItemID  *string        `json:"item_id,omitempty"`
	Kind    string         `json:"kind"`
	Status  string         `json:"status"`
	Summary string         `json:"summary"`
	RawPath *string        `json:"raw_path,omitempty"`
	Details map[string]any `json:"details,omitempty"`
}
type Failure struct {
	ID      string         `json:"id"`
	Source  string         `json:"source"`
	ItemID  *string        `json:"item_id,omitempty"`
	Kind    string         `json:"kind"`
	Status  string         `json:"status"`
	Error   string         `json:"error"`
	RawPath *string        `json:"raw_path,omitempty"`
	Details map[string]any `json:"details,omitempty"`
}
type SourceResult struct {
	Source   string   `json:"source"`
	Status   string   `json:"status"`
	Changes  []string `json:"changes"`
	Failures []string `json:"failures"`
}
type StageResult struct {
	Course   string         `json:"course"`
	DryRun   bool           `json:"dry_run"`
	Status   string         `json:"status"`
	Changes  []Change       `json:"changes"`
	Failures []Failure      `json:"failures"`
	Sources  []SourceResult `json:"sources"`
}

func canvasPath(root string, course config.Course, create bool) (string, error) {
	if course.Code == "" || filepath.Base(course.Code) != course.Code || course.Code == "." || course.Code == ".." {
		return "", fmt.Errorf("invalid course code")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	absolute, err = filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", err
	}
	courseDir, err := filepath.EvalSymlinks(filepath.Join(absolute, "courses", course.Code))
	if err != nil {
		return "", fmt.Errorf("resolve course directory: %w", err)
	}
	rel, err := filepath.Rel(filepath.Join(absolute, "courses"), courseDir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("course directory resolves outside vault")
	}
	stateDir := filepath.Join(courseDir, "state")
	if info, statErr := os.Lstat(stateDir); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("state directory must not be a symlink")
	} else if statErr != nil && !os.IsNotExist(statErr) {
		return "", statErr
	}
	if create {
		if err := os.MkdirAll(stateDir, 0755); err != nil {
			return "", err
		}
	}
	return filepath.Join(stateDir, "canvas.json"), nil
}
func readState(path string, selected []string) (CanvasState, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		sources := map[string]any{}
		for _, source := range selected {
			if source == "syllabus" {
				sources[source] = nil
			} else {
				sources[source] = map[string]any{}
			}
		}
		return CanvasState{Sources: sources}, nil
	}
	if err != nil {
		return CanvasState{}, err
	}
	var state CanvasState
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&state); err != nil {
		return CanvasState{}, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return CanvasState{}, fmt.Errorf("canvas state has trailing JSON")
		}
		return CanvasState{}, err
	}
	if state.Sources == nil {
		state.Sources = map[string]any{}
	}
	return state, nil
}
func atomicJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteFile(path, append(data, '\n'), 0o600, ".canvas-write-*")
}
func sortedResult(result *StageResult) {
	sort.Slice(result.Changes, func(i, j int) bool { return result.Changes[i].ID < result.Changes[j].ID })
	sort.Slice(result.Failures, func(i, j int) bool { return result.Failures[i].ID < result.Failures[j].ID })
	sort.Slice(result.Sources, func(i, j int) bool { return result.Sources[i].Source < result.Sources[j].Source })
}
