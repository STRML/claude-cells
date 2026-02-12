package main

// parseCommand determines which subcommand to run.
// Bare "ccells" defaults to "up" (which does smart attach-or-create).
func parseCommand(args []string) string {
	if len(args) == 0 {
		return "up"
	}
	switch args[0] {
	case "up", "attach", "down", "create", "rm", "pause", "unpause",
		"ps", "logs", "pair", "unpair", "help", "status", "merge":
		return args[0]
	case "--version", "-v":
		return "version"
	case "--help", "-h":
		return "help"
	case "--runtime":
		// Flag before command — skip flag+value, re-parse
		if len(args) >= 3 {
			return parseCommand(args[2:])
		}
		return "up"
	default:
		return "help"
	}
}

// parseFlags extracts global flags (--runtime) from args and returns the
// remaining positional args. This is used before command dispatch.
func parseFlags(args []string) (runtime string, rest []string) {
	rest = make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		if args[i] == "--runtime" && i+1 < len(args) {
			runtime = args[i+1]
			i++ // skip value
		} else {
			rest = append(rest, args[i])
		}
	}
	return runtime, rest
}
