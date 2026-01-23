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

# Check for bypass file first (set by /ccells-commit skill)
if [ -f /tmp/.ccells-commit-active ]; then
    exit 0
fi

# Extract command using grep/sed (more portable than jq)
# Look for "command": "..." pattern in JSON
command=$(echo "$input" | grep -o '"command"[[:space:]]*:[[:space:]]*"[^"]*"' | sed 's/.*"command"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')

# If no command found, allow (not a Bash tool call or different format)
if [ -z "$command" ]; then
    exit 0
fi

# Check if this is a git commit command
# Match: git commit, git commit -m, git commit --amend, etc.
if echo "$command" | grep -qE '(^|\s|&&|\||;)git\s+commit(\s|$)'; then
    cat >&2 <<'EOF'
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
