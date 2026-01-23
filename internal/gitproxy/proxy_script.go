package gitproxy

// ProxyScript is the shell script that runs inside containers to communicate
// with the git proxy socket. It intercepts git/gh commands and forwards them
// to the host for execution.
const ProxyScript = `#!/bin/bash
# ccells-git-proxy: Proxies git/gh commands to the host via Unix socket
# This script is called by Claude Code hooks when git/gh commands are detected.

SOCKET_PATH="/var/run/ccells/git.sock"

# Build the operation name from command and subcommand
build_operation() {
    local cmd="$1"
    shift

    case "$cmd" in
        git)
            local subcmd="$1"
            case "$subcmd" in
                fetch) echo "git-fetch" ;;
                pull)  echo "git-pull" ;;
                push)  echo "git-push" ;;
                remote)
                    echo "ERROR: git remote commands are blocked" >&2
                    exit 1
                    ;;
                *)
                    echo "ERROR: git $subcmd is not proxied" >&2
                    exit 1
                    ;;
            esac
            ;;
        gh)
            local subcmd="$1 $2"
            case "$subcmd" in
                "pr view")    echo "gh-pr-view" ;;
                "pr checks")  echo "gh-pr-checks" ;;
                "pr diff")    echo "gh-pr-diff" ;;
                "pr list")    echo "gh-pr-list" ;;
                "pr create")  echo "gh-pr-create" ;;
                "pr merge")   echo "gh-pr-merge" ;;
                "issue view") echo "gh-issue-view" ;;
                "issue list") echo "gh-issue-list" ;;
                *)
                    echo "ERROR: gh $subcmd is not proxied" >&2
                    exit 1
                    ;;
            esac
            ;;
        *)
            echo "ERROR: $cmd is not a proxied command" >&2
            exit 1
            ;;
    esac
}

# Build JSON array of remaining args
build_args_json() {
    local cmd="$1"
    shift

    # Skip the subcommand(s) to get remaining args
    case "$cmd" in
        git)
            shift  # Skip git subcommand (fetch/pull/push)
            ;;
        gh)
            shift  # Skip gh subcommand (pr/issue)
            shift  # Skip gh sub-subcommand (view/create/etc)
            ;;
    esac

    # Build JSON array
    local json="["
    local first=true
    for arg in "$@"; do
        if [ "$first" = true ]; then
            first=false
        else
            json="$json,"
        fi
        # Escape special characters in JSON string
        # Order matters: escape backslash first, then other special chars
        local escaped=$(printf '%s' "$arg" | sed -e 's/\\/\\\\/g' \
            -e 's/"/\\"/g' \
            -e 's/	/\\t/g' \
            -e ':a' -e 'N' -e '$!ba' -e 's/\n/\\n/g' \
            -e 's/\r/\\r/g')
        json="$json\"$escaped\""
    done
    json="$json]"
    echo "$json"
}

# Main
if [ ! -S "$SOCKET_PATH" ]; then
    echo "ERROR: Git proxy socket not found at $SOCKET_PATH" >&2
    echo "This command must be run inside a ccells container." >&2
    exit 1
fi

# First argument is the command (git or gh)
CMD="$1"
if [ -z "$CMD" ]; then
    echo "Usage: ccells-git-proxy <git|gh> [args...]" >&2
    exit 1
fi

# Get operation
OPERATION=$(build_operation "$@")
if [ $? -ne 0 ]; then
    exit 1
fi

# Get args as JSON
ARGS_JSON=$(build_args_json "$@")

# Build request JSON
REQUEST="{\"operation\":\"$OPERATION\",\"args\":$ARGS_JSON}"

# Send request and read response using nc (netcat)
RESPONSE=$(echo "$REQUEST" | nc -U "$SOCKET_PATH" 2>/dev/null)

if [ -z "$RESPONSE" ]; then
    echo "ERROR: No response from git proxy" >&2
    exit 1
fi

# Parse response using jq if available, otherwise use python
if command -v jq &> /dev/null; then
    EXIT_CODE=$(echo "$RESPONSE" | jq -r '.exit_code // 1')
    STDOUT=$(echo "$RESPONSE" | jq -r '.stdout // ""')
    STDERR=$(echo "$RESPONSE" | jq -r '.stderr // ""')
    ERROR=$(echo "$RESPONSE" | jq -r '.error // ""')
elif command -v python3 &> /dev/null; then
    # Use python for robust JSON parsing when jq is not available
    # Output is NUL-delimited to handle multiline values correctly
    PARSED=$(echo "$RESPONSE" | python3 -c '
import sys, json
try:
    r = json.loads(sys.stdin.read())
    sys.stdout.write(str(r.get("exit_code", 1)))
    sys.stdout.write("\0")
    sys.stdout.write(r.get("stdout", "") or "")
    sys.stdout.write("\0")
    sys.stdout.write(r.get("stderr", "") or "")
    sys.stdout.write("\0")
    sys.stdout.write(r.get("error", "") or "")
except:
    sys.stdout.write("1\0\0\0Failed to parse response")
')
    # Read NUL-delimited fields
    IFS= read -r -d '' EXIT_CODE <<< "$PARSED" || true
    PARSED="${PARSED#*$'\0'}"
    IFS= read -r -d '' STDOUT <<< "$PARSED" || true
    PARSED="${PARSED#*$'\0'}"
    IFS= read -r -d '' STDERR <<< "$PARSED" || true
    PARSED="${PARSED#*$'\0'}"
    ERROR="$PARSED"
else
    # Last resort: basic extraction (may fail on escaped quotes)
    EXIT_CODE=$(echo "$RESPONSE" | grep -o '"exit_code":[0-9]*' | cut -d: -f2)
    [ -z "$EXIT_CODE" ] && EXIT_CODE=1
    STDOUT=""
    STDERR=""
    ERROR="JSON parsing unavailable (install jq or python3)"
fi

# Output results
if [ -n "$STDOUT" ]; then
    echo -n "$STDOUT"
fi

if [ -n "$STDERR" ]; then
    echo -n "$STDERR" >&2
fi

if [ -n "$ERROR" ]; then
    echo "ERROR: $ERROR" >&2
fi

exit $EXIT_CODE
`

