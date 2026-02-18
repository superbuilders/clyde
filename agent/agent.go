package agent

import (
	"github.com/this-is-alpha-iota/clyde/api"
	"github.com/this-is-alpha-iota/clyde/tools"
	"fmt"
	"strings"
)

// ProgressCallback receives progress messages during tool execution
type ProgressCallback func(message string)

// ErrorCallback receives errors during processing (optional, for logging)
type ErrorCallback func(err error)

// Agent handles conversation and tool execution
type Agent struct {
	apiClient        *api.Client
	systemPrompt     string
	history          []api.Message
	progressCallback ProgressCallback
	errorCallback    ErrorCallback
}

// AgentOption is a functional option for configuring an Agent
type AgentOption func(*Agent)

// WithProgressCallback sets the progress callback
func WithProgressCallback(cb ProgressCallback) AgentOption {
	return func(a *Agent) {
		a.progressCallback = cb
	}
}

// WithErrorCallback sets the error callback
func WithErrorCallback(cb ErrorCallback) AgentOption {
	return func(a *Agent) {
		a.errorCallback = cb
	}
}

// NewAgent creates a new agent with optional configuration
func NewAgent(apiClient *api.Client, systemPrompt string, opts ...AgentOption) *Agent {
	agent := &Agent{
		apiClient:    apiClient,
		systemPrompt: systemPrompt,
		history:      []api.Message{},
	}
	
	// Apply options
	for _, opt := range opts {
		opt(agent)
	}
	
	return agent
}

// HandleMessage processes a user message and returns the response
func (a *Agent) HandleMessage(userInput string) (string, error) {
	// Add user message to history
	a.history = append(a.history, api.Message{
		Role:    "user",
		Content: userInput,
	})

	// Get all registered tools
	allTools := tools.GetAllTools()

	// Conversation loop - continue until we get a text response
	for {
		resp, err := a.apiClient.Call(a.systemPrompt, a.history, allTools)
		if err != nil {
			return fmt.Sprintf("Error: %v", err), err
		}

		var assistantContent []api.ContentBlock
		var textResponses []string
		var toolUseBlocks []api.ContentBlock

		for _, block := range resp.Content {
			assistantContent = append(assistantContent, block)

			if block.Type == "text" && block.Text != "" {
				textResponses = append(textResponses, block.Text)
			} else if block.Type == "tool_use" {
				toolUseBlocks = append(toolUseBlocks, block)
			}
		}

		// Add assistant response to history
		a.history = append(a.history, api.Message{
			Role:    "assistant",
			Content: assistantContent,
		})

		// If no tool use, return text responses
		if len(toolUseBlocks) == 0 {
			return strings.Join(textResponses, "\n"), nil
		}

		// Execute tools
		var toolResults []api.ContentBlock
		for _, toolBlock := range toolUseBlocks {
			reg, err := tools.GetTool(toolBlock.Name)
			if err != nil {
				// Unknown tool
				toolResults = append(toolResults, api.ContentBlock{
					Type:      "tool_result",
					ToolUseID: toolBlock.ID,
					Content:   err.Error(),
					IsError:   true,
				})
				continue
			}

			// Display progress message
			if reg.Display != nil {
				displayMsg := reg.Display(toolBlock.Input)
				if displayMsg != "" && a.progressCallback != nil {
					a.progressCallback(displayMsg)
				}
			}

			// Execute the tool
			output, err := reg.Execute(toolBlock.Input, a.apiClient, a.history)

			var resultContent string
			var isError bool
			if err != nil {
				resultContent = err.Error()
				isError = true
			} else {
				resultContent = output
				isError = false
			}

			toolResults = append(toolResults, api.ContentBlock{
				Type:      "tool_result",
				ToolUseID: toolBlock.ID,
				Content:   resultContent,
				IsError:   isError,
			})
		}

		// Add tool results to history
		a.history = append(a.history, api.Message{
			Role:    "user",
			Content: toolResults,
		})
	}
}

// GetHistory returns the conversation history
func (a *Agent) GetHistory() []api.Message {
	return a.history
}
