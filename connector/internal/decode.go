package internal

import (
	"encoding/base64"
	"fmt"
	"mime"
	"net/url"
	"strings"
)

const (
	// EncodingBase64 is base64 encoding for the data url
	EncodingBase64 = "base64"
	// EncodingASCII is ascii encoding for the data url
	EncodingASCII = "ascii"
)

// DataURI represents the Data URI scheme
//
// [Data URI]: https://en.wikipedia.org/wiki/Data_URI_scheme
type DataURI struct {
	MediaType  string
	Parameters map[string]string
	Data       string
}

// DecodeDataURI decodes data URI scheme
// data:[<media type>][;<key>=<value>][;<extension>],<data>
func DecodeDataURI(input string) (*DataURI, error) {
	rawDataURI, ok := strings.CutPrefix(input, "data:")
	if !ok {
		// without data URI, decode base64 by default
		rawDecodedBytes, err := base64.StdEncoding.DecodeString(input)
		if err != nil {
			return nil, err
		}
		return &DataURI{
			Data: string(rawDecodedBytes),
		}, nil
	}

	uriParts := strings.Split(rawDataURI, ",")
	if len(uriParts) < 2 || uriParts[1] == "" {
		return nil, fmt.Errorf("invalid data uri: %s", rawDataURI)
	}

	mediaTypes := strings.Split(uriParts[0], ";")
	dataURI := &DataURI{}

	switch strings.TrimSpace(mediaTypes[len(mediaTypes)-1]) {
	case EncodingBase64:
		rawDecodedBytes, err := base64.StdEncoding.DecodeString(uriParts[1])
		if err != nil {
			return nil, err
		}
		dataURI.Data = string(rawDecodedBytes)
		mediaTypes = mediaTypes[:len(mediaTypes)-1]
	case EncodingASCII:
		dataURI.Data = url.PathEscape(uriParts[1])
		mediaTypes = mediaTypes[:len(mediaTypes)-1]
	default:
		dataURI.Data = url.PathEscape(uriParts[1])
	}

	rawMediaType := strings.Join(mediaTypes, ";")
	mediaType, params, err := mime.ParseMediaType(rawMediaType)
	if err != nil {
		return nil, fmt.Errorf("%w %s", err, rawMediaType)
	}
	dataURI.MediaType = mediaType
	dataURI.Parameters = params

	return dataURI, nil
}
