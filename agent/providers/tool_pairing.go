package providers

import "strings"

// toolPairingErrorMarkers are the substrings the Anthropic and Bedrock APIs
// use to report a broken tool_use/tool_result adjacency contract:
//
//	messages.8.content.0: unexpected `tool_use_id` found in `tool_result`
//	blocks: toolu_017oK28QHiFcjQWLt9L4z7L8.
//
//	messages.4: `tool_use` ids were found without `tool_result` blocks
//	immediately after
//
// This family is recoverable: the conversation on disk is fine, only the
// in-memory message list is malformed.
var toolPairingErrorMarkers = []string{
	"unexpected `tool_use_id` found in `tool_result` blocks",
	"unexpected tool_use_id found in tool_result blocks",
	"ids were found without `tool_result` blocks",
	"ids were found without tool_result blocks",
}

// ToolPairingRecoveryHint is appended to the user-facing message for the
// pairing-error family so the user knows the session is not lost.
const ToolPairingRecoveryHint = "  - This conversation's tool blocks got out of sync (usually after compaction).\n" +
	"    The session on disk is intact — re-run with `--resume` to recover it."

// IsToolPairingError reports whether err is a tool_use/tool_result pairing
// rejection from the provider.
func IsToolPairingError(err error) bool {
	if err == nil {
		return false
	}
	return containsToolPairingMarker(err.Error())
}

func containsToolPairingMarker(msg string) bool {
	for _, marker := range toolPairingErrorMarkers {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}
