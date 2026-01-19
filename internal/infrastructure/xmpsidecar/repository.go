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

// #region Write
func (r *Repository) Write(imagePath string, data *metadata.XMPData) error {
	xmpPath := imagePath + ".xmp"

	doc := etree.NewDocument()
	// 加载已有文件
	if existingData, err := os.ReadFile(xmpPath); err == nil {
		if err := doc.ReadFromBytes(existingData); err != nil {
			// 如果解析失败，备份原文件并创建新文档
			backupPath := fmt.Sprintf("%s.broken%d", xmpPath, time.Now().UnixNano())
			if renameErr := os.Rename(xmpPath, backupPath); renameErr != nil {
				return fmt.Errorf("failed to backup invalid XMP file: %w", renameErr)
			}
			doc = etree.NewDocument()
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to read existing XMP: %w", err)
	}

	// 确保基础结构存在
	if doc.FindElement("x:xmpmeta") == nil {
		doc.CreateProcInst("xml", `version="1.0" encoding="UTF-8"`)
		xmpmeta := doc.CreateElement("x:xmpmeta")
		xmpmeta.CreateAttr("xmlns:x", "adobe:ns:meta/")
	}

	xmpmeta := doc.FindElement("x:xmpmeta")
	rdf := xmpmeta.FindElement("rdf:RDF")
	if rdf == nil {
		rdf = xmpmeta.CreateElement("rdf:RDF")
	}

	// 确保所有命名空间都已声明在 rdf:RDF 上
	ensureNamespace(rdf, "rdf", RDFNamespace)
	ensureNamespace(rdf, "xmp", XMPNamespace)
	ensureNamespace(rdf, "imagefunnel", ImageFunnelNS)
	ensureNamespace(rdf, "MicrosoftPhoto", MicrosoftPhotoNS)

	// 查找或创建 Description (rdf:about="")
	desc := rdf.FindElement("rdf:Description[@rdf:about='']")
	if desc == nil {
		// 如果没找到带 about 的，就找第一个 Description 或者创建一个
		desc = rdf.FindElement("rdf:Description")
		if desc == nil {
			desc = rdf.CreateElement("rdf:Description")
			desc.CreateAttr("rdf:about", "")
		} else if desc.SelectAttr("rdf:about") == nil {
			desc.CreateAttr("rdf:about", "")
		}
	}

	// 更新字段
	createOrUpdateElement(desc, "xmp:Rating", strconv.Itoa(data.Rating()))
	createOrUpdateElement(desc, "MicrosoftPhoto:Rating", strconv.Itoa(data.Rating()))
	createOrUpdateElement(desc, "imagefunnel:Action", data.Action())
	createOrUpdateElement(desc, "imagefunnel:Timestamp", data.Timestamp().Format(time.RFC3339))

	return writeXMPFile(doc, xmpPath)
}

func ensureNamespace(elem *etree.Element, prefix, uri string) {
	attrKey := "xmlns:" + prefix
	if attr := elem.SelectAttr(attrKey); attr == nil {
		elem.CreateAttr(attrKey, uri)
	}
}

// #endregion

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
	var prefix, localName string
	if len(parts) == 2 {
		prefix = parts[0]
		localName = parts[1]
	}

	// 1. 尝试更新现有属性
	if localName != "" {
		attrKey := prefix + ":" + localName
		if attr := parent.SelectAttr(attrKey); attr != nil {
			attr.Value = value
			return
		}
	}

	// 2. 尝试更新现有子元素
	if child := parent.FindElement(tag); child != nil {
		child.SetText(value)
		return
	}

	// 3. 如果都不存在，创建一个新元素
	child := parent.CreateElement(tag)
	child.SetText(value)
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
