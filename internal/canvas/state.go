package canvas

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/algebananazzzzz/Corum/internal/config"
)

type CanvasState struct {
	Version  int            `json:"version"`
	SyncedAt *string        `json:"synced_at"`
	Sources  map[string]any `json:"sources"`
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
	Manifest *RunManifest   `json:"-"`
}

type CanvasManifestStage struct {
	Status   string         `json:"status"`
	Changes  []Change       `json:"changes"`
	Failures []Failure      `json:"failures"`
	Sources  []SourceResult `json:"sources"`
}

// RunManifest is the complete per-course machine-readable sync result.
type RunManifest struct {
	Version           int                 `json:"version"`
	RunID             string              `json:"run_id"`
	Course            string              `json:"course"`
	EffectiveFeatures map[string]bool     `json:"effective_features"`
	Canvas            CanvasManifestStage `json:"canvas"`
	Jira              any                 `json:"jira"`
	Wiki              any                 `json:"wiki"`
}

// ManifestOf returns the stage result's complete run manifest, building a
// minimal one when the sync failed before producing its own manifest.
func (result StageResult) ManifestOf(workspace config.Workspace, course config.Course) *RunManifest {
	if result.Manifest != nil {
		return result.Manifest
	}
	manifest := buildManifest("", workspace, course, result)
	return &manifest
}

func canvasPaths(root string, course config.Course, create bool) (string, string, error) {
	if course.Code == "" || filepath.Base(course.Code) != course.Code || course.Code == "." || course.Code == ".." {
		return "", "", fmt.Errorf("invalid course code")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return "", "", err
	}
	absolute, err = filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", "", err
	}
	courseDir, err := filepath.EvalSymlinks(filepath.Join(absolute, "courses", course.Code))
	if err != nil {
		return "", "", fmt.Errorf("resolve course directory: %w", err)
	}
	rel, err := filepath.Rel(filepath.Join(absolute, "courses"), courseDir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", "", fmt.Errorf("course directory resolves outside vault")
	}
	stateDir := filepath.Join(courseDir, "state")
	if info, statErr := os.Lstat(stateDir); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
		return "", "", fmt.Errorf("state directory must not be a symlink")
	} else if statErr != nil && !os.IsNotExist(statErr) {
		return "", "", statErr
	}
	if create {
		if err := os.MkdirAll(stateDir, 0755); err != nil {
			return "", "", err
		}
	}
	return filepath.Join(stateDir, "canvas.json"), filepath.Join(stateDir, "latest-run.json"), nil
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
		return CanvasState{Version: 2, Sources: sources}, nil
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
	if state.Version != 2 {
		return CanvasState{}, fmt.Errorf("canvas state version must be 2")
	}
	if state.Sources == nil {
		state.Sources = map[string]any{}
	}
	return state, nil
}
func atomicJSON(path string, value any) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+"-*")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer os.Remove(name)
	encoder := json.NewEncoder(temporary)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err == nil {
		err = temporary.Sync()
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Rename(name, path); err != nil {
		return err
	}
	directory, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
func writeState(path string, state CanvasState, zone string) error {
	state.Version = 2
	now := time.Now()
	if location, err := time.LoadLocation(zone); err == nil {
		now = now.In(location)
	}
	formatted := now.Format(time.RFC3339)
	state.SyncedAt = &formatted
	return atomicJSON(path, state)
}
func sortedResult(result *StageResult) {
	sort.Slice(result.Changes, func(i, j int) bool { return result.Changes[i].ID < result.Changes[j].ID })
	sort.Slice(result.Failures, func(i, j int) bool { return result.Failures[i].ID < result.Failures[j].ID })
	sort.Slice(result.Sources, func(i, j int) bool { return result.Sources[i].Source < result.Sources[j].Source })
}
