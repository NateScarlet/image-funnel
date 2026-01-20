package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEnsurePathInRoot(t *testing.T) {
	rootDir := t.TempDir()

	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{
			name:    "current directory",
			path:    ".",
			wantErr: false,
		},
		{
			name:    "single subdirectory",
			path:    "subdir",
			wantErr: false,
		},
		{
			name:    "nested subdirectory",
			path:    "subdir/subdir",
			wantErr: false,
		},
		{
			name:    "directory with dot prefix",
			path:    "subdir/..subdir",
			wantErr: false,
		},
		{
			name:    "directory with dot suffix",
			path:    "subdir/subdir..",
			wantErr: false,
		},
		{
			name:    "directory with middle dots",
			path:    "subdir/sub..dir",
			wantErr: false,
		},
		{
			name:    "path traversal with single parent",
			path:    "../escape",
			wantErr: true,
		},
		{
			name:    "path traversal with double parent",
			path:    "../../escape",
			wantErr: true,
		},
		{
			name:    "path traversal with current parent",
			path:    "./../escape",
			wantErr: true,
		},
		{
			name:    "path traversal with nested parent",
			path:    "subdir/../../escape",
			wantErr: true,
		},
		{
			name:    "path traversal with backslash",
			path:    "..\\escape",
			wantErr: true,
		},
		{
			name:    "path traversal with double backslash",
			path:    "..\\..\\escape",
			wantErr: true,
		},
		{
			name:    "absolute path",
			path:    "/absolute/path",
			wantErr: true,
		},
		{
			name:    "absolute path with drive letter",
			path:    "C:\\Windows\\System32",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := EnsurePathInRoot(rootDir, tt.path)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
