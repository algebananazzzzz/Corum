package jira

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/algebananazzzzz/Corum/internal/config"
)

var (
	planIdentifierRE = regexp.MustCompile(`^\S+$`)
	planProjectRE    = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)
	planIssueRE      = regexp.MustCompile(`^[A-Z][A-Z0-9_]*-[1-9][0-9]*$`)
	planTransitionRE = regexp.MustCompile(`^[0-9]+$`)
)

func DecodePlan(reader io.Reader) (Plan, error) {
	var wire struct {
		Version int               `json:"version"`
		Course  string            `json:"course"`
		Epic    string            `json:"epic"`
		Actions []json.RawMessage `json:"actions"`
	}
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return Plan{}, fmt.Errorf("decode Jira plan: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return Plan{}, fmt.Errorf("decode Jira plan: trailing JSON value")
		}
		return Plan{}, fmt.Errorf("decode Jira plan: %w", err)
	}
	plan := Plan{Version: wire.Version, Course: wire.Course, Epic: wire.Epic, Actions: make([]Action, 0, len(wire.Actions))}
	for index, raw := range wire.Actions {
		action, err := decodeAction(raw)
		if err != nil {
			return Plan{}, fmt.Errorf("decode Jira plan actions[%d]: %w", index, err)
		}
		plan.Actions = append(plan.Actions, action)
	}
	if err := validatePlanShape(plan); err != nil {
		return Plan{}, err
	}
	return plan, nil
}

func decodeAction(raw json.RawMessage) (Action, error) {
	var discriminator struct {
		Action string `json:"action"`
	}
	if err := json.Unmarshal(raw, &discriminator); err != nil {
		return Action{}, err
	}
	switch discriminator.Action {
	case "create":
		var wire struct {
			Action string       `json:"action"`
			Issue  *IssueFields `json:"issue"`
		}
		if err := decodeStrictBytes(raw, &wire); err != nil {
			return Action{}, err
		}
		return Action{Action: wire.Action, Issue: wire.Issue}, nil
	case "update":
		var wire struct {
			Action string         `json:"action"`
			Key    string         `json:"key"`
			Set    map[string]any `json:"set"`
		}
		if err := decodeStrictBytes(raw, &wire); err != nil {
			return Action{}, err
		}
		return Action{Action: wire.Action, Key: wire.Key, Set: wire.Set}, nil
	case "transition":
		var wire struct {
			Action     string `json:"action"`
			Key        string `json:"key"`
			Transition string `json:"transition"`
		}
		if err := decodeStrictBytes(raw, &wire); err != nil {
			return Action{}, err
		}
		return Action{Action: wire.Action, Key: wire.Key, Transition: wire.Transition}, nil
	default:
		return Action{}, &ValidationError{Message: fmt.Sprintf("unknown Jira action %q", discriminator.Action)}
	}
}

