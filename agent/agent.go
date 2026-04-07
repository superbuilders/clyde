package agent

import (
	"fmt"
	"strings"

	"github.com/this-is-alpha-iota/clyde/providers"
	"github.com/this-is-alpha-iota/clyde/loglevel"
	"github.com/this-is-alpha-iota/clyde/tools"
	"github.com/this-is-alpha-iota/clyde/agent/truncate"
)

// ProgressCallback receives progress messages during tool execution.
// The level parameter indicates the minimum log level at which this
// message should be displayed.
type ProgressCallback func(level loglevel.Level, message string)

// ThinkingCallback receives thinking trace text from Claude's extended thinking.
// The text is the raw thinking content; the caller is responsible for styling.
type ThinkingCallback func(text string)

// SpinnerCallback receives signals to start or stop the loading spinner.
// When start is true, message contains the operation text to display.
// When start is false, message is empty and the spinner should stop.
type SpinnerCallback func(start bool, message string)

// ErrorCallback receives errors during processing (optional, for logging)
type ErrorCallback func(err error)

// Agent handles conversation and tool execution
type Agent struct {
	apiClient         *providers.Client
	systemPrompt      string
	history           []providers.Message
	logLevel          loglevel.Level
	progressCallback  ProgressCallback
	thinkingCallback  ThinkingCallback
	spinnerCallback   SpinnerCallback
	errorCallback     ErrorCallback
	lastUsage         providers.Usage // Token usage from the most recent API response
	contextWindowSize int       // Model context window size in tokens (for debug display)
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

// WithThinkingCallback sets the callback for thinking trace display.
// The callback receives truncated (or full) thinking text based on log level.
func WithThinkingCallback(cb ThinkingCallback) AgentOption {
	return func(a *Agent) {
		a.thinkingCallback = cb
	}
}

// WithSpinnerCallback sets the spinner callback for starting/stopping
// the loading spinner during operations.
func WithSpinnerCallback(cb SpinnerCallback) AgentOption {
	return func(a *Agent) {
		a.spinnerCallback = cb
	}
}

// WithErrorCallback sets the error callback
func WithErrorCallback(cb ErrorCallback) AgentOption {
	return func(a *Agent) {
		a.errorCallback = cb
	}
}

// WithContextWindowSize sets the model's context window size in tokens.
// This is used for debug-level cache display to show context usage percentage.
func WithContextWindowSize(size int) AgentOption {
	return func(a *Agent) {
		a.contextWindowSize = size
	}
}

// NewAgent creates a new agent with optional configuration
func NewAgent(apiClient *providers.Client, systemPrompt string, opts ...AgentOption) *Agent {
	agent := &Agent{
		apiClient:    apiClient,
		systemPrompt: systemPrompt,
		history:      []providers.Message{},
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

// LastUsage returns the token usage from the most recent API response.
// Returns a zero-value Usage if no API call has been made yet.
func (a *Agent) LastUsage() providers.Usage {
	return a.lastUsage
}

// emit sends a progress message if the agent's log level allows it.
// The threshold parameter indicates the minimum level required to see
// this message.
func (a *Agent) emit(threshold loglevel.Level, message string) {
	if a.progressCallback != nil && a.logLevel.ShouldShow(threshold) {
		a.progressCallback(threshold, message)
	}
}

// emitThinking sends thinking trace text via the thinking callback.
// Thinking is displayed at Normal level (truncated) and above.
// Suppressed at Silent and Quiet.
func (a *Agent) emitThinking(text string) {
	if a.thinkingCallback == nil {
		return
	}
	if !a.logLevel.ShouldShow(loglevel.Normal) {
		return
	}

	// Apply truncation based on log level
	truncated := truncate.Thinking(text, a.logLevel)
	a.thinkingCallback(truncated)
}

// spinnerStart sends a start signal to the spinner callback if set.
func (a *Agent) spinnerStart(message string) {
	if a.spinnerCallback != nil && a.logLevel != loglevel.Silent {
		a.spinnerCallback(true, message)
	}
}

// spinnerStop sends a stop signal to the spinner callback if set.
func (a *Agent) spinnerStop() {
	if a.spinnerCallback != nil {
		a.spinnerCallback(false, "")
	}
}

// HandleMessage processes a user message and returns the response
func (a *Agent) HandleMessage(userInput string) (string, error) {
	// Add user message to history
	a.history = append(a.history, providers.Message{
		Role:    "user",
		Content: userInput,
	})

	// Get all registered tools
	allTools := tools.GetAllTools()

	// Conversation loop - continue until we get a text response
	for {
		// Start spinner while waiting for API response
		a.spinnerStart("Thinking...")

		resp, err := a.apiClient.Call(a.systemPrompt, a.history, allTools)

		// Stop spinner once API responds
		a.spinnerStop()

		if err != nil {
			return fmt.Sprintf("Error: %v", err), err
		}

		// Store usage for context tracking
		a.lastUsage = resp.Usage

		// Display cache information (Verbose and Debug only).
		// At Normal/Quiet/Silent, cache info is suppressed — the context
		// window percentage on the prompt line serves as the primary
		// "how full is my context?" indicator.
		if resp.Usage.CacheReadInputTokens > 0 {
			totalInputTokens := resp.Usage.InputTokens + resp.Usage.CacheReadInputTokens

			// Verbose: token fraction format
			a.emit(loglevel.Verbose, fmt.Sprintf("💾 Cache: %d/%d tokens",
				resp.Usage.CacheReadInputTokens, totalInputTokens))

			// Debug: detailed format with creation tokens and context %
			if a.logLevel.ShouldShow(loglevel.Debug) {
				detail := fmt.Sprintf("💾 Cache: %d/%d tokens | Creation: %d tokens",
					resp.Usage.CacheReadInputTokens, totalInputTokens,
					resp.Usage.CacheCreationInputTokens)
				if a.contextWindowSize > 0 {
					pct := (totalInputTokens * 100) / a.contextWindowSize
					if pct > 100 {
						pct = 100
					}
					detail += fmt.Sprintf(" | Context: %d%% (%d/%d)",
						pct, totalInputTokens, a.contextWindowSize)
				}
				a.emit(loglevel.Debug, detail)
			}
		}

		// Display debug diagnostics (Debug only)
		a.emit(loglevel.Debug, fmt.Sprintf("🔍 Tokens: input=%d output=%d cache_read=%d cache_create=%d",
			resp.Usage.InputTokens, resp.Usage.OutputTokens,
			resp.Usage.CacheReadInputTokens, resp.Usage.CacheCreationInputTokens))

		var assistantContent []providers.ContentBlock
		var textResponses []string
		var toolUseBlocks []providers.ContentBlock

		for _, block := range resp.Content {
			// Ensure tool_use blocks always have a non-nil Input map.
			// The API requires "input" to be present for tool_use blocks.
			// Our MarshalJSON on ContentBlock handles serialization, but
			// we also set it here for in-memory consistency.
			if block.Type == "tool_use" && block.Input == nil {
				block.Input = map[string]interface{}{}
			}
			assistantContent = append(assistantContent, block)

			switch block.Type {
			case "text":
				if block.Text != "" {
					textResponses = append(textResponses, block.Text)
				}
			case "tool_use":
				toolUseBlocks = append(toolUseBlocks, block)
			case "thinking":
				// Display thinking trace
				if block.Thinking != "" {
					a.emitThinking(block.Thinking)
				}
			case "redacted_thinking":
				// Redacted thinking — note it but don't display content
				a.emit(loglevel.Debug, "🔒 Redacted thinking block (encrypted by safety system)")
			}
		}

		// Add assistant response to history (includes thinking blocks
		// for proper round-tripping as required by the API)
		a.history = append(a.history, providers.Message{
			Role:    "assistant",
			Content: assistantContent,
		})

		// If no tool use, return text responses
		if len(toolUseBlocks) == 0 {
			return strings.Join(textResponses, "\n"), nil
		}

		// Execute tools
		var toolResults []providers.ContentBlock
		var pendingImages []providers.ContentBlock

		for _, toolBlock := range toolUseBlocks {
			reg, err := tools.GetTool(toolBlock.Name)
			if err != nil {
				// Unknown tool
				toolResults = append(toolResults, providers.ContentBlock{
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
						pendingImages = append(pendingImages, providers.ContentBlock{
							Type: "image",
							Source: &providers.ImageSource{
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

			// Display tool output body (Normal and above).
			// Apply truncation based on log level: 25-line limit at Normal,
			// full output at Verbose/Debug.
			if resultContent != "" && !strings.HasPrefix(resultContent, "Image loaded") {
				truncatedOutput := truncate.ToolOutput(resultContent, a.logLevel)
				a.emit(loglevel.Normal, truncatedOutput)
			}

			toolResults = append(toolResults, providers.ContentBlock{
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
		a.history = append(a.history, providers.Message{
			Role:    "user",
			Content: toolResults,
		})
	}
}

// GetHistory returns the conversation history
func (a *Agent) GetHistory() []providers.Message {
	return a.history
}
