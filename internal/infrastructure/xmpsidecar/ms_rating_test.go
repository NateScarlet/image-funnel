package xmpsidecar

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"main/internal/domain/metadata"

	"github.com/beevik/etree"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMicrosoftPhotoRating(t *testing.T) {
	repo := NewRepository()
	tempFile := filepath.Join(os.TempDir(), "test-ms-rating.jpg")
	xmpPath := tempFile + ".xmp"
	defer os.Remove(xmpPath)

	// Test case 1: Write ensures MicrosoftPhoto:Rating is percentile
	t.Run("Write maps to percentile", func(t *testing.T) {
		testData := metadata.NewXMPData(3, "", time.Now()) // 3 stars -> 50
		err := repo.Write(tempFile, testData)
		require.NoError(t, err)

		// Parse just to be sure we find the element regardless of format
		doc := etree.NewDocument()
		err = doc.ReadFromFile(xmpPath)
		require.NoError(t, err)

		desc := doc.FindElement("//rdf:Description")
		require.NotNil(t, desc, "Should find rdf:Description")

		msRating := findElementText(desc, []string{"MicrosoftPhoto:Rating"})
		assert.Equal(t, "50", msRating, "Should write MicrosoftPhoto rating as 50")

		xRating := findElementText(desc, []string{"xmp:Rating"})
		assert.Equal(t, "3", xRating, "Should write xmp rating as 3")
	})

	// Test case 2: Read handles percentile from MicrosoftPhoto:Rating only
	t.Run("Read maps from percentile", func(t *testing.T) {
		// Manually create XMP with ONLY MicrosoftPhoto:Rating
		msXMP := `<?xml version="1.0" encoding="UTF-8"?>
<x:xmpmeta xmlns:x="adobe:ns:meta/">
  <rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">
    <rdf:Description rdf:about="" xmlns:MicrosoftPhoto="http://ns.microsoft.com/photo/1.0/">
      <MicrosoftPhoto:Rating>75</MicrosoftPhoto:Rating>
    </rdf:Description>
  </rdf:RDF>
</x:xmpmeta>`
		err := os.WriteFile(xmpPath, []byte(msXMP), 0644)
		require.NoError(t, err)

		readData, err := repo.Read(tempFile)
		require.NoError(t, err)
		assert.NotNil(t, readData)
		// 75 -> 4 stars
		assert.Equal(t, 4, readData.Rating(), "Should map 75 back to 4 stars")
	})

	// Test case 3: Read prioritizes xmp:Rating over MicrosoftPhoto:Rating
	t.Run("Read prioritizes XMP standard", func(t *testing.T) {
		// Manually create XMP with conflicting values
		// xmp:Rating = 2, MicrosoftPhoto:Rating = 99 (5 stars)
		mixedXMP := `<?xml version="1.0" encoding="UTF-8"?>
<x:xmpmeta xmlns:x="adobe:ns:meta/">
  <rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">
    <rdf:Description rdf:about="" xmlns:xmp="http://ns.adobe.com/xap/1.0/" xmlns:MicrosoftPhoto="http://ns.microsoft.com/photo/1.0/">
      <xmp:Rating>2</xmp:Rating>
      <MicrosoftPhoto:Rating>99</MicrosoftPhoto:Rating>
    </rdf:Description>
  </rdf:RDF>
</x:xmpmeta>`
		err := os.WriteFile(xmpPath, []byte(mixedXMP), 0644)
		require.NoError(t, err)

		readData, err := repo.Read(tempFile)
		require.NoError(t, err)
		assert.NotNil(t, readData)
		// Should take xmp:Rating = 2
		assert.Equal(t, 2, readData.Rating(), "Should prioritize standard XMP rating")
	})

	// Test case 4: Check all mappings
	t.Run("Check all mappings", func(t *testing.T) {
		mappings := []struct {
			stars           int
			expectedPercent int
		}{
			{0, 0},
			{1, 1},
			{2, 25},
			{3, 50},
			{4, 75},
			{5, 100},
		}

		for _, m := range mappings {
			// Clean up previous file to force new creation
			os.Remove(xmpPath)

			testData := metadata.NewXMPData(m.stars, "", time.Now())
			err := repo.Write(tempFile, testData)
			require.NoError(t, err)

			doc := etree.NewDocument()
			err = doc.ReadFromFile(xmpPath)
			require.NoError(t, err)

			desc := doc.FindElement("//rdf:Description")
			require.NotNil(t, desc, "Should find rdf:Description")

			msRating := findElementText(desc, []string{"MicrosoftPhoto:Rating"})
			assert.Equal(t, strconv.Itoa(m.expectedPercent), msRating, "Failed mapping for %d stars", m.stars)
		}
	})
}

func findElementText(elem *etree.Element, tags []string) string {
	for _, tag := range tags {
		parts := strings.Split(tag, ":")
		if len(parts) == 2 {
			prefix := parts[0]
			localName := parts[1]
			attrKey := prefix + ":" + localName
			if attr := elem.SelectAttr(attrKey); attr != nil {
				return attr.Value
			}
		}

		if child := elem.FindElement(tag); child != nil {
			return child.Text()
		}
	}
	return ""
}
