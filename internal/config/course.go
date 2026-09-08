package config

import (
	"fmt"
	"path/filepath"
)

// Course is courses/<code>/course.yaml.
type Course struct {
	Version int           `yaml:"version"`
	Code    string        `yaml:"code"`
	Canvas  *CanvasCourse `yaml:"canvas,omitempty"`
	Jira    *JiraCourse   `yaml:"jira,omitempty"`
}

type CanvasCourse struct {
	ID      int               `yaml:"id"`
	Name    string            `yaml:"name,omitempty"`
	Sources []string          `yaml:"sources"`
	Folders map[string]string `yaml:"folders"`
}

type JiraCourse struct {
	Epic string `yaml:"epic"`
}

// Services is the effective set for a course.
type Services struct {
	Canvas bool
	Jira   bool
	Wiki   bool
}

// Effective uses vault-wide Jira and wiki settings. Canvas remains course-specific.
func Effective(workspace Workspace, course Course) Services {
	return Services{
		Canvas: workspace.Canvas != nil && course.Canvas != nil,
		Jira:   workspace.Jira != nil,
		Wiki:   workspace.Wiki != nil,
	}
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
