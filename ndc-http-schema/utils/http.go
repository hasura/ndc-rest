package utils

import (
	"strings"

	"github.com/hasura/ndc-http/ndc-http-schema/schema"
)

// IsContentTypeJSON checks if the content type is JSON
func IsContentTypeJSON(contentType string) bool {
	return contentType == schema.ContentTypeJSON || strings.HasSuffix(contentType, "+json")
}

// IsContentTypeXML checks if the content type is XML
func IsContentTypeXML(contentType string) bool {
	return contentType == schema.ContentTypeXML || strings.HasSuffix(contentType, "+xml")
}

// IsContentTypeText checks if the content type relates to text
func IsContentTypeText(contentType string) bool {
	return strings.HasPrefix(contentType, "text/") || strings.HasPrefix(contentType, "image/svg")
}

// IsContentTypeText checks if the content type relates to binary
func IsContentTypeBinary(contentType string) bool {
	return strings.HasPrefix(contentType, "application/") || strings.HasPrefix(contentType, "image/") || strings.HasPrefix(contentType, "video/")
}

// IsContentTypeMultipartForm checks the content type relates to multipart form.
func IsContentTypeMultipartForm(contentType string) bool {
	return strings.HasPrefix(contentType, "multipart/")
}
