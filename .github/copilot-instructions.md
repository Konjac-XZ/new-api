### CRITICAL WARNING: Using mcp-feedback-enhanced

Invoke `mcp-feedback-enhanced` to solicit user feedback and obtain permission or instructions for the next steps in scenarios including, but not limited to, the following:

- When you believe you have completed the task (do not end the conversation yourself without the user indicating "task completed" via this tool).
- When you believe the user's instructions are ambiguous and require more information from the user to continue the task.
- When you need additional help from the user (e.g., requiring third-party dependency tools).

### Special Note: Personal Branch

All modifications made by the current user to the program **will not** be merged into the mainline; they are written solely for personal convenience.

Therefore, the following steps are unnecessary:

- Internationalization beyond Chinese and English
- Unit testing
- Other heavyweight testing or reliability verification

At the same time, if a modification involves multiple types of channels, generally only the following types of channels need to be handled:

- OpenAI (including traditional Completion and modern Response types)
- Anthropic Claude
- Google Gemini

Users generally do not use other types of channels.