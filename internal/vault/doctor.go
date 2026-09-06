package vault

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/algebananazzzzz/Corum/internal/config"
)

// Validate loads a vault and every course file, rejects duplicate course codes,
// and registers the resulting absolute vault path only after success.
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
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
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
	if err := Register(root); err != nil {
		return config.Workspace{}, nil, err
	}
	return workspace, courses, nil
}