// GitHookScript is a PreToolUse hook script that intercepts git and gh commands.
// It receives JSON on stdin with the bash command, checks if it matches git/gh patterns,
// and either:
// - Runs the command through the proxy and exits 2 (for proxied commands)
// - Exits 2 with an error message (for blocked commands like git remote)
// - Exits 0 to allow the command (for non-git/gh commands)
const GitHookScript = `#!/bin/bash
# ccells-git-hook: PreToolUse hook for intercepting git/gh commands
# Receives JSON on stdin, extracts the bash command, and decides:
# - git fetch/pull/push: run through proxy, output result, exit 2 (block original)
# - git remote: block with error message
# - gh pr/issue: run through proxy, output result, exit 2 (block original)
# - other commands: exit 0 (allow)

# Read JSON input from stdin
input=$(cat)

# Extract command from JSON using grep/sed (portable)
command=$(echo "$input" | grep -o '"command"[[:space:]]*:[[:space:]]*"[^"]*"' | head -1 | sed 's/.*"command"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')

# If no command found, allow
if [ -z "$command" ]; then
    exit 0
fi

# Check for git remote commands (blocked entirely)
if echo "$command" | grep -qE '^git\s+remote(\s|$)'; then
    echo "git remote commands are blocked in ccells containers" >&2
    exit 2
fi

# Check for git fetch/pull/push commands (proxy)
if echo "$command" | grep -qE '^git\s+(fetch|pull|push)(\s|$)'; then
    # Run through proxy - the proxy script handles the full command
    /root/.claude/bin/ccells-git-proxy $command
    exit_code=$?
    # Exit 2 to block the original command (proxy already ran it)
    # Use exit code 0 if proxy succeeded, 2 otherwise (exit 2 = block in Claude)
    if [ $exit_code -eq 0 ]; then
        exit 2  # Block original, proxy succeeded
    else
        exit 2  # Block original, but proxy failed (error already shown)
    fi
fi

# Check for gh pr commands (proxy)
if echo "$command" | grep -qE '^gh\s+pr\s+(view|checks|diff|list|create|merge)(\s|$)'; then
    /root/.claude/bin/ccells-git-proxy $command
    exit 2  # Block original
fi

# Check for gh issue commands (proxy)
if echo "$command" | grep -qE '^gh\s+issue\s+(view|list)(\s|$)'; then
    /root/.claude/bin/ccells-git-proxy $command
    exit 2  # Block original
fi

# Allow all other commands
exit 0
`
