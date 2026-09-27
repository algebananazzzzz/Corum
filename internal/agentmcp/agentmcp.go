// Package agentmcp installs the task tracker's MCP server in project-local
// agent client configuration. Corum never authenticates to trackers itself;
// Codex and Claude sign in when an agent first uses the entry.
package agentmcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/algebananazzzzz/Corum/internal/config"
)

const (
	jiraServer  = "atlassian-jira"
	kaneoServer = "kaneo"
	jiraMCPURL  = "https://mcp.atlassian.com/v2/mcp"
)

// managedServers lists every entry Corum may own, so switching trackers
// removes the previous tracker's entry without touching user entries.
var managedServers = []string{jiraServer, kaneoServer}

func codexConfigPath(root string) string  { return filepath.Join(root, ".codex", "config.toml") }
func claudeConfigPath(root string) string { return filepath.Join(root, ".mcp.json") }

// Configure makes the project's agent clients match the selected tracker.
// Google Tasks has no MCP server; agents use the gws CLI for it.
func Configure(root string, workspace config.Workspace) error {
	name, url := "", ""
	switch workspace.TaskTracker {
	case "jira":
		name, url = jiraServer, jiraMCPURL
	case "kaneo":
		if workspace.Kaneo == nil {
			return fmt.Errorf("kaneo settings are required")
		}
		name, url = kaneoServer, strings.TrimSuffix(workspace.Kaneo.URL, "/")+"/api/mcp"
	}
	if err := writeCodexMCPConfig(codexConfigPath(root), name, url); err != nil {
		return fmt.Errorf("write Codex MCP config: %w", err)
	}
	if err := writeClaudeMCPConfig(claudeConfigPath(root), name, url); err != nil {
		return fmt.Errorf("write Claude MCP config: %w", err)
	}
	return nil
}

// writeCodexMCPConfig removes managed tables and appends the selected one.
// An empty name only removes; an absent file stays absent.
func writeCodexMCPConfig(path, name, url string) error {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		if name == "" {
			return nil
		}
		data = nil
	} else if err != nil {
		return err
	}
	original := strings.TrimRight(string(data), "\n")
	content := original
	for _, server := range managedServers {
		content = removeTOMLTables(content, "mcp_servers."+server)
	}
	if name != "" {
		if content != "" {
			content += "\n\n"
		}
		content += "[mcp_servers." + name + "]\nurl = \"" + url + "\"\n"
	} else if content == original {
		return nil
	} else if content != "" {
		content += "\n"
	}
	return atomicConfigWrite(path, []byte(content), 0o644)
}

func writeClaudeMCPConfig(path, name, url string) error {
	document := map[string]json.RawMessage{}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		if name == "" {
			return nil
		}
	} else if err != nil {
		return err
	} else if err := json.Unmarshal(data, &document); err != nil {
		return err
	}
	servers := map[string]json.RawMessage{}
	if raw, ok := document["mcpServers"]; ok && len(raw) > 0 {
		if err := json.Unmarshal(raw, &servers); err != nil {
			return fmt.Errorf("mcpServers must be an object: %w", err)
		}
	}
	changed := false
	for _, server := range managedServers {
		if _, ok := servers[server]; ok {
			delete(servers, server)
			changed = true
		}
	}
	if name != "" {
		servers[name] = json.RawMessage(fmt.Sprintf(`{"type":"http","url":%q}`, url))
		changed = true
	}
	if !changed {
		return nil
	}
	encodedServers, err := json.Marshal(servers)
	if err != nil {
		return err
	}
	document["mcpServers"] = encodedServers
	result, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return err
	}
	return atomicConfigWrite(path, append(result, '\n'), 0o644)
}

// removeTOMLTables drops a table and its subtables, such as custom headers
// a user added under the managed server.
func removeTOMLTables(content, table string) string {
	lines := strings.Split(content, "\n")
	var kept []string
	inTarget := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			name := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(trimmed, "["), "]"))
			inTarget = name == table || strings.HasPrefix(name, table+".")
		}
		if !inTarget {
			kept = append(kept, line)
		}
	}
	return strings.TrimRight(strings.Join(kept, "\n"), "\n")
}

func atomicConfigWrite(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".corum-mcp-*")
	if err != nil {
		return err
	}
	temporary := file.Name()
	defer os.Remove(temporary)
	if err := file.Chmod(mode); err != nil {
		file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}
