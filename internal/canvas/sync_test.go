package canvas

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/algebananazzzzz/Corum/internal/config"
)

func initializedCanvasVault(t *testing.T) (string, config.Workspace, config.Course) {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "courses", "CS3103", "raw"), 0755); err != nil {
		t.Fatal(err)
	}
	workspace := config.Workspace{Version: 2, Workspace: config.WorkspaceDetails{Timezone: "Asia/Singapore", Term: "T"}, Canvas: &config.CanvasWorkspace{URL: "https://canvas.example.edu"}, Calendar: config.Calendar{Timetable: "Timetable.md", Term: "Term.md"}}
	course := config.Course{Version: 2, Code: "CS3103", Canvas: &config.CanvasCourse{ID: 1, Sources: []string{"announcements"}, Folders: map[string]string{}}}
	return root, workspace, course
}

func TestDryRunDoesNotWriteFiles(t *testing.T) {
	root, workspace, course := initializedCanvasVault(t)
	before := snapshotTree(t, root)
	result, err := Sync(context.Background(), root, workspace, course, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "pending" {
		t.Fatalf("status = %q", result.Status)
	}
	after := snapshotTree(t, root)
	if before != after {
		t.Fatalf("dry run wrote files: before=%q after=%q", before, after)
	}
}

func TestDisabledSyncNeedsNoClient(t *testing.T) {
	root, workspace, course := initializedCanvasVault(t)
	workspace.Canvas = nil
	result, err := Sync(context.Background(), root, workspace, course, nil, false)
	if err != nil || result.Status != "disabled" {
		t.Fatalf("Sync() = %#v, %v", result, err)
	}
}

func snapshotTree(t *testing.T, root string) string {
	t.Helper()
	entries := []string{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			entries = append(entries, path+":"+string(data))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return stringJoin(entries, "\n")
}
func stringJoin(values []string, separator string) string {
	result := ""
	for i, value := range values {
		if i > 0 {
			result += separator
		}
		result += value
	}
	return result
}
