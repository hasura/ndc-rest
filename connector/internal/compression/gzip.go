package compression

import (
	"compress/gzip"
	"io"
)

const (
	EncodingGzip = "gzip"
)

// GzipCompressor implements the compression handler for gzip encoding.
type GzipCompressor struct{}

// Compress the reader content with gzip encoding.
func (gc GzipCompressor) Compress(w io.Writer, data []byte) (int, error) {
	zw := gzip.NewWriter(w)

	size, err := zw.Write(data)
	if err != nil {
		return size, err
	}
	err = zw.Close()

	return size, err
}

// Decompress the reader content with gzip encoding.
func (gc GzipCompressor) Decompress(reader io.ReadCloser) (io.ReadCloser, error) {
	compressionReader, err := gzip.NewReader(reader)
	if err != nil {
		return nil, err
	}

	return readCloserWrapper{
		CompressionReader: compressionReader,
		OriginalReader:    reader,
	}, nil
}
