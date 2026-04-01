package titleutil

import (
	"testing"
)

func TestTitleMatchScore(t *testing.T) {
	tests := []struct {
		name     string
		a, b     string
		wantMin  float64
		wantMax  float64
	}{
		{
			name:    "exact match",
			a:       "The Matrix",
			b:       "The Matrix",
			wantMin: 1.0,
			wantMax: 1.0,
		},
		{
			name:    "stopword removal makes match",
			a:       "The Matrix",
			b:       "Matrix",
			wantMin: 1.0,
			wantMax: 1.0,
		},
		{
			name:    "case insensitive",
			a:       "THE MATRIX",
			b:       "the matrix",
			wantMin: 1.0,
			wantMax: 1.0,
		},
		{
			name:    "punctuation removed",
			a:       "Spider-Man: No Way Home",
			b:       "Spider Man No Way Home",
			wantMin: 0.9,
			wantMax: 1.0,
		},
		{
			name:    "partial overlap",
			a:       "Star Wars A New Hope",
			b:       "Star Wars",
			wantMin: 0.3,
			wantMax: 0.7,
		},
		{
			name:    "no overlap",
			a:       "Inception",
			b:       "Titanic",
			wantMin: 0.0,
			wantMax: 0.0,
		},
		{
			name:    "empty first string",
			a:       "",
			b:       "Something",
			wantMin: 0.0,
			wantMax: 0.0,
		},
		{
			name:    "empty second string",
			a:       "Something",
			b:       "",
			wantMin: 0.0,
			wantMax: 0.0,
		},
		{
			name:    "both empty",
			a:       "",
			b:       "",
			wantMin: 0.0,
			wantMax: 0.0,
		},
		{
			name:    "only stopwords",
			a:       "the and or",
			b:       "the and or",
			wantMin: 0.0,
			wantMax: 0.0,
		},
		{
			name:    "one word same",
			a:       "Avatar",
			b:       "Avatar",
			wantMin: 1.0,
			wantMax: 1.0,
		},
		{
			name:    "subset match",
			a:       "Breaking Bad",
			b:       "Breaking Bad Season 1",
			wantMin: 0.5,
			wantMax: 0.8,
		},
		{
			name:    "year in title",
			a:       "Blade Runner 2049",
			b:       "Blade Runner 2049",
			wantMin: 1.0,
			wantMax: 1.0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			score := TitleMatchScore(tc.a, tc.b)
			if score < tc.wantMin || score > tc.wantMax {
				t.Errorf("TitleMatchScore(%q, %q) = %.4f, want [%.2f, %.2f]",
					tc.a, tc.b, score, tc.wantMin, tc.wantMax)
			}
		})
	}
}

func TestTitleMatchScoreSymmetry(t *testing.T) {
	pairs := [][2]string{
		{"The Matrix", "Matrix"},
		{"Star Wars", "Star Wars A New Hope"},
		{"Breaking Bad", "Breaking"},
	}

	for _, pair := range pairs {
		score1 := TitleMatchScore(pair[0], pair[1])
		score2 := TitleMatchScore(pair[1], pair[0])
		if score1 != score2 {
			t.Errorf("TitleMatchScore(%q, %q) = %.4f != TitleMatchScore(%q, %q) = %.4f (not symmetric)",
				pair[0], pair[1], score1, pair[1], pair[0], score2)
		}
	}
}

func TestNormalise(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected map[string]bool
	}{
		{
			name:     "basic words",
			input:    "Hello World",
			expected: map[string]bool{"hello": true, "world": true},
		},
		{
			name:     "stopwords removed",
			input:    "The Lord of the Rings",
			expected: map[string]bool{"lord": true, "rings": true},
		},
		{
			name:     "punctuation cleaned",
			input:    "Spider-Man: Into the Spider-Verse",
			expected: map[string]bool{"spider": true, "man": true, "into": true, "verse": true},
		},
		{
			name:     "empty string",
			input:    "",
			expected: map[string]bool{},
		},
		{
			name:     "only stopwords",
			input:    "the a an",
			expected: map[string]bool{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := normalise(tc.input)
			if len(got) != len(tc.expected) {
				t.Errorf("normalise(%q) has %d words, want %d", tc.input, len(got), len(tc.expected))
			}
			for w := range tc.expected {
				if !got[w] {
					t.Errorf("normalise(%q) missing word %q", tc.input, w)
				}
			}
		})
	}
}
