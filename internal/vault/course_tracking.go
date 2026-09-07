package vault

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/algebananazzzzz/Corum/internal/config"
	"gopkg.in/yaml.v3"
)

var unsafeCourseCode = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

const maxCourseCodeLength = 120

var defaultCanvasSources = []string{"announcements", "assignments", "files", "pages", "modules", "syllabus"}

// CanvasCourseChoice is one current Canvas course offered for tracking.
type CanvasCourseChoice struct {
	ID   string
	Code string
	Name string
}

// CanvasTrackingResult summarizes only current courses affected by a selection.
type CanvasTrackingResult struct {
	Enabled   []string
	Disabled  []string
	Unchanged []string
}

// TrackedCanvasIDs returns the Canvas IDs already enabled in the project.
func TrackedCanvasIDs(root string) (map[string]bool, error) {
	_, courses, err := Validate(root)
	if err != nil {
		return nil, err
	}
	result := make(map[string]bool)
	for _, course := range courses {
		if course.Canvas != nil {
			result[strconv.Itoa(course.Canvas.ID)] = true
		}
	}
	return result, nil
}

// ConfigureCanvasCourses applies an explicit selection over the supplied
// current-course set. Tracked courses absent from that set are not changed.
func ConfigureCanvasCourses(root string, current []CanvasCourseChoice, selectedIDs []string) (CanvasTrackingResult, error) {
	return configureCanvasCourses(root, current, selectedIDs, writeConfigAtomic)
}

func configureCanvasCourses(root string, current []CanvasCourseChoice, selectedIDs []string, write func(string, []byte) error) (CanvasTrackingResult, error) {
	_, courses, err := Validate(root)
	if err != nil {
		return CanvasTrackingResult{}, err
	}
	visible := make(map[string]CanvasCourseChoice, len(current))
	for _, course := range current {
		id, err := positiveCourseID(course.ID)
		if err != nil {
			return CanvasTrackingResult{}, err
		}
		key := strconv.Itoa(id)
		if _, exists := visible[key]; exists {
			return CanvasTrackingResult{}, fmt.Errorf("Canvas returned duplicate course id %s", key)
		}
		course.ID = key
		visible[key] = course
	}
	selected := make(map[string]bool, len(selectedIDs))
	for _, rawID := range selectedIDs {
		id, err := positiveCourseID(rawID)
		if err != nil {
			return CanvasTrackingResult{}, err
		}
		key := strconv.Itoa(id)
		if _, ok := visible[key]; !ok {
			return CanvasTrackingResult{}, fmt.Errorf("selected Canvas course %s is not current", key)
		}
		selected[key] = true
	}

	byCode := make(map[string]int, len(courses))
	byCanvasID := make(map[string]int)
	for i, course := range courses {
		byCode[course.Code] = i
		if course.Canvas == nil {
			continue
		}
		key := strconv.Itoa(course.Canvas.ID)
		if _, exists := byCanvasID[key]; exists {
			return CanvasTrackingResult{}, fmt.Errorf("Canvas course id %s is configured more than once", key)
		}
		byCanvasID[key] = i
	}

	result := CanvasTrackingResult{}
	changed := make(map[string]config.Course)
	for id, index := range byCanvasID {
		if _, isVisible := visible[id]; !isVisible {
			continue
		}
		course := courses[index]
		if selected[id] {
			result.Unchanged = append(result.Unchanged, course.Code)
			continue
		}
		course.Canvas = nil
		changed[course.Code] = course
		courses[index] = course
		result.Disabled = append(result.Disabled, course.Code)
	}

	for _, original := range current {
		id, _ := positiveCourseID(original.ID)
		choice := visible[strconv.Itoa(id)]
		if !selected[choice.ID] {
			continue
		}
		if _, exists := byCanvasID[choice.ID]; exists {
			continue
		}
		code, err := availableCourseCode(root, normalizedCourseCode(choice), choice.ID, byCode)
		if err != nil {
			return CanvasTrackingResult{}, err
		}
		course := config.Course{Version: 2, Code: code, Canvas: newCanvasCourse(id)}
		courses = append(courses, course)
		byCode[code] = len(courses) - 1
		changed[code] = course
		byCanvasID[choice.ID] = byCode[code]
		result.Enabled = append(result.Enabled, code)
	}

	type change struct {
		path      string
		directory string
		data      []byte
		before    []byte
		exists    bool
		dirExists bool
	}
	codes := make([]string, 0, len(changed))
	for code := range changed {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	changes := make([]change, 0, len(codes))
	for _, code := range codes {
		course := changed[code]
		if err := config.ValidateCourse(course); err != nil {
			return CanvasTrackingResult{}, fmt.Errorf("validate course %s: %w", code, err)
		}
		data, err := yaml.Marshal(course)
		if err != nil {
			return CanvasTrackingResult{}, err
		}
		path := filepath.Join(root, "courses", code, "course.yaml")
		directory := filepath.Dir(path)
		_, statErr := os.Stat(directory)
		if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
			return CanvasTrackingResult{}, statErr
		}
		before, readErr := os.ReadFile(path)
		if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
			return CanvasTrackingResult{}, readErr
		}
		changes = append(changes, change{path: path, directory: directory, data: data, before: before, exists: readErr == nil, dirExists: statErr == nil})
	}
	for i, item := range changes {
		if err := write(item.path, item.data); err != nil {
			var rollbackErrors []error
			for rollback := i; rollback >= 0; rollback-- {
				previous := changes[rollback]
				if previous.exists {
					if restoreErr := writeConfigAtomic(previous.path, previous.before); restoreErr != nil {
						rollbackErrors = append(rollbackErrors, restoreErr)
					}
				} else if removeErr := os.Remove(previous.path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
					rollbackErrors = append(rollbackErrors, removeErr)
				} else if !previous.dirExists {
					if removeDirErr := os.Remove(previous.directory); removeDirErr != nil && !errors.Is(removeDirErr, os.ErrNotExist) {
						rollbackErrors = append(rollbackErrors, removeDirErr)
					}
				}
			}
			return CanvasTrackingResult{}, errors.Join(err, errors.Join(rollbackErrors...))
		}
	}
	sort.Strings(result.Enabled)
	sort.Strings(result.Disabled)
	sort.Strings(result.Unchanged)
	return result, nil
}

