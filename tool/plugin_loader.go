package tool

import (
	"errors"
	"os"
	"path/filepath"
	"time"
)

const defaultPluginTimeout = 30 * time.Second

var (
	errPluginConflict      = errors.New("plugin tool conflicts with built-in tool")
	errReadingPluginDir    = errors.New("reading plugin directory")
)

// LoadPlugins discovers and loads all plugin tools from a directory.
// Returns the loaded tools and any errors encountered (non-fatal per plugin).
func LoadPlugins(dir string, timeout time.Duration) ([]Tool, []error) {
	if dir == "" {
		return nil, nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // Directory doesn't exist — no plugins
		}

		return nil, []error{errors.Join(errReadingPluginDir, err)}
	}

	if timeout == 0 {
		timeout = defaultPluginTimeout
	}

	var tools []Tool
	var errs []error

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		plugin, err := NewPluginTool(path, timeout)
		if err != nil {
			errs = append(errs, err)

			continue
		}

		tools = append(tools, plugin)
	}

	return tools, errs
}

// MergeTools combines built-in tools with plugin tools.
// Returns an error if any plugin name conflicts with a built-in tool.
func MergeTools(builtIn []Tool, plugins []Tool) ([]Tool, error) {
	names := make(map[string]bool)
	for _, t := range builtIn {
		names[t.Param().Name] = true
	}

	for _, p := range plugins {
		name := p.Param().Name
		if names[name] {
			return nil, errPluginConflict
		}

		names[name] = true
	}

	result := make([]Tool, 0, len(builtIn)+len(plugins))
	result = append(result, builtIn...)
	result = append(result, plugins...)

	return result, nil
}
