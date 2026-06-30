package http

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gorilla/mux"
)

func TestStaticRoutes(t *testing.T) {
	// Create a temporary directory for frontend
	tempDir := t.TempDir()

	// Write mock index.html
	indexContent := "<html><body>index</body></html>"
	err := os.WriteFile(filepath.Join(tempDir, "index.html"), []byte(indexContent), 0644)
	if err != nil {
		t.Fatalf("failed to write mock index.html: %v", err)
	}

	// Create static/assets directory and write a file
	assetsDir := filepath.Join(tempDir, "assets")
	err = os.MkdirAll(assetsDir, 0755)
	if err != nil {
		t.Fatalf("failed to create assets dir: %v", err)
	}

	jsContent := "console.log('test')"
	err = os.WriteFile(filepath.Join(assetsDir, "app-123456.js"), []byte(jsContent), 0644)
	if err != nil {
		t.Fatalf("failed to write mock JS: %v", err)
	}

	// Create router and register static routes
	r := mux.NewRouter()
	addStaticRoutes(r, tempDir)

	tests := []struct {
		name                string
		path                string
		expectedStatus      int
		expectedCache       string
		expectedContentType string
		expectedBody        string
	}{
		{
			name:                "Root path fallback to index.html",
			path:                "/",
			expectedStatus:      http.StatusOK,
			expectedCache:       "no-cache",
			expectedContentType: "text/html",
			expectedBody:        indexContent,
		},
		{
			name:                "Hashed static asset",
			path:                "/static/assets/app-123456.js",
			expectedStatus:      http.StatusOK,
			expectedCache:       "public, max-age=31536000, immutable",
			expectedContentType: "text/javascript",
			expectedBody:        jsContent,
		},
		{
			name:                "SPA routing fallback to index.html",
			path:                "/dashboard/settings",
			expectedStatus:      http.StatusOK,
			expectedCache:       "no-cache",
			expectedContentType: "text/html",
			expectedBody:        indexContent,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tc.path, nil)
			rr := httptest.NewRecorder()

			r.ServeHTTP(rr, req)

			if rr.Code != tc.expectedStatus {
				t.Errorf("expected status %d, got %d", tc.expectedStatus, rr.Code)
			}

			cacheControl := rr.Header().Get("Cache-Control")
			if !strings.Contains(cacheControl, tc.expectedCache) {
				t.Errorf("expected Cache-Control to contain %q, got %q", tc.expectedCache, cacheControl)
			}

			contentType := rr.Header().Get("Content-Type")
			if !strings.Contains(contentType, tc.expectedContentType) {
				t.Errorf("expected Content-Type to contain %q, got %q", tc.expectedContentType, contentType)
			}

			body, _ := io.ReadAll(rr.Body)
			if string(body) != tc.expectedBody {
				t.Errorf("expected body %q, got %q", tc.expectedBody, string(body))
			}
		})
	}
}
