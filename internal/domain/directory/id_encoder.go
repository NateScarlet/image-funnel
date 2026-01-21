package directory

import (
	"fmt"
	"strings"

	"main/internal/scalar"
)

const idPrefix = "dir:"

func EncodeID(path string) scalar.ID {
	return scalar.ToID(idPrefix + path)
}

func DecodeID(id scalar.ID) (string, error) {
	idStr := id.String()
	if idStr == "" {
		return "", fmt.Errorf("id must not be empty")
	}
	if !strings.HasPrefix(idStr, idPrefix) {
		return "", fmt.Errorf("invalid directory ID format")
	}

	return strings.TrimPrefix(idStr, idPrefix), nil
}
