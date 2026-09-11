package google

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
)

// Command reuses an installed CLI, or obtains a pinned release through npm.
// No global package installation or shell interpolation is required.
func Command(ctx context.Context, args ...string) *exec.Cmd {
	if binary := os.Getenv("CORUM_GWS_BIN"); binary != "" {
		return exec.CommandContext(ctx, binary, args...)
	}
	if binary, err := exec.LookPath("gws"); err == nil {
		return exec.CommandContext(ctx, binary, args...)
	}
	return exec.CommandContext(ctx, "npx", append([]string{"--yes", "@googleworkspace/cli@0.22.5"}, args...)...)
}

func RunInteractive(ctx context.Context, in io.Reader, out io.Writer, args ...string) error {
	cmd := Command(ctx, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = in, out, out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("Google setup did not complete: %w", err)
	}
	return nil
}

func JSON(ctx context.Context, resource, method string, params, body any, result any) error {
	args := []string{"tasks", resource, method}
	if params != nil {
		data, err := json.Marshal(params)
		if err != nil {
			return err
		}
		args = append(args, "--params", string(data))
	}
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		args = append(args, "--json", string(data))
	}
	data, err := Command(ctx, args...).Output()
	if err != nil {
		return fmt.Errorf("Google Tasks request failed; check connection and API access in Task tracker settings: %w", err)
	}
	if err := json.Unmarshal(data, result); err != nil {
		return fmt.Errorf("invalid Google Tasks response: %w", err)
	}
	return nil
}

type List struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

func Lists(ctx context.Context) ([]List, error) {
	result := []List{}
	seen := map[string]bool{}
	token := ""
	for {
		params := map[string]any{"maxResults": 100}
		if token != "" {
			params["pageToken"] = token
		}
		var page struct {
			Items []List `json:"items"`
			Next  string `json:"nextPageToken"`
		}
		if err := JSON(ctx, "tasklists", "list", params, nil, &page); err != nil {
			return nil, err
		}
		result = append(result, page.Items...)
		if page.Next == "" {
			return result, nil
		}
		if seen[page.Next] {
			return nil, fmt.Errorf("Google Tasks returned repeated pagination")
		}
		seen[page.Next] = true
		token = page.Next
	}
}

func CreateList(ctx context.Context, title string) (List, error) {
	var result List
	err := JSON(ctx, "tasklists", "insert", nil, map[string]string{"title": title}, &result)
	if err == nil && result.ID == "" {
		err = fmt.Errorf("Google did not return the created list ID")
	}
	return result, err
}
