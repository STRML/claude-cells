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

// GenerateUniqueBranchName creates a unique git branch name from a prompt,
// checking against existing branch names and appending a numeric suffix if needed.
func GenerateUniqueBranchName(prompt string, existingBranches []string) string {
	baseName := GenerateBranchName(prompt)

	// Build a set of existing branches for O(1) lookup
	existing := make(map[string]bool)
	for _, b := range existingBranches {
		existing[b] = true
	}

	// If base name is unique, use it
	if !existing[baseName] {
		return baseName
	}

	// Append numeric suffix until unique
	for i := 2; i <= 100; i++ {
		candidate := baseName + "-" + itoa(i)
		// Ensure we don't exceed max length
		if len(candidate) > maxBranchLength {
			// Truncate base name to make room for suffix
			maxBase := maxBranchLength - len("-") - len(itoa(i))
			if maxBase < 1 {
				maxBase = 1
			}
			truncatedBase := baseName
			if len(truncatedBase) > maxBase {
				truncatedBase = strings.TrimRight(truncatedBase[:maxBase], "-")
			}
			candidate = truncatedBase + "-" + itoa(i)
		}
		if !existing[candidate] {
			return candidate
		}
	}

	// Fallback: use timestamp suffix (shouldn't happen in practice)
	return baseName + "-" + itoa(int(idCounter.Add(1)))
}

// itoa is a simple int-to-string helper to avoid importing strconv
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	negative := i < 0
	if negative {
		i = -i
	}
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if negative {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

// GenerateBranchName creates a git branch name from a prompt.
// Rules:
// - Lowercase
// - Hyphens instead of spaces
// - Max 50 chars
// - Strip common words
// - Keep first ~5 meaningful words for readability
// - Truncate at word boundaries
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

	// Limit to first 5 meaningful words to keep names concise
	maxWords := 5
	if len(filtered) > maxWords {
		filtered = filtered[:maxWords]
	}

	// Join with hyphens
	name = strings.Join(filtered, "-")

	// Truncate at word boundary if still too long
	if len(name) > maxBranchLength {
		// Find last hyphen within the limit
		lastHyphen := strings.LastIndex(name[:maxBranchLength], "-")
		if lastHyphen > 0 {
			name = name[:lastHyphen]
		} else {
			// No hyphen found, just truncate (single long word)
			name = name[:maxBranchLength]
		}
	}

	// Remove trailing hyphen if any
	name = strings.TrimRight(name, "-")

	return name
}
