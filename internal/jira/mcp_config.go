package jira

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const projectJiraMCPName = "atlassian-jira"

func codexConfigPath(root string) string  { return filepath.Join(root, ".codex", "config.toml") }
func claudeConfigPath(root string) string { return filepath.Join(root, ".mcp.json") }

// writeProjectMCPConfig installs the Jira MCP endpoint in both supported
// project-local clients without writing credentials.
func writeProjectMCPConfig(root string) error {
	if err := writeCodexMCPConfig(codexConfigPath(root)); err != nil {
		return fmt.Errorf("write Codex MCP config: %w", err)
	}
	if err := writeClaudeMCPConfig(claudeConfigPath(root)); err != nil {
		return fmt.Errorf("write Claude MCP config: %w", err)
	}
	return nil
}

// ConfigureProjectMCP installs Corum's project-local Jira MCP entries.
func ConfigureProjectMCP(root string) error { return writeProjectMCPConfig(root) }

func writeCodexMCPConfig(path string) error {
	data, err := os.ReadFile(path)
	if errorsIsNotExist(err) {
		data = nil
	} else if err != nil {
		return err
	}
	content := strings.TrimRight(string(data), "\n")
	content = removeTOMLTable(content, "mcp_servers."+projectJiraMCPName)
	if content != "" {
		content += "\n\n"
	}
	content += "[mcp_servers." + projectJiraMCPName + "]\nurl = \"" + rovoMCPURL + "\"\n"
	return atomicConfigWrite(path, []byte(content), 0o644)
}

func writeClaudeMCPConfig(path string) error {
	document := map[string]json.RawMessage{}
	data, err := os.ReadFile(path)
	if errorsIsNotExist(err) {
		document["mcpServers"] = json.RawMessage(`{}`)
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
	servers[projectJiraMCPName] = json.RawMessage(fmt.Sprintf(`{"type":"http","url":%q}`, rovoMCPURL))
	encodedServers, err := json.Marshal(servers)
	if err != nil {
		return err
	}
	document["mcpServers"] = encodedServers
	result, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return err
	}
	result = append(result, '\n')
	return atomicConfigWrite(path, result, 0o644)
}

func removeTOMLTable(content, table string) string {
	lines := strings.Split(content, "\n")
	var kept []string
	inTarget := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			inTarget = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(trimmed, "["), "]")) == table
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

func errorsIsNotExist(err error) bool { return err != nil && os.IsNotExist(err) }
