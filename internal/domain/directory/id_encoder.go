package directory

import (
	"encoding/base64"
	"fmt"
	"strings"
)

const directoryIDPrefix = "data:text/x.dir,"

func EncodeDirectoryID(path string) string {
	if path == "." {
		return ""
	}
	encoded := base64.URLEncoding.EncodeToString([]byte(path))
	return directoryIDPrefix + encoded
}

func DecodeDirectoryID(id string) (string, error) {
	if id == "" {
		return ".", nil
	}
	if !strings.HasPrefix(id, directoryIDPrefix) {
		return "", fmt.Errorf("invalid directory ID format")
	}

	encoded := strings.TrimPrefix(id, directoryIDPrefix)
	decoded, err := base64.URLEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("failed to decode directory ID: %w", err)
	}

	return string(decoded), nil
}
