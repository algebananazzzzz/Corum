package canvas

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/algebananazzzzz/Corum/internal/config"
	"github.com/algebananazzzzz/Corum/internal/lockfile"
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

func TestAtomicJSONPreservesStateOnEncodingFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	before := []byte("{\"version\":2}\n")
	if err := os.WriteFile(path, before, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := atomicJSON(path, make(chan int)); err == nil {
		t.Fatal("atomicJSON accepted an unsupported JSON value")
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != string(before) {
		t.Fatalf("state = %q, %v; want %q", got, err, before)
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

type syncClient struct {
	gets      map[string]map[string]any
	lists     map[string][]map[string]any
	errors    map[string]error
	downloads []string
}

func (c *syncClient) Get(_ context.Context, endpoint string, _ url.Values) (map[string]any, error) {
	if err := c.errors["get:"+endpoint]; err != nil {
		return nil, err
	}
	value, ok := c.gets[endpoint]
	if !ok {
		return nil, fmt.Errorf("missing GET fixture for %s", endpoint)
	}
	return value, nil
}

func (c *syncClient) GetAll(_ context.Context, endpoint string, _ url.Values) ([]map[string]any, error) {
	if err := c.errors["list:"+endpoint]; err != nil {
		return nil, err
	}
	value, ok := c.lists[endpoint]
	if !ok {
		return nil, fmt.Errorf("missing list fixture for %s", endpoint)
	}
	return value, nil
}

func (c *syncClient) Download(_ context.Context, _ string, target string) error {
	c.downloads = append(c.downloads, target)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	return os.WriteFile(target, []byte("download"), 0o644)
}

func TestSyncRebuildsModulesFromChangedAndUnchangedModules(t *testing.T) {
	root, workspace, course := initializedCanvasVault(t)
	course.Canvas.Sources = []string{"modules"}
	client := &syncClient{lists: map[string][]map[string]any{
		"/api/v1/courses/1/modules": {
			{"id": float64(1), "name": "Week One", "position": float64(1)},
			{"id": float64(2), "name": "Week Two", "position": float64(2)},
		},
		"/api/v1/courses/1/modules/1/items": {{"id": float64(11), "position": float64(1), "type": "Page", "title": "Intro"}},
		"/api/v1/courses/1/modules/2/items": {{"id": float64(21), "position": float64(1), "type": "Page", "title": "Original"}},
	}}
	if _, err := Sync(context.Background(), root, workspace, course, client, false); err != nil {
		t.Fatal(err)
	}
	client.lists["/api/v1/courses/1/modules/2/items"][0]["title"] = "Changed"
	result, err := Sync(context.Background(), root, workspace, course, client, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Changes) != 1 || result.Changes[0].ItemID == nil || *result.Changes[0].ItemID != "2" {
		t.Fatalf("changes = %#v", result.Changes)
	}
	contents, err := os.ReadFile(filepath.Join(root, "courses", "CS3103", "raw", "modules.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contents), "## Week One") || !strings.Contains(string(contents), "## Week Two") {
		t.Fatalf("modules.md lost an unchanged module:\n%s", contents)
	}
}

func TestSyncTreatsEquivalentAssignmentOffsetsAsSameMinute(t *testing.T) {
	root, workspace, course := initializedCanvasVault(t)
	course.Canvas.Sources = []string{"assignments"}
	client := &syncClient{lists: map[string][]map[string]any{
		"/api/v1/courses/1/assignments": {{"id": float64(7), "name": "Essay", "due_at": "2026-09-07T00:00:30Z", "description": "<p>Write</p>"}},
	}}
	if _, err := Sync(context.Background(), root, workspace, course, client, false); err != nil {
		t.Fatal(err)
	}
	result, err := Sync(context.Background(), root, workspace, course, client, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "up_to_date" || len(result.Changes) != 0 {
		t.Fatalf("second Sync() = %#v", result)
	}
}

func TestSyncRejectsNestedSymlinkBeforeAttachmentWrite(t *testing.T) {
	root, workspace, course := initializedCanvasVault(t)
	course.Canvas.Sources = []string{"files"}
	course.Canvas.Folders = map[string]string{"Danger": "escape"}
	outside := t.TempDir()
	raw := filepath.Join(root, "courses", "CS3103", "raw")
	if err := os.Symlink(outside, filepath.Join(raw, "escape")); err != nil {
		t.Fatal(err)
	}
	client := &syncClient{lists: map[string][]map[string]any{
		"/api/v1/courses/1/files":   {{"id": float64(5), "display_name": "outside.txt", "folder_id": float64(9), "url": "https://canvas.example.edu/file?verifier=secret"}},
		"/api/v1/courses/1/folders": {{"id": float64(9), "full_name": "course files/Danger"}},
	}}
	result, err := Sync(context.Background(), root, workspace, course, client, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Failures) != 1 || len(client.downloads) != 0 {
		t.Fatalf("result = %#v, downloads = %#v", result, client.downloads)
	}
	if _, err := os.Stat(filepath.Join(outside, "outside.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("write escaped through symlink: %v", err)
	}
}

func TestSyncRestoresNormalizedCaptureMetadata(t *testing.T) {
	root, workspace, course := initializedCanvasVault(t)
	course.Canvas.Sources = []string{"announcements", "assignments", "files", "pages", "modules", "syllabus"}
	client := &syncClient{
		gets: map[string]map[string]any{
			"/api/v1/courses/1": {"syllabus_body": "<p>Syllabus</p>", "html_url": "https://canvas.example.edu/courses/1/assignments/syllabus?verifier=secret"},
		},
		lists: map[string][]map[string]any{
			"/api/v1/courses/1/discussion_topics": {{"id": float64(1), "title": "News", "posted_at": "2026-09-06T00:00:00Z", "user_name": "Ada", "html_url": "https://canvas.example.edu/a?verifier=secret", "message": "<p>Hello</p>"}},
			"/api/v1/courses/1/assignments":       {{"id": float64(2), "name": "Essay", "due_at": "2026-09-07T00:00:00Z", "points_possible": float64(20), "html_url": "https://canvas.example.edu/b?verifier=secret", "description": "<p>Write</p>"}},
			"/api/v1/courses/1/files":             {{"id": float64(3), "display_name": "slides.pdf", "folder_id": float64(30), "url": "https://canvas.example.edu/file?verifier=secret"}},
			"/api/v1/courses/1/folders":           {{"id": float64(30), "full_name": "course files/Lectures"}},
			"/api/v1/courses/1/pages":             {{"url": "lesson", "title": "Lesson", "updated_at": "2026-09-06T01:00:00Z", "html_url": "https://canvas.example.edu/page?verifier=secret", "body": "<p>Page</p>"}},
			"/api/v1/courses/1/modules":           {{"id": float64(4), "name": "Week One", "position": float64(1)}},
			"/api/v1/courses/1/modules/4/items":   {{"id": float64(40), "position": float64(1), "type": "Page", "title": "Lesson"}},
		},
		errors: map[string]error{"get:/api/v1/courses/1/front_page": errors.New("no front page")},
	}
	result, err := Sync(context.Background(), root, workspace, course, client, false)
	if err != nil {
		t.Fatal(err)
	}
	details := map[string]map[string]any{}
	for _, change := range result.Changes {
		details[change.Source] = change.Details
	}
	checks := map[string]map[string]any{
		"announcements": {"author": "Ada", "posted": "2026-09-06T08:00:00+08:00", "url": "https://canvas.example.edu/a"},
		"assignments":   {"points": float64(20), "due": "2026-09-07T08:00:00+08:00", "url": "https://canvas.example.edu/b"},
		"files":         {"folder_id": "30", "folder": "Lectures"},
		"pages":         {"page_url": "lesson", "updated": "2026-09-06T09:00:00+08:00", "url": "https://canvas.example.edu/page"},
		"modules":       {"name": "Week One", "position": float64(1)},
		"syllabus":      {"url": "https://canvas.example.edu/courses/1/assignments/syllabus"},
	}
	for source, fields := range checks {
		for name, want := range fields {
			if got := details[source][name]; got != want {
				t.Errorf("%s %s = %#v, want %#v; all details = %#v", source, name, got, want, details[source])
			}
		}
	}
	for relative, fragments := range map[string][]string{
		"announcements/news-1.md": {"author: Ada", "posted: \"2026-09-06T08:00:00+08:00\""},
		"assignments/essay-2.md":  {"points: 20", "due: \"2026-09-07T08:00:00+08:00\""},
		"pages/lesson-lesson.md":  {"page_url: lesson", "updated: \"2026-09-06T09:00:00+08:00\"", "url: https://canvas.example.edu/page"},
		"syllabus.md":             {"url: https://canvas.example.edu/courses/1/assignments/syllabus"},
	} {
		data, readErr := os.ReadFile(filepath.Join(root, "courses", "CS3103", "raw", filepath.FromSlash(relative)))
		if readErr != nil {
			t.Fatal(readErr)
		}
		for _, fragment := range fragments {
			if !strings.Contains(string(data), fragment) {
				t.Errorf("%s missing %q:\n%s", relative, fragment, data)
			}
		}
	}
}

func TestSyncPersistsManifestBeforeAdvancingCanvasState(t *testing.T) {
	root, workspace, course := initializedCanvasVault(t)
	client := &syncClient{lists: map[string][]map[string]any{"/api/v1/courses/1/discussion_topics": {}}}
	originalManifest := writeCanvasManifest
	originalState := writeCanvasState
	t.Cleanup(func() {
		writeCanvasManifest = originalManifest
		writeCanvasState = originalState
	})
	order := []string{}
	writeCanvasManifest = func(path string, manifest RunManifest) error {
		order = append(order, "manifest")
		return originalManifest(path, manifest)
	}
	writeCanvasState = func(path string, state CanvasState, zone string) error {
		order = append(order, "state")
		return originalState(path, state, zone)
	}
	if _, err := Sync(context.Background(), root, workspace, course, client, false); err != nil {
		t.Fatal(err)
	}
	if strings.Join(order, ",") != "manifest,state" {
		t.Fatalf("persistence order = %v", order)
	}
}

func TestSyncManifestFailureLeavesCanvasStateUnadvanced(t *testing.T) {
	root, workspace, course := initializedCanvasVault(t)
	client := &syncClient{lists: map[string][]map[string]any{"/api/v1/courses/1/discussion_topics": {}}}
	originalManifest := writeCanvasManifest
	originalState := writeCanvasState
	t.Cleanup(func() {
		writeCanvasManifest = originalManifest
		writeCanvasState = originalState
	})
	writeCanvasManifest = func(string, RunManifest) error { return errors.New("injected manifest failure") }
	stateCalled := false
	writeCanvasState = func(string, CanvasState, string) error { stateCalled = true; return nil }
	if _, err := Sync(context.Background(), root, workspace, course, client, false); err == nil || !strings.Contains(err.Error(), "injected manifest failure") {
		t.Fatalf("Sync() error = %v", err)
	}
	if stateCalled {
		t.Fatal("Canvas state advanced after manifest failure")
	}
}

func TestSyncReturnsFullRunManifest(t *testing.T) {
	root, workspace, course := initializedCanvasVault(t)
	client := &syncClient{lists: map[string][]map[string]any{"/api/v1/courses/1/discussion_topics": {}}}
	result, err := Sync(context.Background(), root, workspace, course, client, false)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(result.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(encoded, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest["run_id"] == "" || manifest["course"] != "CS3103" || manifest["jira"] == nil || manifest["wiki"] == nil || manifest["effective_features"] == nil {
		t.Fatalf("manifest = %#v", manifest)
	}
}

func TestSyncUsesSharedCourseLockBeforeReadingOrWritingState(t *testing.T) {
	root, workspace, course := initializedCanvasVault(t)
	stateDir := filepath.Join(root, "courses", "CS3103", "state")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	lock, err := lockfile.TryAcquire(filepath.Join(stateDir, ".course.lock"))
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	client := &syncClient{lists: map[string][]map[string]any{"/api/v1/courses/1/discussion_topics": {}}}
	if _, err := Sync(context.Background(), root, workspace, course, client, false); !errors.Is(err, lockfile.ErrLocked) {
		t.Fatalf("Sync() error = %v, want ErrLocked", err)
	}
}
