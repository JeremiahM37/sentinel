// Package titleutil provides title matching utilities using Jaccard word overlap
// with stopword removal.
package titleutil

import (
	"regexp"
	"strings"
)

var (
	nonAlphaNum = regexp.MustCompile(`[^\w\s]`)
	stopwords   = map[string]bool{
		"a": true, "an": true, "the": true, "and": true, "or": true,
		"but": true, "in": true, "on": true, "at": true, "to": true,
		"for": true, "of": true, "with": true, "by": true, "from": true,
		"is": true, "it": true, "as": true, "was": true, "are": true,
		"be": true, "this": true, "that": true, "which": true, "who": true,
		"whom": true, "what": true, "where": true, "when": true,
	}
)

// normalise lowercases the input, strips non-alphanumeric characters (except
// whitespace and underscores), and removes stopwords.
func normalise(text string) map[string]bool {
	lower := strings.ToLower(text)
	cleaned := nonAlphaNum.ReplaceAllString(lower, " ")
	words := strings.Fields(cleaned)
	result := make(map[string]bool)
	for _, w := range words {
		if !stopwords[w] && w != "" {
			result[w] = true
		}
	}
	return result
}

// TitleMatchScore computes Jaccard word overlap between two titles after
// stopword removal. Returns a value between 0.0 (no overlap) and 1.0 (perfect match).
func TitleMatchScore(a, b string) float64 {
	wordsA := normalise(a)
	wordsB := normalise(b)

	if len(wordsA) == 0 || len(wordsB) == 0 {
		return 0.0
	}

	intersection := 0
	for w := range wordsA {
		if wordsB[w] {
			intersection++
		}
	}

	// Union = |A| + |B| - |A ∩ B|
	union := len(wordsA) + len(wordsB) - intersection
	if union == 0 {
		return 0.0
	}

	return float64(intersection) / float64(union)
}
