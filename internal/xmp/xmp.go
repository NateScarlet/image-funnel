package xmp

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"main/internal/util"

	"github.com/beevik/etree"
)

const (
	XMPNamespace     = "http://ns.adobe.com/xap/1.0/"
	RDFNamespace     = "http://www.w3.org/1999/02/22-rdf-syntax-ns#"
	ImageFunnelNS    = "http://ns.imagefunnel.app/1.0/"
	MicrosoftPhotoNS = "http://ns.microsoft.com/photo/1.0/"
)

type XMPData struct {
	Rating    int
	Action    string
	SessionID string
	Timestamp time.Time
	Preset    string
}

func Read(imagePath string) (*XMPData, error) {
	xmpPath := imagePath + ".xmp"

	data, err := os.ReadFile(xmpPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &XMPData{Rating: 0}, nil
		}
		return nil, fmt.Errorf("failed to read XMP file: %w", err)
	}

	doc := etree.NewDocument()
	if err := doc.ReadFromBytes(data); err != nil {
		return nil, fmt.Errorf("failed to parse XMP: %w", err)
	}

	result := &XMPData{Rating: 0}

	for _, rdf := range doc.FindElements("//RDF/Description") {
		rating := findElementText(rdf, []string{"xmp:Rating", "Rating", "MicrosoftPhoto:Rating"})
		if rating != "" {
			if val, err := strconv.Atoi(rating); err == nil {
				result.Rating = val
			}
		}

		action := findElementText(rdf, []string{"imagefunnel:Action", "Action"})
		if action != "" {
			result.Action = action
		}

		sessionID := findElementText(rdf, []string{"imagefunnel:SessionID", "SessionID"})
		if sessionID != "" {
			result.SessionID = sessionID
		}

		timestamp := findElementText(rdf, []string{"imagefunnel:Timestamp", "Timestamp"})
		if timestamp != "" {
			if val, err := time.Parse(time.RFC3339, timestamp); err == nil {
				result.Timestamp = val
			}
		}

		preset := findElementText(rdf, []string{"imagefunnel:Preset", "Preset"})
		if preset != "" {
			result.Preset = preset
		}
	}

	return result, nil
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

func Write(imagePath string, data *XMPData) error {
	xmpPath := imagePath + ".xmp"

	var doc *etree.Document

	if _, err := os.Stat(xmpPath); err == nil {
		existingData, err := os.ReadFile(xmpPath)
		if err == nil {
			doc = etree.NewDocument()
			if err := doc.ReadFromBytes(existingData); err == nil {
				if err := updateExistingXMP(doc, data); err != nil {
					return err
				}
				return writeXMP(doc, xmpPath)
			}
		}
	}

	doc = createNewXMP(data)
	return writeXMP(doc, xmpPath)
}

func createNewXMP(data *XMPData) *etree.Document {
	doc := etree.NewDocument()
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

	createOrUpdateElement(desc, "xmp:Rating", strconv.Itoa(data.Rating))
	createOrUpdateElement(desc, "MicrosoftPhoto:Rating", strconv.Itoa(data.Rating))
	createOrUpdateElement(desc, "imagefunnel:Action", data.Action)
	createOrUpdateElement(desc, "imagefunnel:Timestamp", data.Timestamp.Format(time.RFC3339))
	createOrUpdateElement(desc, "imagefunnel:Preset", data.Preset)

	return doc
}

func updateExistingXMP(doc *etree.Document, data *XMPData) error {
	for _, rdf := range doc.FindElements("//RDF") {
		for _, desc := range rdf.FindElements("Description") {
			createOrUpdateElement(desc, "xmp:Rating", strconv.Itoa(data.Rating))
			createOrUpdateElement(desc, "MicrosoftPhoto:Rating", strconv.Itoa(data.Rating))
			createOrUpdateElement(desc, "imagefunnel:Action", data.Action)
			createOrUpdateElement(desc, "imagefunnel:Timestamp", data.Timestamp.Format(time.RFC3339))
			createOrUpdateElement(desc, "imagefunnel:Preset", data.Preset)
		}
	}
	return nil
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

func writeXMP(doc *etree.Document, path string) error {
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

func BatchWrite(imagePaths []string, dataMap map[string]*XMPData) (int, []error) {
	success := 0
	var errors []error

	for _, path := range imagePaths {
		data, exists := dataMap[path]
		if !exists {
			continue
		}

		if err := Write(path, data); err != nil {
			errors = append(errors, fmt.Errorf("%s: %w", path, err))
			continue
		}
		success++
	}

	return success, errors
}

func GetXMPPath(imagePath string) string {
	return imagePath + ".xmp"
}

func IsSupportedImage(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	return ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".webp" || ext == ".avif"
}
