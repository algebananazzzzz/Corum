package config

import (
	"bytes"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

var (
	projectRE    = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)
	issueRE      = regexp.MustCompile(`^[A-Z][A-Z0-9_]*-[1-9][0-9]*$`)
	identifierRE = regexp.MustCompile(`^\S+$`)
	transitionRE = regexp.MustCompile(`^[0-9]+$`)
)

func bytesReader(data []byte) *bytes.Reader { return bytes.NewReader(data) }

func validateWorkspace(value Workspace) error {
	if value.Version != 2 {
		return fmt.Errorf("unsupported configuration version %d (only version 2 is supported)", value.Version)
	}
	if value.Workspace.Timezone == "" || value.Workspace.Term == "" {
		return fmt.Errorf("workspace timezone and term are required")
	}
	if value.Workspace.Timezone == "Local" {
		return fmt.Errorf("workspace timezone must be an IANA timezone")
	}
	if _, err := time.LoadLocation(value.Workspace.Timezone); err != nil {
		return fmt.Errorf("workspace timezone is not a valid IANA timezone: %w", err)
	}
	if value.Canvas != nil {
		if err := validateOrigin(value.Canvas.URL, "canvas url"); err != nil {
			return err
		}
	}
	if value.Jira != nil {
		if value.Jira.CloudID == "" || !identifierRE.MatchString(value.Jira.CloudID) {
			return fmt.Errorf("jira cloud_id must be a non-blank identifier")
		}
		if !projectRE.MatchString(value.Jira.Project) {
			return fmt.Errorf("jira project is invalid")
		}
		for name, id := range value.Jira.Transitions {
			if !identifierRE.MatchString(name) || !transitionRE.MatchString(id) {
				return fmt.Errorf("jira transition %q is invalid", name)
			}
		}
	}
	if err := safeRelativePath(value.Calendar.Timetable); err != nil {
		return fmt.Errorf("calendar timetable: %w", err)
	}
	if err := safeRelativePath(value.Calendar.Term); err != nil {
		return fmt.Errorf("calendar term: %w", err)
	}
	return nil
}

// ValidateWorkspace checks an in-memory v2 workspace before it is written.
func ValidateWorkspace(value Workspace) error { return validateWorkspace(value) }

func validateCourse(value Course) error {
	if value.Version != 2 {
		return fmt.Errorf("unsupported configuration version %d (only version 2 is supported)", value.Version)
	}
	if !identifierRE.MatchString(value.Code) {
		return fmt.Errorf("course code is invalid")
	}
	if value.Canvas != nil {
		if value.Canvas.ID <= 0 {
			return fmt.Errorf("canvas id must be positive")
		}
		seen := map[string]bool{}
		for _, source := range value.Canvas.Sources {
			if !validSource(source) || seen[source] {
				return fmt.Errorf("canvas source %q is invalid or duplicated", source)
			}
			seen[source] = true
		}
		for name, path := range value.Canvas.Folders {
			if strings.TrimSpace(name) == "" {
				return fmt.Errorf("canvas folder %q is invalid", name)
			}
			if err := safeRelativePath(path); err != nil {
				return fmt.Errorf("canvas folder %q: %w", name, err)
			}
		}
	}
	if value.Jira != nil && !issueRE.MatchString(value.Jira.Epic) {
		return fmt.Errorf("jira epic is invalid")
	}
	return nil
}

func validSource(value string) bool {
	switch value {
	case "announcements", "assignments", "files", "pages", "modules", "syllabus":
		return true
	default:
		return false
	}
}

func validateOrigin(value, label string) error {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("%s must be a credential-free HTTPS origin", label)
	}
	return nil
}

func safeRelativePath(value string) error {
	if value == "" || filepath.IsAbs(value) || filepath.VolumeName(value) != "" {
		return fmt.Errorf("must be a safe relative path")
	}
	for _, part := range strings.FieldsFunc(value, func(r rune) bool { return r == '/' || r == '\\' }) {
		if part == ".." {
			return fmt.Errorf("must not traverse parent directories")
		}
	}
	return nil
}

func rejectCredentialKeys(node *yaml.Node) error {
	if node.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(node.Content); i += 2 {
			key := node.Content[i].Value
			if credentialKey(key) {
				return fmt.Errorf("credential-like key %q is forbidden", key)
			}
			if err := rejectCredentialKeys(node.Content[i+1]); err != nil {
				return err
			}
		}
		return nil
	}
	for _, child := range node.Content {
		if err := rejectCredentialKeys(child); err != nil {
			return err
		}
	}
	return nil
}

func credentialKey(key string) bool {
	parts := strings.FieldsFunc(strings.ToLower(key), func(r rune) bool { return r == '_' || r == '-' })
	normalized := strings.Join(parts, "")
	for _, forbidden := range []string{"token", "password", "secret", "credential", "credentials", "authorization", "apikey", "accesstoken", "clientsecret", "canvastoken", "jiratoken"} {
		if normalized == forbidden {
			return true
		}
		for _, part := range parts {
			if part == forbidden {
				return true
			}
		}
	}
	return false
}
