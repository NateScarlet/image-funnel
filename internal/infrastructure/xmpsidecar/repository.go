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
	ImageFunnelNS    = "https://github.com/NateScarlet/image-funnel/ns/1.0/"
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

	localResult := &XMPData{rating: 0}
	for _, rdf := range doc.FindElements("//RDF/Description") {
		// 优先读取标准 XMP 评分
		var ratingVal int
		foundRating := false

		// 1. 尝试查找 xmp:Rating
		if valStr, ok := getValueByNamespace(rdf, XMPNamespace, "Rating"); ok {
			if val, err := strconv.Atoi(valStr); err == nil {
				ratingVal = val
				foundRating = true
			}
		}

		// 2. 如果没找到，尝试查找 MicrosoftPhoto:Rating
		if !foundRating {
			if valStr, ok := getValueByNamespace(rdf, MicrosoftPhotoNS, "Rating"); ok {
				if val, err := strconv.Atoi(valStr); err == nil {
					ratingVal = fromMicrosoftRating(val)
					foundRating = true
				}
			}
		}

		if foundRating {
			localResult.rating = ratingVal
		}

		if valStr, ok := getValueByNamespace(rdf, ImageFunnelNS, "Action"); ok {
			localResult.action = valStr
		}

		if valStr, ok := getValueByNamespace(rdf, ImageFunnelNS, "Timestamp"); ok {
			if val, err := time.Parse(time.RFC3339, valStr); err == nil {
				localResult.timestamp = val
			}
		}
	}

	return metadata.NewXMPData(
		localResult.rating,
		localResult.action,
		localResult.timestamp,
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
	// Ensure x:xmptk attribute
	if attr := xmpmeta.SelectAttr("x:xmptk"); attr != nil {
		attr.Value = "XMP Core 6.0.0"
	} else {
		xmpmeta.CreateAttr("x:xmptk", "XMP Core 6.0.0")
	}

	rdf := xmpmeta.FindElement("rdf:RDF")
	if rdf == nil {
		rdf = xmpmeta.CreateElement("rdf:RDF")
	}

	// 确保 rdf 命名空间声明在 rdf:RDF 上
	ensureNamespace(rdf, "rdf", RDFNamespace)

	// 移除 rdf:RDF 上的其他命名空间（如果存在），以符合“仅在 Description 上定义”的要求
	rdf.RemoveAttr("xmlns:xmp")
	rdf.RemoveAttr("xmlns:ImageFunnel")
	rdf.RemoveAttr("xmlns:MicrosoftPhoto")

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

	// 确保命名空间定义在 rdf:Description 上
	ensureNamespace(desc, "xmp", XMPNamespace)
	ensureNamespace(desc, "ImageFunnel", ImageFunnelNS)
	ensureNamespace(desc, "MicrosoftPhoto", MicrosoftPhotoNS)

	// 更新字段
	setValueByNamespace(desc, XMPNamespace, "xmp", "Rating", strconv.Itoa(data.Rating()))
	setValueByNamespace(desc, MicrosoftPhotoNS, "MicrosoftPhoto", "Rating", strconv.Itoa(toMicrosoftRating(data.Rating())))
	setValueByNamespace(desc, ImageFunnelNS, "ImageFunnel", "Action", data.Action())
	setValueByNamespace(desc, ImageFunnelNS, "ImageFunnel", "Timestamp", data.Timestamp().Format(time.RFC3339))

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

func resolveNamespace(elem *etree.Element, prefix string) string {
	attrKey := "xmlns"
	if prefix != "" {
		attrKey = "xmlns:" + prefix
	}

	curr := elem
	for curr != nil {
		if attr := curr.SelectAttr(attrKey); attr != nil {
			return attr.Value
		}
		curr = curr.Parent()
	}
	return ""
}

func splitTag(tag string) (string, string) {
	parts := strings.SplitN(tag, ":", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "", tag
}

func getValueByNamespace(elem *etree.Element, nsURL, localName string) (string, bool) {
	// 1. Check attributes
	for _, attr := range elem.Attr {
		prefix := attr.Space
		local := attr.Key
		if prefix == "" {
			p, l := splitTag(attr.Key)
			if p != "" {
				prefix = p
				local = l
			}
		}

		var attrNS string
		if prefix != "" {
			attrNS = resolveNamespace(elem, prefix)
		}

		if local == localName && attrNS == nsURL {
			return attr.Value, true
		}
	}

	// 2. Check children
	for _, child := range elem.ChildElements() {
		if child.Tag == localName {
			childNS := resolveNamespace(child, child.Space)
			if childNS == nsURL {
				return child.Text(), true
			}
		}
	}

	return "", false
}

func setValueByNamespace(elem *etree.Element, nsURL, preferredPrefix, localName, value string) {
	// 1. Try to find existing attribute and update it
	// We iterate to find the attribute that matches key/ns
	for i, attr := range elem.Attr {
		prefix := attr.Space
		local := attr.Key
		if prefix == "" {
			p, l := splitTag(attr.Key)
			if p != "" {
				prefix = p
				local = l
			}
		}

		var attrNS string
		if prefix != "" {
			attrNS = resolveNamespace(elem, prefix)
		}

		if local == localName && attrNS == nsURL {
			elem.Attr[i].Value = value
			return
		}
	}

	// 2. Try to find existing child and update it
	for _, child := range elem.ChildElements() {
		if child.Tag == localName {
			childNS := resolveNamespace(child, child.Space)
			if childNS == nsURL {
				child.SetText(value)
				return
			}
		}
	}

	// 3. Create new child
	tagName := localName
	if preferredPrefix != "" {
		tagName = preferredPrefix + ":" + localName
	}
	child := elem.CreateElement(tagName)
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

func toMicrosoftRating(rating int) int {
	switch rating {
	case 0:
		return 0
	case 1:
		return 1
	case 2:
		return 25
	case 3:
		return 50
	case 4:
		return 75
	case 5:
		return 100
	default:
		// Clamp to valid range
		if rating < 0 {
			return 0
		}
		return 100
	}
}

func fromMicrosoftRating(msRating int) int {
	switch {
	case msRating <= 0:
		return 0
	case msRating <= 12:
		return 1
	case msRating <= 37:
		return 2
	case msRating <= 62:
		return 3
	case msRating <= 87:
		return 4
	default:
		return 5
	}
}
