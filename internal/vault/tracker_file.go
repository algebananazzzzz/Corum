package vault

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/algebananazzzzz/Corum/internal/config"
	"github.com/google/jsonschema-go/jsonschema"
)

// ValidateTrackerFiles checks each course's agent-written state/tracker.json
// against the schema and the vault, so an agent can find and fix its own
// mistakes before planning relies on the file.
func ValidateTrackerFiles(root string, workspace config.Workspace, courses []config.Course, schemaData []byte) error {
	var schema jsonschema.Schema
	if err := json.Unmarshal(schemaData, &schema); err != nil {
		return fmt.Errorf("tracker.json schema: %w", err)
	}
	resolved, err := schema.Resolve(nil)
	if err != nil {
		return fmt.Errorf("tracker.json schema: %w", err)
	}
	for _, course := range courses {
		relative := filepath.Join("courses", course.Code, "state", "tracker.json")
		data, err := os.ReadFile(filepath.Join(root, relative))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		var value any
		if err := json.Unmarshal(data, &value); err != nil {
			return fmt.Errorf("%s: %w", relative, err)
		}
		if err := resolved.Validate(value); err != nil {
			return fmt.Errorf("%s: %w", relative, err)
		}
		var file struct {
			Tracker string `json:"tracker"`
			Group   *struct {
				Name string `json:"name"`
			} `json:"group"`
		}
		if err := json.Unmarshal(data, &file); err != nil {
			return fmt.Errorf("%s: %w", relative, err)
		}
		if file.Tracker != workspace.TaskTracker {
			active := workspace.TaskTracker
			if active == "" {
				active = "none"
			}
			return fmt.Errorf("%s: file is from %s but the task tracker is %s; sync the course again", relative, file.Tracker, active)
		}
		if file.Group != nil && file.Group.Name != course.Code {
			return fmt.Errorf("%s: course group %q must be named after course code %q", relative, file.Group.Name, course.Code)
		}
	}
	return nil
}
