package tools

import (
	"github.com/this-is-alpha-iota/clyde/providers"
	"fmt"
)

// ExecutorFunc is a function that executes a tool
type ExecutorFunc func(input map[string]interface{}, apiClient *providers.Client, conversationHistory []providers.Message) (string, error)

// DisplayFunc is a function that formats a display message for a tool
type DisplayFunc func(input map[string]interface{}) string

// Registration holds a tool registration
type Registration struct {
	Tool     providers.Tool
	Execute  ExecutorFunc
	Display  DisplayFunc
}

// Registry holds all registered tools
var Registry = make(map[string]*Registration)

// Register registers a tool with its executor and display functions
func Register(tool providers.Tool, execute ExecutorFunc, display DisplayFunc) {
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
func GetAllTools() []providers.Tool {
	tools := make([]providers.Tool, 0, len(Registry))
	for _, reg := range Registry {
		tools = append(tools, reg.Tool)
	}
	return tools
}
