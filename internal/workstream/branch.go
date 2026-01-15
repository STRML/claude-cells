package workstream

import (
	"regexp"
	"strings"
)

// stopWords are common words stripped from branch names
var stopWords = map[string]bool{
	"the":  true,
	"a":    true,
	"an":   true,
	"to":   true,
	"for":  true,
	"with": true,
	"and":  true,
	"or":   true,
	"in":   true,
	"on":   true,
	"at":   true,
	"by":   true,
	"of":   true,
	"is":   true,
	"it":   true,
	"that": true,
}

const maxBranchLength = 50
const defaultBranchName = "workstream"

// nonAlphaNum matches any character that is not alphanumeric, whitespace, or hyphen
var nonAlphaNum = regexp.MustCompile(`[^a-z0-9\s-]`)

// GenerateBranchName creates a git branch name from a prompt.
// Rules:
// - Lowercase
// - Hyphens instead of spaces
// - Max 50 chars
// - Strip common words
// - Keep it readable
func GenerateBranchName(prompt string) string {
	// Convert to lowercase
	name := strings.ToLower(prompt)

	// Replace special characters with spaces (except hyphens)
	name = nonAlphaNum.ReplaceAllString(name, " ")

	// Split into words
	words := strings.Fields(name)

	// Filter out stop words
	var filtered []string
	for _, word := range words {
		if !stopWords[word] && word != "" {
			filtered = append(filtered, word)
		}
	}

	// Handle empty result
	if len(filtered) == 0 {
		return defaultBranchName
	}

	// Join with hyphens
	name = strings.Join(filtered, "-")

	// Truncate to max length
	if len(name) > maxBranchLength {
		name = name[:maxBranchLength]
	}

	// Remove trailing hyphen if truncation left one
	name = strings.TrimRight(name, "-")

	return name
}
