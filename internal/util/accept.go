package util

import (
	"sort"
	"strconv"
	"strings"
)

// MediaType represents a parsed media type with quality factor
type MediaType struct {
	Type      string // e.g., "image/avif"
	Quality   float64
	Params    map[string]string // other parameters
	Specificity int            // for tie-breaking: more specific types first
}

// parseAcceptHeader parses an Accept header value and returns a slice of MediaType
// sorted by quality (descending) and specificity (descending).
// RFC 7231 Section 5.3.2
func ParseAcceptHeader(accept string) []MediaType {
	if accept == "" {
		return nil
	}

	var types []MediaType
	parts := strings.Split(accept, ",")

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// Split on ';' to separate media type from parameters
		segments := strings.Split(part, ";")
		if len(segments) == 0 {
			continue
		}

		mediaType := strings.TrimSpace(segments[0])
		if mediaType == "" {
			continue
		}

		mt := MediaType{
			Type:    mediaType,
			Quality: 1.0, // default q=1
			Params:  make(map[string]string),
		}

		// Parse parameters
		for i := 1; i < len(segments); i++ {
			param := strings.TrimSpace(segments[i])
			if param == "" {
				continue
			}

			kv := strings.SplitN(param, "=", 2)
			if len(kv) != 2 {
				continue
			}
			key := strings.TrimSpace(kv[0])
			value := strings.TrimSpace(kv[1])
			value = strings.Trim(value, "\"")

			if key == "q" {
				if q, err := strconv.ParseFloat(value, 64); err == nil {
					mt.Quality = q
				}
			} else {
				mt.Params[key] = value
			}
		}

		// Calculate specificity: more specific types sort first
		// */* = 0, type/* = 1, type/subtype = 2
		if mediaType == "*/*" {
			mt.Specificity = 0
		} else if strings.HasSuffix(mediaType, "/*") {
			mt.Specificity = 1
		} else {
			mt.Specificity = 2
		}

		types = append(types, mt)
	}

	// Sort by quality desc, then specificity desc
	sort.Slice(types, func(i, j int) bool {
		if types[i].Quality != types[j].Quality {
			return types[i].Quality > types[j].Quality
		}
		return types[i].Specificity > types[j].Specificity
	})

	return types
}

// PreferredImageFormats extracts supported image formats from Accept header
// in priority order. Returns a slice of MIME types like ["image/avif", "image/webp"].
func PreferredImageFormats(accept string) []string {
	types := ParseAcceptHeader(accept)
	var formats []string
	supported := map[string]bool{
		"image/avif": true,
		"image/webp": true,
	}
	wildcardsAdded := make(map[string]bool)

	for _, mt := range types {
		if supported[mt.Type] {
			formats = append(formats, mt.Type)
		}
		// Include wildcards (image/*, */*) as fallback indicators
		if (mt.Type == "image/*" || mt.Type == "*/*") && !wildcardsAdded[mt.Type] {
			formats = append(formats, mt.Type)
			wildcardsAdded[mt.Type] = true
		}
	}

	return formats
}