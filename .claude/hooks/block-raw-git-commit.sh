#!/bin/bash
# PreToolUse hook that blocks raw "git commit" commands
# Forces use of /ccells-commit skill instead
#
# Hook type: PreToolUse (receives JSON on stdin)
# Exit 0 = allow, Exit non-zero = block
#
# Bypass: Create /tmp/.ccells-commit-active to allow commits
# (used by /ccells-commit skill itself)

# Read JSON input from stdin
input=$(cat)

# Extract tool name and command
tool_name=$(echo "$input" | jq -r '.tool_name // empty')
command=$(echo "$input" | jq -r '.tool_input.command // empty')

# Only check Bash tool calls
if [ "$tool_name" != "Bash" ]; then
    exit 0
fi

# Check for bypass file (set by /ccells-commit skill)
if [ -f /tmp/.ccells-commit-active ]; then
    exit 0
fi

# Check if this is a git commit command
# Match: git commit, git commit -m, git commit --amend, etc.
if echo "$command" | grep -qE '(^|\s|&&|\||;)git\s+commit(\s|$)'; then
    cat <<'EOF'
BLOCKED: Raw "git commit" is not allowed in ccells containers.

Use /ccells-commit instead. This skill:
- Handles CLAUDE.md updates automatically
- Runs pre-commit verification
- Provides proper commit message formatting

To commit, simply run: /ccells-commit
EOF
    exit 2
fi

exit 0
