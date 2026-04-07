package agent

import (
	"fmt"
	"strings"

	"github.com/this-is-alpha-iota/clyde/providers"
	"github.com/this-is-alpha-iota/clyde/tools"
)

// ProgressCallback receives tool progress lines (the → lines).
// Called unconditionally for every tool execution.
type ProgressCallback func(message string)

// OutputCallback receives tool output bodies (full, untruncated text).
// Called unconditionally; the caller is responsible for truncation.
type OutputCallback func(output string)

// ThinkingCallback receives thinking trace text from Claude's extended thinking.
// The text is the raw, full thinking content; the caller is responsible for
// truncation and styling.
type ThinkingCallback func(text string)

// DiagnosticCallback receives diagnostic information (cache stats, token counts, etc.).
// Called unconditionally; the caller decides whether to display.
type DiagnosticCallback func(message string)

// SpinnerCallback receives signals to start or stop the loading spinner.
// When start is true, message contains the operation text to display.
// When start is false, message is empty and the spinner should stop.
// Called unconditionally; the caller decides whether to act.
type SpinnerCallback func(start bool, message string)

// ErrorCallback receives errors during processing (optional, for logging)
type ErrorCallback func(err error)

// Agent handles conversation and tool execution
type Agent struct {
	apiClient          *providers.Client
	systemPrompt       string
	history            []providers.Message
	progressCallback   ProgressCallback
	outputCallback     OutputCallback
	thinkingCallback   ThinkingCallback
	diagnosticCallback DiagnosticCallback
	spinnerCallback    SpinnerCallback
	errorCallback      ErrorCallback
	lastUsage          providers.Usage // Token usage from the most recent API response
	contextWindowSize  int             // Model context window size in tokens (for diagnostic display)
}

// AgentOption is a functional option for configuring an Agent
type AgentOption func(*Agent)

// WithProgressCallback sets the callback for tool progress lines (→ lines).
func WithProgressCallback(cb ProgressCallback) AgentOption {
	return func(a *Agent) {
		a.progressCallback = cb
	}
}

// WithOutputCallback sets the callback for tool output bodies (full text).
func WithOutputCallback(cb OutputCallback) AgentOption {
	return func(a *Agent) {
		a.outputCallback = cb
	}
}

// WithThinkingCallback sets the callback for thinking trace display.
// The callback receives the full, untruncated thinking text.
func WithThinkingCallback(cb ThinkingCallback) AgentOption {
	return func(a *Agent) {
		a.thinkingCallback = cb
	}
}

// WithDiagnosticCallback sets the callback for diagnostic messages
// (cache stats, token counts, redacted thinking notes, etc.).
func WithDiagnosticCallback(cb DiagnosticCallback) AgentOption {
	return func(a *Agent) {
		a.diagnosticCallback = cb
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
// This is used for diagnostic display to show context usage percentage.
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
	}

	// Apply options
	for _, opt := range opts {
		opt(agent)
	}

	return agent
}

// LastUsage returns the token usage from the most recent API response.
// Returns a zero-value Usage if no API call has been made yet.
func (a *Agent) LastUsage() providers.Usage {
	return a.lastUsage
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
		if a.spinnerCallback != nil {
			a.spinnerCallback(true, "Thinking...")
		}

		resp, err := a.apiClient.Call(a.systemPrompt, a.history, allTools)

		// Stop spinner once API responds
		if a.spinnerCallback != nil {
			a.spinnerCallback(false, "")
		}

		if err != nil {
			return fmt.Sprintf("Error: %v", err), err
		}

		// Store usage for context tracking
		a.lastUsage = resp.Usage

		// Emit cache and diagnostic information unconditionally.
		// The CLI layer filters based on its own log level.
		if resp.Usage.CacheReadInputTokens > 0 && a.diagnosticCallback != nil {
			totalInputTokens := resp.Usage.InputTokens + resp.Usage.CacheReadInputTokens

			// Cache token fraction
			a.diagnosticCallback(fmt.Sprintf("💾 Cache: %d/%d tokens",
				resp.Usage.CacheReadInputTokens, totalInputTokens))

			// Detailed cache info with creation tokens and context %
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
			a.diagnosticCallback(detail)
		}

		// Token usage diagnostics
		if a.diagnosticCallback != nil {
			a.diagnosticCallback(fmt.Sprintf("🔍 Tokens: input=%d output=%d cache_read=%d cache_create=%d",
				resp.Usage.InputTokens, resp.Usage.OutputTokens,
				resp.Usage.CacheReadInputTokens, resp.Usage.CacheCreationInputTokens))
		}

		var assistantContent []providers.ContentBlock
		var textResponses []string
		var toolUseBlocks []providers.ContentBlock

		for _, block := range resp.Content {
			// Ensure tool_use blocks always have a non-nil Input map.
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
				// Emit full thinking trace unconditionally
				if block.Thinking != "" && a.thinkingCallback != nil {
					a.thinkingCallback(block.Thinking)
				}
			case "redacted_thinking":
				// Redacted thinking — note it via diagnostics
				if a.diagnosticCallback != nil {
					a.diagnosticCallback("🔒 Redacted thinking block (encrypted by safety system)")
				}
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

			// Emit progress message unconditionally (the → lines)
			if reg.Display != nil && a.progressCallback != nil {
				displayMsg := reg.Display(toolBlock.Input)
				if displayMsg != "" {
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

			// Emit tool output body unconditionally (full, untruncated).
			// The CLI layer handles truncation and display filtering.
			if resultContent != "" && !strings.HasPrefix(resultContent, "Image loaded") {
				if a.outputCallback != nil {
					a.outputCallback(resultContent)
				}
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
