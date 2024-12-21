package compression

import (
	"io"
	"strings"

	"github.com/hasura/ndc-sdk-go/utils"
)

// Compressor abstracts the interface for a compression handler.
type Compressor interface {
	Compress(w io.Writer, data []byte) (int, error)
	Decompress(reader io.ReadCloser) (io.ReadCloser, error)
}

type Compressors struct {
	acceptEncoding string
	compressors    map[string]Compressor
}

// NewCompressors create a Compressors instance.
func NewCompressors() *Compressors {
	compressors := map[string]Compressor{
		EncodingGzip:    GzipCompressor{},
		EncodingDeflate: DeflateCompressor{},
	}

	return &Compressors{
		acceptEncoding: strings.Join(utils.GetSortedKeys(compressors), ", "),
		compressors:    compressors,
	}
}

// AcceptEncoding returns the Accept-Encoding header with supported compression encodings.
func (c Compressors) AcceptEncoding() string {
	return c.acceptEncoding
}

// ReadAll reads the decompresses the reader with equivalent the content encoding
func (c Compressors) Decompress(reader io.ReadCloser, encoding string) (io.ReadCloser, error) {
	compressor, ok := c.compressors[strings.ToLower(strings.TrimSpace(encoding))]
	if !ok {
		return reader, nil
	}

	return compressor.Decompress(reader)
}

type readCloserWrapper struct {
	CompressionReader io.ReadCloser
	OriginalReader    io.ReadCloser
}

func (rcw readCloserWrapper) Close() error {
	_ = rcw.OriginalReader.Close()

	return rcw.CompressionReader.Close()
}

func (rcw readCloserWrapper) Read(p []byte) (n int, err error) {
	return rcw.CompressionReader.Read(p)
}
