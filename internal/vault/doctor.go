package vault

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/algebananazzzzz/Corum/internal/config"
)

// Validate loads the vault and every course file, and rejects duplicate course codes.
func Validate(root string) (config.Workspace, []config.Course, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return config.Workspace{}, nil, err
	}
	workspace, err := config.LoadWorkspace(root)
	if err != nil {
		return config.Workspace{}, nil, err
	}
	entries, err := os.ReadDir(filepath.Join(root, "courses"))
	if err != nil && !os.IsNotExist(err) {
		return config.Workspace{}, nil, err
	}
	courses := make([]config.Course, 0)
	codes := map[string]struct{}{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		coursePath := filepath.Join(root, "courses", entry.Name(), "course.yaml")
		if _, err := os.Stat(coursePath); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return config.Workspace{}, nil, err
		}
		course, err := config.LoadCourse(root, entry.Name())
		if err != nil {
			return config.Workspace{}, nil, err
		}
		if _, exists := codes[course.Code]; exists {
			return config.Workspace{}, nil, fmt.Errorf("duplicate course code %q", course.Code)
		}
		codes[course.Code] = struct{}{}
		courses = append(courses, course)
	}
	return workspace, courses, nil
}
