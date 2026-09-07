package vault

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/algebananazzzzz/Corum/internal/config"
)

func TestConfigureCanvasCoursesReconcilesOnlyVisibleSelection(t *testing.T) {
	root := initializedVault(t, "test")
	writeCourseFixture(t, root, "OLD", "version: 2\ncode: OLD\ncanvas:\n  id: 1\n  sources: [assignments]\njira:\n  epic: TODO-1\nwiki: {}\n")
	writeCourseFixture(t, root, "ABSENT", "version: 2\ncode: ABSENT\ncanvas:\n  id: 3\n  sources: [pages]\n")
	sentinel := filepath.Join(root, "courses", "OLD", "state", "keep")
	if err := os.MkdirAll(filepath.Dir(sentinel), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sentinel, []byte("preserve"), 0o644); err != nil {
		t.Fatal(err)
	}

	current := []CanvasCourseChoice{
		{ID: "1", Code: "OLD", Name: "Old Course"},
		{ID: "2", Code: "CS2103/CS2103T", Name: "Software Engineering"},
	}
	result, err := ConfigureCanvasCourses(root, current, []string{"2"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.Enabled, []string{"CS2103-CS2103T"}) || !reflect.DeepEqual(result.Disabled, []string{"OLD"}) {
		t.Fatalf("result = %+v", result)
	}

	old, err := config.LoadCourse(root, "OLD")
	if err != nil || old.Canvas != nil || old.Jira == nil || old.Wiki == nil {
		t.Fatalf("OLD = %+v, %v", old, err)
	}
	if got, err := os.ReadFile(sentinel); err != nil || string(got) != "preserve" {
		t.Fatalf("sentinel = %q, %v", got, err)
	}
	absent, err := config.LoadCourse(root, "ABSENT")
	if err != nil || absent.Canvas == nil || absent.Canvas.ID != 3 {
		t.Fatalf("ABSENT = %+v, %v", absent, err)
	}
	added, err := config.LoadCourse(root, "CS2103-CS2103T")
	if err != nil || added.Canvas == nil || added.Canvas.ID != 2 {
		t.Fatalf("added = %+v, %v", added, err)
	}
	wantSources := []string{"announcements", "assignments", "files", "pages", "modules", "syllabus"}
	if !reflect.DeepEqual(added.Canvas.Sources, wantSources) {
		t.Fatalf("sources = %#v", added.Canvas.Sources)
	}
}

func TestConfigureCanvasCoursesNormalizesIDs(t *testing.T) {
	root := initializedVault(t, "test")
	current := []CanvasCourseChoice{{ID: "007", Code: "CS101"}}
	result, err := ConfigureCanvasCourses(root, current, []string{"7"})
	if err != nil || !reflect.DeepEqual(result.Enabled, []string{"CS101"}) {
		t.Fatalf("ConfigureCanvasCourses() = %+v, %v", result, err)
	}
	course, err := config.LoadCourse(root, "CS101")
	if err != nil || course.Canvas == nil || course.Canvas.ID != 7 {
		t.Fatalf("course = %+v, %v", course, err)
	}
	if current[0].ID != "007" {
		t.Fatal("caller course selection was modified")
	}
}

func TestConfigureCanvasCoursesRollsBackEarlierWrites(t *testing.T) {
	root := initializedVault(t, "test")
	writeCourseFixture(t, root, "ONE", "version: 2\ncode: ONE\ncanvas:\n  id: 1\n  sources: [assignments]\n")
	writeCourseFixture(t, root, "TWO", "version: 2\ncode: TWO\ncanvas:\n  id: 2\n  sources: [assignments]\n")
	onePath := filepath.Join(root, "courses", "ONE", "course.yaml")
	oneBefore, err := os.ReadFile(onePath)
	if err != nil {
		t.Fatal(err)
	}
	writes := 0
	_, err = configureCanvasCourses(root, []CanvasCourseChoice{{ID: "1", Code: "ONE"}, {ID: "2", Code: "TWO"}}, nil, func(path string, data []byte) error {
		writes++
		if writes == 2 {
			return errors.New("injected write failure")
		}
		return writeConfigAtomic(path, data)
	})
	if err == nil || !strings.Contains(err.Error(), "injected write failure") {
		t.Fatalf("error = %v", err)
	}
	oneAfter, err := os.ReadFile(onePath)
	if err != nil || string(oneAfter) != string(oneBefore) {
		t.Fatalf("ONE changed after rollback: %q, %v", oneAfter, err)
	}
}

func TestConfigureCanvasCoursesRemovesNewDirectoryOnRollback(t *testing.T) {
	root := initializedVault(t, "test")
	writeCourseFixture(t, root, "OLD", "version: 2\ncode: OLD\ncanvas:\n  id: 1\n  sources: [assignments]\n")
	newDirectory := filepath.Join(root, "courses", "ANEW")
	writes := 0

	_, err := configureCanvasCourses(root, []CanvasCourseChoice{{ID: "1", Code: "OLD"}, {ID: "2", Code: "ANEW"}}, []string{"2"}, func(path string, data []byte) error {
		writes++
		if writes == 2 {
			return errors.New("injected write failure")
		}
		return writeConfigAtomic(path, data)
	})
	if err == nil || !strings.Contains(err.Error(), "injected write failure") {
		t.Fatalf("error = %v", err)
	}
	if _, err := os.Stat(newDirectory); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("new directory survived rollback: %v", err)
	}
}

