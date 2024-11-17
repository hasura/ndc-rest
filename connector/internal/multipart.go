package internal

import (
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"

	"github.com/hasura/ndc-http/ndc-http-schema/schema"
	"github.com/hasura/ndc-sdk-go/utils"
)

var quoteEscaper = strings.NewReplacer("\\", "\\\\", `"`, "\\\"")

func escapeQuotes(s string) string {
	return quoteEscaper.Replace(s)
}

// MultipartWriter extends multipart.Writer with helpers
type MultipartWriter struct {
	*multipart.Writer
}

// NewMultipartWriter creates a MultipartWriter instance
func NewMultipartWriter(w io.Writer) *MultipartWriter {
	return &MultipartWriter{multipart.NewWriter(w)}
}

// WriteDataURI write a file from data URI string
func (w *MultipartWriter) WriteDataURI(name string, value any, headers http.Header) error {
	b64, err := utils.DecodeString(value)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	dataURI, err := DecodeDataURI(b64)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}

	h := make(textproto.MIMEHeader)
	for key, header := range headers {
		h[key] = header
	}
	h.Set("Content-Disposition",
		fmt.Sprintf(`form-data; name="%s"; filename="%s"`,
			escapeQuotes(name), escapeQuotes(name)))

	if dataURI.MediaType == "" {
		h.Set("Content-Type", "application/octet-stream")
	} else {
		h.Set("Content-Type", dataURI.MediaType)
	}

	p, err := w.CreatePart(h)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}

	_, err = p.Write([]byte(dataURI.Data))

	return err
}

// WriteField calls CreateFormField and then writes the given value with json encoding.
func (w *MultipartWriter) WriteJSON(fieldName string, value any, headers http.Header) error {
	bs, err := json.Marshal(value)
	if err != nil {
		return err
	}

	h := createFieldMIMEHeader(fieldName, headers)
	h.Set(schema.ContentTypeHeader, schema.ContentTypeJSON)
	p, err := w.CreatePart(h)
	if err != nil {
		return err
	}

	_, err = p.Write(bs)

	return err
}

// WriteField calls CreateFormField and then writes the given value.
func (w *MultipartWriter) WriteField(fieldName, value string, headers http.Header) error {
	h := createFieldMIMEHeader(fieldName, headers)
	p, err := w.CreatePart(h)
	if err != nil {
		return err
	}
	_, err = p.Write([]byte(value))

	return err
}

func createFieldMIMEHeader(fieldName string, headers http.Header) textproto.MIMEHeader {
	h := make(textproto.MIMEHeader)
	for key, header := range headers {
		h[key] = header
	}
	h.Set("Content-Disposition",
		fmt.Sprintf(`form-data; name="%s"`, escapeQuotes(fieldName)))

	return h
}
