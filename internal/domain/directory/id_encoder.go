package directory

import (
	"encoding/base64"
	"fmt"
	"strings"

	"main/internal/scalar"
)

const idPrefix = "data:text/x.dir,"

func EncodeID(path string) scalar.ID {
	if path == "." {
		return scalar.ID{}
	}
	encoded := base64.URLEncoding.EncodeToString([]byte(path))
	return scalar.ToID(idPrefix + encoded)
}

func DecodeID(id scalar.ID) (string, error) {
	idStr := id.String()
	if idStr == "" {
		return ".", nil
	}
	if !strings.HasPrefix(idStr, idPrefix) {
		return "", fmt.Errorf("invalid directory ID format")
	}

	encoded := strings.TrimPrefix(idStr, idPrefix)
	decoded, err := base64.URLEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("failed to decode directory ID: %w", err)
	}

	return string(decoded), nil
}