func TestConfigureCanvasCoursesCleansDirectoryCreatedByFailingWrite(t *testing.T) {
	root := initializedVault(t, "test")
	newDirectory := filepath.Join(root, "courses", "ANEW")

	_, err := configureCanvasCourses(root, []CanvasCourseChoice{{ID: "2", Code: "ANEW"}}, []string{"2"}, func(path string, _ []byte) error {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		return errors.New("injected write failure")
	})
	if err == nil || !strings.Contains(err.Error(), "injected write failure") {
		t.Fatalf("error = %v", err)
	}
	if _, err := os.Stat(newDirectory); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failing write directory survived rollback: %v", err)
	}
}

func TestConfigureCanvasCoursesSuffixesConflictingCourseCode(t *testing.T) {
	root := initializedVault(t, "test")
	writeCourseFixture(t, root, "CS1010", "version: 2\ncode: CS1010\ncanvas:\n  id: 1\n  sources: [assignments]\n")

	result, err := ConfigureCanvasCourses(root, []CanvasCourseChoice{{ID: "2", Code: "CS1010", Name: "Programming Methodology"}}, []string{"2"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.Enabled, []string{"CS1010-2"}) {
		t.Fatalf("result = %+v", result)
	}
	original, err := config.LoadCourse(root, "CS1010")
	if err != nil || original.Canvas == nil || original.Canvas.ID != 1 {
		t.Fatalf("original = %+v, %v", original, err)
	}
	added, err := config.LoadCourse(root, "CS1010-2")
	if err != nil || added.Canvas == nil || added.Canvas.ID != 2 {
		t.Fatalf("added = %+v, %v", added, err)
	}
}

func TestConfigureCanvasCoursesDoesNotGraftOntoNonCanvasCourse(t *testing.T) {
	root := initializedVault(t, "test")
	writeCourseFixture(t, root, "CS1010", "version: 2\ncode: CS1010\njira:\n  epic: TODO-1\nwiki: {}\n")

	result, err := ConfigureCanvasCourses(root, []CanvasCourseChoice{{ID: "2", Code: "CS1010"}}, []string{"2"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.Enabled, []string{"CS1010-2"}) {
		t.Fatalf("result = %+v", result)
	}
	original, err := config.LoadCourse(root, "CS1010")
	if err != nil || original.Canvas != nil || original.Jira == nil || original.Wiki == nil {
		t.Fatalf("original = %+v, %v", original, err)
	}
}

func TestAvailableCourseCodePropagatesFilesystemErrors(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "courses"), []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := availableCourseCode(root, "CS1010", "2", map[string]int{}); err == nil {
		t.Fatal("availableCourseCode ignored a filesystem error")
	}
}

func TestNormalizedCourseCodeIsBounded(t *testing.T) {
	code := normalizedCourseCode(CanvasCourseChoice{ID: "2", Name: strings.Repeat("A", 500)})
	if len(code) > maxCourseCodeLength {
		t.Fatalf("normalized code has length %d", len(code))
	}
}

func writeCourseFixture(t *testing.T, root, code, contents string) {
	t.Helper()
	path := filepath.Join(root, "courses", code, "course.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
