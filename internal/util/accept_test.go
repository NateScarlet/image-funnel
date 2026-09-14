package util

import (
	"testing"
)

func TestParseAcceptHeader(t *testing.T) {
	tests := []struct {
		name     string
		accept   string
		expected []MediaType
	}{
		{
			name:   "empty",
			accept: "",
			expected: nil,
		},
		{
			name:   "single type",
			accept: "image/avif",
			expected: []MediaType{
				{Type: "image/avif", Quality: 1.0, Specificity: 2},
			},
		},
		{
			name:   "multiple types",
			accept: "image/avif, image/webp",
			expected: []MediaType{
				{Type: "image/avif", Quality: 1.0, Specificity: 2},
				{Type: "image/webp", Quality: 1.0, Specificity: 2},
			},
		},
		{
			name:   "with q-values",
			accept: "image/avif;q=0.9, image/webp;q=0.8",
			expected: []MediaType{
				{Type: "image/avif", Quality: 0.9, Specificity: 2},
				{Type: "image/webp", Quality: 0.8, Specificity: 2},
			},
		},
		{
			name:   "q-values with different order",
			accept: "image/webp;q=0.8, image/avif;q=0.9",
			expected: []MediaType{
				{Type: "image/avif", Quality: 0.9, Specificity: 2},
				{Type: "image/webp", Quality: 0.8, Specificity: 2},
			},
		},
		{
			name:   "with wildcards",
			accept: "image/avif, image/*, */*",
			expected: []MediaType{
				{Type: "image/avif", Quality: 1.0, Specificity: 2},
				{Type: "image/*", Quality: 1.0, Specificity: 1},
				{Type: "*/*", Quality: 1.0, Specificity: 0},
			},
		},
		{
			name:   "wildcards with q-values",
			accept: "image/*;q=0.5, image/avif;q=0.9, */*;q=0.1",
			expected: []MediaType{
				{Type: "image/avif", Quality: 0.9, Specificity: 2},
				{Type: "image/*", Quality: 0.5, Specificity: 1},
				{Type: "*/*", Quality: 0.1, Specificity: 0},
			},
		},
		{
			name:   "with extra parameters",
			accept: "image/avif;q=0.9;foo=bar, image/webp",
			expected: []MediaType{
				{Type: "image/webp", Quality: 1.0, Specificity: 2},
				{Type: "image/avif", Quality: 0.9, Specificity: 2, Params: map[string]string{"foo": "bar"}},
			},
		},
		{
			name:   "Chrome-style",
			accept: "image/avif,image/webp,image/apng,image/*,*/*;q=0.8",
			expected: []MediaType{
				{Type: "image/avif", Quality: 1.0, Specificity: 2},
				{Type: "image/webp", Quality: 1.0, Specificity: 2},
				{Type: "image/apng", Quality: 1.0, Specificity: 2},
				{Type: "image/*", Quality: 1.0, Specificity: 1},
				{Type: "*/*", Quality: 0.8, Specificity: 0},
			},
		},
		{
			name:   "Firefox-style",
			accept: "image/avif,image/webp,*/*;q=0.8",
			expected: []MediaType{
				{Type: "image/avif", Quality: 1.0, Specificity: 2},
				{Type: "image/webp", Quality: 1.0, Specificity: 2},
				{Type: "*/*", Quality: 0.8, Specificity: 0},
			},
		},
		{
			name:   "Safari-style (no avif/webp explicit)",
			accept: "image/*,*/*;q=0.8",
			expected: []MediaType{
				{Type: "image/*", Quality: 1.0, Specificity: 1},
				{Type: "*/*", Quality: 0.8, Specificity: 0},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseAcceptHeader(tt.accept)
			if len(result) != len(tt.expected) {
				t.Errorf("expected %d types, got %d: %+v", len(tt.expected), len(result), result)
				return
			}
			for i, exp := range tt.expected {
				if result[i].Type != exp.Type {
					t.Errorf("type[%d]: expected %q, got %q", i, exp.Type, result[i].Type)
				}
				if result[i].Quality != exp.Quality {
					t.Errorf("quality[%d]: expected %v, got %v", i, exp.Quality, result[i].Quality)
				}
				if result[i].Specificity != exp.Specificity {
					t.Errorf("specificity[%d]: expected %v, got %v", i, exp.Specificity, result[i].Specificity)
				}
				if exp.Params != nil {
					for k, v := range exp.Params {
						if result[i].Params[k] != v {
							t.Errorf("param[%d][%s]: expected %q, got %q", i, k, v, result[i].Params[k])
						}
					}
				}
			}
		})
	}
}

func TestPreferredImageFormats(t *testing.T) {
	tests := []struct {
		name     string
		accept   string
		expected []string
	}{
		{
			name:     "avif preferred",
			accept:   "image/avif, image/webp",
			expected: []string{"image/avif", "image/webp"},
		},
		{
			name:     "webp preferred",
			accept:   "image/webp, image/avif",
			expected: []string{"image/webp", "image/avif"},
		},
		{
			name:     "with q-values",
			accept:   "image/avif;q=0.9, image/webp;q=0.8",
			expected: []string{"image/avif", "image/webp"},
		},
		{
			name:     "only webp supported",
			accept:   "image/webp, image/*",
			expected: []string{"image/webp", "image/*"},
		},
		{
			name:     "only avif supported",
			accept:   "image/avif, */*",
			expected: []string{"image/avif", "*/*"},
		},
		{
			name:     "no explicit image formats",
			accept:   "image/*, */*",
			expected: []string{"image/*", "*/*"},
		},
		{
			name:     "empty accept",
			accept:   "",
			expected: nil,
		},
		{
			name:     "unsupported formats only",
			accept:   "image/apng, image/jpeg",
			expected: []string{}, // no supported formats, but will have wildcards if present
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := PreferredImageFormats(tt.accept)
			if len(result) != len(tt.expected) {
				t.Errorf("expected %d formats, got %d: %v", len(tt.expected), len(result), result)
				return
			}
			for i, exp := range tt.expected {
				if result[i] != exp {
					t.Errorf("format[%d]: expected %q, got %q", i, exp, result[i])
				}
			}
		})
	}
}