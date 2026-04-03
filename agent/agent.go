package agent

import (
	"fmt"
	"strings"

	"github.com/this-is-alpha-iota/clyde/api"
	"github.com/this-is-alpha-iota/clyde/loglevel"
	"github.com/this-is-alpha-iota/clyde/tools"
)

// ProgressCallback receives progress messages during tool execution.
// The level parameter indicates the minimum log level at which this
// message should be displayed.
type ProgressCallback func(level loglevel.Level, message string)

// ErrorCallback receives errors during processing (optional, for logging)
type ErrorCallback func(err error)

// Agent handles conversation and tool execution
type Agent struct {
	apiClient        *api.Client
	systemPrompt     string
	history          []api.Message
	logLevel         loglevel.Level
	progressCallback ProgressCallback
	errorCallback    ErrorCallback
}

// AgentOption is a functional option for configuring an Agent
type AgentOption func(*Agent)

// WithLogLevel sets the log level for the agent
func WithLogLevel(level loglevel.Level) AgentOption {
	return func(a *Agent) {
		a.logLevel = level
	}
}

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
		logLevel:     loglevel.Normal, // Default
	}

	// Apply options
	for _, opt := range opts {
		opt(agent)
	}

	return agent
}

// LogLevel returns the agent's current log level
func (a *Agent) LogLevel() loglevel.Level {
	return a.logLevel
}

// emit sends a progress message if the agent's log level allows it.
// The threshold parameter indicates the minimum level required to see
// this message.
func (a *Agent) emit(threshold loglevel.Level, message string) {
	if a.progressCallback != nil && a.logLevel.ShouldShow(threshold) {
		a.progressCallback(threshold, message)
	}
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

		// Display cache hit information if available (Verbose and above)
		if resp.Usage.CacheReadInputTokens > 0 {
			totalInputTokens := resp.Usage.InputTokens + resp.Usage.CacheReadInputTokens
			cachePercentage := float64(resp.Usage.CacheReadInputTokens) / float64(totalInputTokens) * 100
			a.emit(loglevel.Verbose, fmt.Sprintf("💾 Cache hit: %d tokens (%.0f%% of input)",
				resp.Usage.CacheReadInputTokens, cachePercentage))
		}

		// Display debug diagnostics (Debug only)
		a.emit(loglevel.Debug, fmt.Sprintf("🔍 Tokens: input=%d output=%d cache_read=%d cache_create=%d",
			resp.Usage.InputTokens, resp.Usage.OutputTokens,
			resp.Usage.CacheReadInputTokens, resp.Usage.CacheCreationInputTokens))

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
		var pendingImages []api.ContentBlock

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

			// Display progress message (Quiet and above — the → lines)
			if reg.Display != nil {
				displayMsg := reg.Display(toolBlock.Input)
				if displayMsg != "" {
					a.emit(loglevel.Quiet, displayMsg)
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

				// Check for IMAGE_LOADED marker
				if strings.HasPrefix(output, "IMAGE_LOADED:") {
					// Parse: IMAGE_LOADED:<media_type>:<size_kb>:<base64_data>
					parts := strings.SplitN(output, ":", 4)
					if len(parts) == 4 {
						mediaType := parts[1]
						sizeKB := parts[2]
						imageData := parts[3]

						// Store image for inclusion in this turn's response
						pendingImages = append(pendingImages, api.ContentBlock{
							Type: "image",
							Source: &api.ImageSource{
								Type:      "base64",
								MediaType: mediaType,
								Data:      imageData,
							},
						})

						// Update result content to confirmation message
						resultContent = fmt.Sprintf("Image loaded successfully (%s, %s KB)", mediaType, sizeKB)
					}
				}
			}

			// Display tool output body (Normal and above)
			if resultContent != "" && !strings.HasPrefix(resultContent, "Image loaded") {
				a.emit(loglevel.Normal, resultContent)
			}

			toolResults = append(toolResults, api.ContentBlock{
				Type:      "tool_result",
				ToolUseID: toolBlock.ID,
				Content:   resultContent,
				IsError:   isError,
			})
		}

		// If we loaded any images, add them to the tool results
		if len(pendingImages) > 0 {
			toolResults = append(toolResults, pendingImages...)
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
