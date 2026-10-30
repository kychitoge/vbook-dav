package webdav

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"strings"

	"vbook-dav/internal/storage"
)

// MultiStatus represents the root tag <d:multistatus xmlns:d="DAV:">.
type MultiStatus struct {
	XMLName   xml.Name   `xml:"d:multistatus"`
	XmlnsD    string     `xml:"xmlns:d,attr"`
	Responses []Response `xml:"d:response"`
}

// Response represents the <d:response> XML element.
type Response struct {
	Href     string   `xml:"d:href"`
	Propstat Propstat `xml:"d:propstat"`
}

// Propstat represents the <d:propstat> XML element.
type Propstat struct {
	Prop   Prop   `xml:"d:prop"`
	Status string `xml:"d:status"`
}

// ResourceType represents the <d:resourcetype> XML element.
type ResourceType struct {
	Collection *struct{} `xml:"d:collection,omitempty"`
}

// Prop represents the <d:prop> element containing RFC 4918 metadata.
type Prop struct {
	DisplayName  string       `xml:"d:displayname,omitempty"`
	ResourceType ResourceType `xml:"d:resourcetype"`
	ContentLen   *int64       `xml:"d:getcontentlength,omitempty"`
	LastModified string       `xml:"d:getlastmodified,omitempty"`
	ContentType  string       `xml:"d:getcontenttype,omitempty"`
	ETag         string       `xml:"d:getetag,omitempty"`
}

// NewMultiStatus initializes a MultiStatus element with standard DAV: namespace.
func NewMultiStatus() *MultiStatus {
	return &MultiStatus{
		XmlnsD:    "DAV:",
		Responses: make([]Response, 0),
	}
}

// AddFileItem converts storage.FileItem into a WebDAV XML response entry.
func (m *MultiStatus) AddFileItem(item storage.FileItem) {
	href := item.Path
	if !strings.HasPrefix(href, "/") {
		href = "/" + href
	}

	// Ensure collections end with a trailing slash per RFC 4918
	if item.IsDir && !strings.HasSuffix(href, "/") {
		href += "/"
	}

	name := item.Name
	if name == "" || name == "/" {
		name = "root"
	}

	res := Response{
		Href: href,
		Propstat: Propstat{
			Status: "HTTP/1.1 200 OK",
			Prop: Prop{
				DisplayName:  name,
				LastModified: item.ModTime.UTC().Format(http.TimeFormat),
				ETag:         item.ETag,
			},
		},
	}

	if item.IsDir {
		res.Propstat.Prop.ResourceType = ResourceType{
			Collection: &struct{}{},
		}
	} else {
		size := item.Size
		res.Propstat.Prop.ContentLen = &size
		res.Propstat.Prop.ContentType = item.ContentType
	}

	m.Responses = append(m.Responses, res)
}

// RenderXML serializes the MultiStatus element into XML with a standard XML declaration header.
func (m *MultiStatus) RenderXML() ([]byte, error) {
	data, err := xml.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to encode multistatus XML: %w", err)
	}

	output := append([]byte(xml.Header), data...)
	return output, nil
}
