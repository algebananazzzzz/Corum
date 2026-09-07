package canvas

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadCredentialUsesOnlyEnvironmentThenProjectLocal(t *testing.T) {
	globalConfig := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", globalConfig)
	t.Setenv("CORUM_CANVAS_TOKEN", "")
	root := t.TempDir()
	project, _ := CredentialPathFor(root)

	if _, err := LoadCredential(root); !os.IsNotExist(err) && err != ErrNoCredential {
		t.Fatalf("LoadCredential() = %v, want ErrNoCredential", err)
	}
	if err := SaveCredential(root, "project-token"); err != nil {
		t.Fatal(err)
	}
	token, err := LoadCredential(root)
	if err != nil || token != "project-token" {
		t.Fatalf("LoadCredential() = %q, %v", token, err)
	}
	t.Setenv("CORUM_CANVAS_TOKEN", "env-token")
	token, err = LoadCredential(root)
	if err != nil || token != "env-token" {
		t.Fatalf("env precedence: %q, %v", token, err)
	}
	t.Setenv("CORUM_CANVAS_TOKEN", "")
	globalPath := filepath.Join(globalConfig, "corum", "canvas.json")
	if err := os.MkdirAll(filepath.Dir(globalPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(globalPath, []byte(`{"version":1,"token":"global-token"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(project); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadCredential(root); err != ErrNoCredential {
		t.Fatalf("global credential was used: %v", err)
	}
}

func TestCredentialPathRequiresProjectRoot(t *testing.T) {
	if _, err := CredentialPathFor(""); err == nil {
		t.Fatal("CredentialPathFor accepted an empty project root")
	}
}

func TestSaveCredentialReplacesWithPrivateFile(t *testing.T) {
	root := t.TempDir()
	path, _ := CredentialPathFor(root)
	if err := SaveCredential(root, "old"); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SaveCredential(root, "new"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("credential mode = %o, want 600", info.Mode().Perm())
	}
}

func TestSaveCredentialUsesPrivateModesAndGitignore(t *testing.T) {
	root := t.TempDir()
	path, _ := CredentialPathFor(root)
	if err := SaveCredential(root, "secret"); err != nil {
		t.Fatal(err)
	}
	dir, err := os.Stat(filepath.Dir(path))
	if err != nil || dir.Mode().Perm() != 0o700 {
		t.Fatalf("dir mode = %v, %v", dir.Mode(), err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("file mode = %v, %v", info.Mode(), err)
	}
	guard, err := os.ReadFile(filepath.Join(filepath.Dir(path), ".gitignore"))
	if err != nil || !strings.Contains(string(guard), "*") {
		t.Fatalf("gitignore guard = %q, %v", guard, err)
	}
}

func TestCoursesListsAuthenticatedCourses(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/courses" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("enrollment_state") != "active" || r.URL.Query().Get("include[]") != "concluded" {
			http.Error(w, "missing current-course filters", http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`[{"id":7,"course_code":"CS3103","name":"Algorithms","concluded":false},{"id":8,"course_code":"CS1010","name":"Intro","concluded":true},{"id":9}]`))
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "token")
	if err != nil {
		t.Fatal(err)
	}
	client.http = server.Client()
	courses, err := client.Courses(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(courses) != 1 || courses[0].ID != "7" || courses[0].CourseCode != "CS3103" || courses[0].Name != "Algorithms" || !courses[0].Current {
		t.Fatalf("courses = %#v", courses)
	}
}
