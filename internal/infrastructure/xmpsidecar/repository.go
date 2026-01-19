package xmpsidecar

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"main/internal/domain/metadata"
	"main/internal/util"

	"github.com/beevik/etree"
)

const (
	XMPNamespace     = "http://ns.adobe.com/xap/1.0/"
	RDFNamespace     = "http://www.w3.org/1999/02/22-rdf-syntax-ns#"
	ImageFunnelNS    = "http://ns.imagefunnel.internal/1.0/"
	MicrosoftPhotoNS = "http://ns.microsoft.com/photo/1.0/"
)

type Repository struct{}

func NewRepository() *Repository {
	return &Repository{}
}

func (r *Repository) Read(imagePath string) (*metadata.XMPData, error) {
	xmpPath := imagePath + ".xmp"

	data, err := os.ReadFile(xmpPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read XMP file: %w", err)
	}

	doc := etree.NewDocument()
	if err := doc.ReadFromBytes(data); err != nil {
		return nil, fmt.Errorf("failed to parse XMP: %w", err)
	}

	result := &XMPData{rating: 0}
	for _, rdf := range doc.FindElements("//RDF/Description") {
		rating := findElementText(rdf, []string{"xmp:Rating", "Rating", "MicrosoftPhoto:Rating"})
		if rating != "" {
			if val, err := strconv.Atoi(rating); err == nil {
				result.rating = val
			}
		}

		action := findElementText(rdf, []string{"imagefunnel:Action", "Action"})
		if action != "" {
			result.action = action
		}

		timestamp := findElementText(rdf, []string{"imagefunnel:Timestamp", "Timestamp"})
		if timestamp != "" {
			if val, err := time.Parse(time.RFC3339, timestamp); err == nil {
				result.timestamp = val
			}
		}

	}

	return metadata.NewXMPData(
		result.rating,
		result.action,
		result.timestamp,
	), nil
}

func (r *Repository) Write(imagePath string, data *metadata.XMPData) error {
	xmpPath := imagePath + ".xmp"

	var doc *etree.Document

	// 更新已有文件
	if _, err := os.Stat(xmpPath); err == nil {
		existingData, err := os.ReadFile(xmpPath)
		if err == nil {
			doc = etree.NewDocument()
			if err := doc.ReadFromBytes(existingData); err == nil {
				for _, rdf := range doc.FindElements("//RDF") {
					for _, desc := range rdf.FindElements("Description") {
						createOrUpdateElement(desc, "xmp:Rating", strconv.Itoa(data.Rating()))
						createOrUpdateElement(desc, "MicrosoftPhoto:Rating", strconv.Itoa(data.Rating()))
						createOrUpdateElement(desc, "imagefunnel:Action", data.Action())
						createOrUpdateElement(desc, "imagefunnel:Timestamp", data.Timestamp().Format(time.RFC3339))

					}
				}
				return writeXMPFile(doc, xmpPath)
			}
		}
	}

	// 创建新文件
	doc = etree.NewDocument()
	doc.CreateProcInst("xml", `version="1.0" encoding="UTF-8"`)

	xmpmeta := doc.CreateElement("x:xmpmeta")
	xmpmeta.CreateAttr("xmlns:x", "adobe:ns:meta/")
	rdf := xmpmeta.CreateElement("rdf:RDF")
	rdf.CreateAttr("xmlns:rdf", RDFNamespace)
	rdf.CreateAttr("xmlns:xmp", XMPNamespace)
	rdf.CreateAttr("xmlns:imagefunnel", ImageFunnelNS)
	rdf.CreateAttr("xmlns:rdfs", "http://www.w3.org/2000/01/rdf-schema#")
	rdf.CreateAttr("xmlns:dc", "http://purl.org/dc/elements/1.1/")
	rdf.CreateAttr("xmlns:exif", "http://ns.adobe.com/exif/1.0/")
	rdf.CreateAttr("xmlns:MicrosoftPhoto", MicrosoftPhotoNS)

	desc := rdf.CreateElement("rdf:Description")
	desc.CreateAttr("rdf:about", "")
	createOrUpdateElement(desc, "xmp:Rating", strconv.Itoa(data.Rating()))
	createOrUpdateElement(desc, "MicrosoftPhoto:Rating", strconv.Itoa(data.Rating()))
	createOrUpdateElement(desc, "imagefunnel:Action", data.Action())
	createOrUpdateElement(desc, "imagefunnel:Timestamp", data.Timestamp().Format(time.RFC3339))

	return writeXMPFile(doc, xmpPath)
}

type XMPData struct {
	rating    int
	action    string
	timestamp time.Time
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

func createOrUpdateElement(parent *etree.Element, tag, value string) {
	parts := strings.Split(tag, ":")
	if len(parts) == 2 {
		prefix := parts[0]
		localName := parts[1]
		attrKey := prefix + ":" + localName
		if attr := parent.SelectAttr(attrKey); attr != nil {
			attr.Value = value
			return
		}
	}

	if child := parent.FindElement(tag); child != nil {
		child.SetText(value)
	} else {
		child := parent.CreateElement(tag)
		child.SetText(value)
	}
}

func writeXMPFile(doc *etree.Document, path string) error {
	doc.Indent(2)
	output, err := doc.WriteToString()
	if err != nil {
		return fmt.Errorf("failed to marshal XMP: %w", err)
	}

	err = util.AtomicSave(path, func(file *os.File) error {
		_, err := file.WriteString(output)
		return err
	}, util.AtomicSaveWithBackupSuffix("~"))
	if err != nil {
		return fmt.Errorf("failed to write XMP file: %w", err)
	}

	return nil
}

var _ metadata.Repository = (*Repository)(nil)
