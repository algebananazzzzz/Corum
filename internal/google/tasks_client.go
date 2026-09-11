package google

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
)

type CommandClient struct {
	Binary string
}

func NewCommandClient(binary string) *CommandClient {
	if binary == "" {
		binary = "gws"
	}
	return &CommandClient{Binary: binary}
}

func (c *CommandClient) ListTasks(ctx context.Context, listID string) ([]map[string]any, error) {
	params, err := json.Marshal(map[string]any{
		"tasklist":      listID,
		"showCompleted": true,
		"showHidden":    true,
		"maxResults":    100,
	})
	if err != nil {
		return nil, err
	}
	args := []string{"tasks", "tasks", "list", "--params", string(params), "--page-all"}
	command := Command(ctx, args...)
	if c.Binary != "" && c.Binary != "gws" {
		command = exec.CommandContext(ctx, c.Binary, args...)
	}
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		message := stderr.String()
		if message == "" {
			message = err.Error()
		}
		return nil, fmt.Errorf("gws tasks list failed: %s", message)
	}
	return decodeTaskPages(stdout.Bytes())
}

func decodeTaskPages(data []byte) ([]map[string]any, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	var tasks []map[string]any
	pages := 0
	next := ""
	for {
		var page struct {
			Kind  string           `json:"kind"`
			Next  string           `json:"nextPageToken"`
			Items []map[string]any `json:"items"`
		}
		if err := decoder.Decode(&page); err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("invalid gws task response: %w", err)
		}
		if page.Kind != "tasks#tasks" {
			return nil, fmt.Errorf("invalid Google Tasks list response")
		}
		pages++
		next = page.Next
		tasks = append(tasks, page.Items...)
	}
	if pages == 0 || next != "" {
		return nil, fmt.Errorf("incomplete Google Tasks response; previous cache preserved")
	}
	if tasks == nil {
		tasks = []map[string]any{}
	}
	return tasks, nil
}

func DefaultCommandClient() *CommandClient {
	if binary := os.Getenv("CORUM_GWS_BIN"); binary != "" {
		return NewCommandClient(binary)
	}
	return NewCommandClient("gws")
}
