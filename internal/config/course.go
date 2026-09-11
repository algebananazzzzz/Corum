package config

import (
	"fmt"
	"path/filepath"
)

// Course is courses/<code>/course.yaml.
type Course struct {
	Code        string             `yaml:"code"`
	Canvas      *CanvasCourse      `yaml:"canvas,omitempty"`
	Jira        *JiraCourse        `yaml:"jira,omitempty"`
	GoogleTasks *GoogleTasksCourse `yaml:"google_tasks,omitempty"`
}

type CanvasCourse struct {
	ID      int               `yaml:"id"`
	Name    string            `yaml:"name,omitempty"`
	Sources []string          `yaml:"sources"`
	Folders map[string]string `yaml:"folders,omitempty"`
}

type JiraCourse struct {
	Epic string `yaml:"epic"`
}

type GoogleTasksCourse struct {
	ListID string `yaml:"list_id"`
}

// LoadCourse reads one course configuration below root without changing the vault.
func LoadCourse(root, code string) (Course, error) {
	if !courseCodeRE.MatchString(code) {
		return Course{}, fmt.Errorf("invalid course code %q", code)
	}
	var course Course
	path := filepath.Join(root, "courses", code, "course.yaml")
	if err := decodeFile(path, &course); err != nil {
		return Course{}, fmt.Errorf("load course %q: %w", code, err)
	}
	if err := ValidateCourse(course); err != nil {
		return Course{}, fmt.Errorf("load course %q: %w", code, err)
	}
	if course.Code != code {
		return Course{}, fmt.Errorf("load course %q: declared code does not match directory", code)
	}
	return course, nil
}
