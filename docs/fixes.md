Agent fixes:
- [ ] Tool calls that flood the context
  - although we have automatic compaction over a certain threshold, when a tool call brings back a lot of data and it, by itself is larger than the context window (or even the remaining context window), the harness tries to send it in the subsequent LLM call and the loop crashes; instead, we should be able to detect this ahead of time and replace the tool result with an appropriate error message *before* sending, so that the receiving LLM understands what happened and can either adjust their query or figure out how to write the same result to file for later greping (in the case of bash commands the LLM may know how to do this), that way the harness never crashes for context window reasons

- [ ] System prompt for head, tail, count etc
  - to mitigate the issue above, we have been including text in our prompts warning the llm about the failure mode and suggesting use of head/tail/count type tools, that should also be in the system prompt to avoid the failure case above even when it is recoverable
  - example: "{do thing}; be careful pulling too much data from tool calls because the session will crash if the context window is overloaded - use count, head, and tail as necessary"

- [ ] Remove progress.md from system prompt
  - we should not specify specific project managment files by name otherwise the harness will start creating them, unprompted
- [ ] .png files should get saved to gitignored folder by default
  - clyde keeps taking screenshots with playwright and then commiting them to repositories that it works on - we need to prevent this either in code or in the system prompt

Session Viewer
- [ ] Save entered text when changing tabs
  - localstorage should be used to preserve text state in the text entry area so tht it persists when it is navigated away from for any reason; it should be scoped to chat though
- [ ] Image input
  - there should be an image input next to the text entry that lets users upload images, "uploaded" images should be placed in the root of the project in question and the text "include ./image-name.ext\n" should be prepended to the next prompt so that clyde knows to use the image tool to include the "uploaded" image
- [ ] Hidden/collapse/expand toggle 
  - instead of visible/invisible, the buttons at the top right of the chat should toggle through Hidden/collapse/expand, where
    - hidden: everything except debug info *shows* as 'n tool calls' '1 thinking trace' etc
    - collapsed: same as now (n lines per n tool calls) but "collapse all"; user can still uncollapse individual lines
    - expand: same as now, but collapsed sections should get expanded when toggled back to this as well; user can still collapse individual lines
  - also, user and agent output should never be hidden, those buttons can be removed
- [ ] improved visibility through colors:
  - the user input and the agent output should have 'You:\n' and 'Clyde:\n' prepended before they are drawn, 
  - also those labels should have colors so that when scanning large sections it is easy to find them
  - also, thinking traces and tool call/results, when expanded should have unique colors 
  - (see the clyde TUI for what colors to use)
  - 

Tui fixes
- [ ] Back cursor movement with multiple lines
    - when we use multiple lines in the text entry and then go back so far that we go to a previous line, the redraw function appears to "delete" 1 to many lines - that is to say, the last line of output *above* the text entry prompt gets deleted upon every subsequent backcurosr (e.g. type 3 lines, move to the beginning of the 3rd line => works great, press back from the beginning of the 3rd line => moves to end of line 2 and deletes the last output line, press back 5 more times => now we are on the len-5 char of line 2 and the last 6 output lines have been deleted from the terminal)
- [ ] Redraw chat on -r
    - when we resume a chat with -r the entire previous message history shgould be output to the terminal with verbosity level respected
- [ ] Redraw chat on verbosity change
    - just like when we resume a chat, we should be able to use a command to change verbosity in the TUI; it should be imlemented so that "/verbosity <level>" has the same effect as: killing the chat, then calling 'clyde -r' with the new verbosity level passed in (*including* that the entire chat gets reemitted in the new verbosity)
