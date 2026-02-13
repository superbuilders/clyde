package tools

import (
	"claude-repl/api"
	"fmt"
)

// ExecutorFunc is a function that executes a tool
type ExecutorFunc func(input map[string]interface{}, apiClient *api.Client, conversationHistory []api.Message) (string, error)

// DisplayFunc is a function that formats a display message for a tool
type DisplayFunc func(input map[string]interface{}) string

// Registration holds a tool registration
type Registration struct {
	Tool     api.Tool
	Execute  ExecutorFunc
	Display  DisplayFunc
}

// Registry holds all registered tools
var Registry = make(map[string]*Registration)

// Register registers a tool with its executor and display functions
func Register(tool api.Tool, execute ExecutorFunc, display DisplayFunc) {
	Registry[tool.Name] = &Registration{
		Tool:    tool,
		Execute: execute,
		Display: display,
	}
}

// GetTool returns the tool registration for a given name
func GetTool(name string) (*Registration, error) {
	reg, ok := Registry[name]
	if !ok {
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
	return reg, nil
}

// GetAllTools returns all registered tools
func GetAllTools() []api.Tool {
	tools := make([]api.Tool, 0, len(Registry))
	for _, reg := range Registry {
		tools = append(tools, reg.Tool)
	}
	return tools
}