func decodeStrictBytes(raw []byte, target any) error {
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func validatePlanShape(plan Plan) error {
	if plan.Version != 2 {
		return &ValidationError{Message: "Jira plan version must be 2"}
	}
	if !planIdentifierRE.MatchString(plan.Course) {
		return &ValidationError{Message: "Jira plan course has invalid syntax"}
	}
	if err := validateIssueKey(plan.Epic, "Jira plan epic"); err != nil {
		return err
	}
	if plan.Actions == nil {
		return &ValidationError{Message: "Jira plan actions are required"}
	}
	for index, action := range plan.Actions {
		if err := validateAction(action); err != nil {
			return &ValidationError{Message: fmt.Sprintf("actions[%d]: %v", index, err)}
		}
	}
	return nil
}

func validateAction(action Action) error {
	switch action.Action {
	case "create":
		if action.Issue == nil || action.Key != "" || action.Set != nil || action.Transition != "" {
			return fmt.Errorf("create action has invalid fields")
		}
		return validateCreateFields(*action.Issue)
	case "update":
		if action.Issue != nil || action.Transition != "" || action.Set == nil || len(action.Set) == 0 {
			return fmt.Errorf("update action has invalid fields")
		}
		if err := validateIssueKey(action.Key, "Jira issue key"); err != nil {
			return err
		}
		return validateUpdateFields(action.Set)
	case "transition":
		if action.Issue != nil || action.Set != nil || action.Transition == "" {
			return fmt.Errorf("transition action has invalid fields")
		}
		if err := validateIssueKey(action.Key, "Jira issue key"); err != nil {
			return err
		}
		if !planIdentifierRE.MatchString(action.Transition) {
			return fmt.Errorf("transition name has invalid syntax")
		}
		return nil
	default:
		return fmt.Errorf("unknown Jira action %q", action.Action)
	}
}

func validateCreateFields(fields IssueFields) error {
	if !validIssueType(fields.Type) {
		return fmt.Errorf("unsupported Jira issue type %q", fields.Type)
	}
	if err := validateIssueKey(fields.Parent, "Jira parent key"); err != nil {
		return err
	}
	if strings.TrimSpace(fields.Summary) == "" {
		return fmt.Errorf("Jira summary must be non-blank")
	}
	if fields.Due != nil {
		if err := validateDate(*fields.Due, "Jira due date"); err != nil {
			return err
		}
	}
	return nil
}

func validateUpdateFields(fields map[string]any) error {
	for name, value := range fields {
		switch name {
		case "type":
			text, ok := value.(string)
			if !ok || !validIssueType(text) {
				return fmt.Errorf("unsupported Jira issue type")
			}
		case "parent":
			text, ok := value.(string)
			if !ok {
				return fmt.Errorf("Jira parent cannot be cleared")
			}
			if err := validateIssueKey(text, "Jira parent key"); err != nil {
				return err
			}
		case "summary":
			text, ok := value.(string)
			if !ok || strings.TrimSpace(text) == "" {
				return fmt.Errorf("Jira summary must be non-blank and cannot be cleared")
			}
		case "description":
			if value != nil {
				if _, ok := value.(string); !ok {
					return fmt.Errorf("Jira description must be a string or null")
				}
			}
		case "due":
			if value != nil {
				text, ok := value.(string)
				if !ok {
					return fmt.Errorf("Jira due date must be a string or null")
				}
				if err := validateDate(text, "Jira due date"); err != nil {
					return err
				}
			}
		case "labels":
			if value != nil {
				if _, err := stringSlice(value, "Jira labels"); err != nil {
					return err
				}
			}
		default:
			return fmt.Errorf("unsupported Jira field: %s", name)
		}
	}
	return nil
}

func validateConfiguration(workspace config.Workspace, course config.Course, plan Plan, requireCloud bool) (*config.JiraWorkspace, error) {
	if err := validatePlanShape(plan); err != nil {
		return nil, err
	}
	if !config.Effective(workspace, course).Jira || workspace.Jira == nil || course.Jira == nil {
		return nil, &ValidationError{Message: fmt.Sprintf("Jira is disabled for %s", course.Code)}
	}
	if workspace.Version != 2 || course.Version != 2 {
		return nil, &ValidationError{Message: "Jira requires version 2 configuration"}
	}
	if requireCloud && !planIdentifierRE.MatchString(workspace.Jira.CloudID) {
		return nil, &ValidationError{Message: "configured Jira cloud ID has invalid syntax"}
	}
	if !planProjectRE.MatchString(workspace.Jira.Project) {
		return nil, &ValidationError{Message: "configured Jira project has invalid syntax"}
	}
	if err := validateIssueKey(course.Jira.Epic, "configured Jira epic key"); err != nil {
		return nil, err
	}
	if plan.Course != course.Code {
		return nil, &ValidationError{Message: fmt.Sprintf("plan course %q does not match selected course %q", plan.Course, course.Code)}
	}
	if plan.Epic != course.Jira.Epic {
		return nil, &ValidationError{Message: fmt.Sprintf("plan epic %q does not match configured epic %q", plan.Epic, course.Jira.Epic)}
	}
	for index, action := range plan.Actions {
		parent := ""
		if action.Action == "create" {
			parent = action.Issue.Parent
		}
		if action.Action == "update" {
			if value, exists := action.Set["parent"]; exists && value != nil {
				parent, _ = value.(string)
			}
		}
		if parent != "" && parent != plan.Epic {
			return nil, &ValidationError{Message: fmt.Sprintf("actions[%d] parent %q does not match plan epic %q", index, parent, plan.Epic)}
		}
		if action.Action == "transition" {
			id, exists := workspace.Jira.Transitions[action.Transition]
			if !exists {
				return nil, &ValidationError{Message: fmt.Sprintf("actions[%d] names unknown Jira transition %q", index, action.Transition)}
			}
			if !planTransitionRE.MatchString(id) {
				return nil, &ValidationError{Message: fmt.Sprintf("actions[%d] has invalid configured transition", index)}
			}
		}
	}
	return workspace.Jira, nil
}

// ValidatePlan verifies a complete plan and its selected v2 configuration
// without opening OAuth or calling Jira.
func ValidatePlan(workspace config.Workspace, course config.Course, plan Plan, requireCloud bool) error {
	_, err := validateConfiguration(workspace, course, plan, requireCloud)
	return err
}

func validIssueType(value string) bool {
	return value == "Task" || value == "Session" || value == "Milestone"
}

func validateIssueKey(value, label string) error {
	if !planIssueRE.MatchString(value) {
		return &ValidationError{Message: fmt.Sprintf("%s has invalid syntax", label)}
	}
	return nil
}

func validateProjectKey(value, label string) error {
	if !planProjectRE.MatchString(value) {
		return &ValidationError{Message: fmt.Sprintf("%s has invalid syntax", label)}
	}
	return nil
}

func validateTransitionID(value string) error {
	if !planTransitionRE.MatchString(value) {
		return &ValidationError{Message: "Jira transition ID has invalid syntax"}
	}
	return nil
}

func validateDate(value, label string) error {
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil || parsed.Format("2006-01-02") != value {
		return &ValidationError{Message: label + " must be an ISO 8601 date"}
	}
	return nil
}

func stringSlice(value any, label string) ([]string, error) {
	switch list := value.(type) {
	case []string:
		return append([]string(nil), list...), nil
	case []any:
		result := make([]string, len(list))
		for i, item := range list {
			text, ok := item.(string)
			if !ok {
				return nil, &ValidationError{Message: label + " must be an array of strings"}
			}
			result[i] = text
		}
		return result, nil
	default:
		return nil, &ValidationError{Message: label + " must be an array of strings"}
	}
}
