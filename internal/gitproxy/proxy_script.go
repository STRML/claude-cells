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

# Parse response using jq if available, otherwise use grep/sed
if command -v jq &> /dev/null; then
    EXIT_CODE=$(echo "$RESPONSE" | jq -r '.exit_code // 1')
    STDOUT=$(echo "$RESPONSE" | jq -r '.stdout // ""')
    STDERR=$(echo "$RESPONSE" | jq -r '.stderr // ""')
    ERROR=$(echo "$RESPONSE" | jq -r '.error // ""')
else
    # Fallback parsing without jq (less robust but works for simple cases)
    EXIT_CODE=$(echo "$RESPONSE" | grep -o '"exit_code":[0-9]*' | cut -d: -f2)
    [ -z "$EXIT_CODE" ] && EXIT_CODE=1

    # For stdout/stderr/error, this is a simplified extraction
    # In practice, jq should always be available in our containers
    STDOUT=""
    STDERR=""
    ERROR=$(echo "$RESPONSE" | grep -o '"error":"[^"]*"' | cut -d'"' -f4)
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

// HookConfig returns the Claude Code settings.json hook configuration
// that intercepts git and gh commands.
const HookConfig = `{
  "hooks": {
    "Bash": [
      {
        "matcher": "^git\\s+(fetch|pull|push)",
        "hooks": [
          {
            "type": "command",
            "command": "ccells-git-proxy git $@"
          }
        ]
      },
      {
        "matcher": "^git\\s+remote",
        "hooks": [
          {
            "type": "block",
            "message": "git remote commands are blocked in ccells containers"
          }
        ]
      },
      {
        "matcher": "^gh\\s+pr\\s+(view|checks|diff|list|create|merge)",
        "hooks": [
          {
            "type": "command",
            "command": "ccells-git-proxy gh $@"
          }
        ]
      },
      {
        "matcher": "^gh\\s+issue\\s+(view|list)",
        "hooks": [
          {
            "type": "command",
            "command": "ccells-git-proxy gh $@"
          }
        ]
      }
    ]
  }
}`