func positiveCourseID(value string) (int, error) {
	id, err := strconv.Atoi(value)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("Canvas course id %q is invalid", value)
	}
	return id, nil
}

func newCanvasCourse(id int) *config.CanvasCourse {
	return &config.CanvasCourse{ID: id, Sources: append([]string(nil), defaultCanvasSources...), Folders: map[string]string{}}
}

func normalizedCourseCode(choice CanvasCourseChoice) string {
	code := strings.TrimSpace(choice.Code)
	if code == "" {
		code = strings.TrimSpace(choice.Name)
	}
	code = unsafeCourseCode.ReplaceAllString(code, "-")
	code = strings.Trim(code, "-_")
	if code == "" {
		return "canvas-" + choice.ID
	}
	if len(code) > maxCourseCodeLength {
		code = strings.TrimRight(code[:maxCourseCodeLength], "-_")
	}
	return code
}

func availableCourseCode(root, base, id string, existing map[string]int) (string, error) {
	if _, configured := existing[base]; !configured {
		available, err := coursePathAvailable(root, base)
		if err != nil {
			return "", err
		}
		if available {
			return base, nil
		}
	}
	for suffix := 0; suffix < 10_000; suffix++ {
		candidate := base + "-" + id
		if suffix > 0 {
			candidate = fmt.Sprintf("%s-%s-%d", base, id, suffix+1)
		}
		if _, configured := existing[candidate]; configured {
			continue
		}
		available, err := coursePathAvailable(root, candidate)
		if err != nil {
			return "", err
		}
		if available {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("could not allocate a local code for Canvas course %s", id)
}

func coursePathAvailable(root, code string) (bool, error) {
	path := filepath.Join(root, "courses", code)
	_, err := os.Lstat(path)
	if err == nil {
		return false, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return true, nil
	}
	return false, fmt.Errorf("inspect course path %q: %w", path, err)
}
