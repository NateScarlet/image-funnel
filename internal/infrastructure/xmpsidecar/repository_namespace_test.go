package xmpsidecar

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"main/internal/domain/metadata"

	"github.com/beevik/etree"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWrite_NamespacePlacement(t *testing.T) {
	repo := NewRepository()
	tempFile := filepath.Join(os.TempDir(), "test_ns_placement.jpg")
	xmpPath := tempFile + ".xmp"
	defer os.Remove(xmpPath)

	testData := metadata.NewXMPData(5, "keep", time.Now())
	err := repo.Write(tempFile, testData)
	require.NoError(t, err)

	// Read raw content to parse and check
	content, err := os.ReadFile(xmpPath)
	require.NoError(t, err)

	doc := etree.NewDocument()
	err = doc.ReadFromBytes(content)
	require.NoError(t, err)

	rdf := doc.FindElement("//rdf:RDF")
	require.NotNil(t, rdf, "rdf:RDF element not found")

	// 1. Verify rdf:RDF has xmlns:rdf
	rdfAttr := rdf.SelectAttr("xmlns:rdf")
	assert.NotNil(t, rdfAttr, "rdf:RDF should have xmlns:rdf")
	assert.Equal(t, RDFNamespace, rdfAttr.Value)

	// 2. Verify rdf:RDF does NOT have other namespaces
	assert.Nil(t, rdf.SelectAttr("xmlns:xmp"), "rdf:RDF should not have xmlns:xmp")
	assert.Nil(t, rdf.SelectAttr("xmlns:imagefunnel"), "rdf:RDF should not have xmlns:imagefunnel")
	assert.Nil(t, rdf.SelectAttr("xmlns:MicrosoftPhoto"), "rdf:RDF should not have xmlns:MicrosoftPhoto")

	// 3. Verify rdf:Description has the other namespaces
	desc := rdf.FindElement("rdf:Description")
	require.NotNil(t, desc, "rdf:Description element not found")

	assert.NotNil(t, desc.SelectAttr("xmlns:xmp"), "rdf:Description should have xmlns:xmp")
	assert.NotNil(t, desc.SelectAttr("xmlns:imagefunnel"), "rdf:Description should have xmlns:imagefunnel")
	assert.NotNil(t, desc.SelectAttr("xmlns:MicrosoftPhoto"), "rdf:Description should have xmlns:MicrosoftPhoto")
}
