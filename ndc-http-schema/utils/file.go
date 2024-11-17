package utils

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/hasura/ndc-http/ndc-http-schema/schema"
	"gopkg.in/yaml.v3"
)

// MarshalSchema encodes the NDC HTTP schema to bytes
func MarshalSchema(content any, format schema.SchemaFileFormat) ([]byte, error) {
	var fileBuffer bytes.Buffer
	switch format {
	case schema.SchemaFileJSON:
		encoder := json.NewEncoder(&fileBuffer)
		encoder.SetIndent("", "  ")

		if err := encoder.Encode(content); err != nil {
			return nil, fmt.Errorf("failed to encode NDC HTTP schema: %w", err)
		}
	case schema.SchemaFileYAML:
		encoder := yaml.NewEncoder(&fileBuffer)
		encoder.SetIndent(2)
		if err := encoder.Encode(content); err != nil {
			return nil, fmt.Errorf("failed to encode NDC HTTP schema: %w", err)
		}
	default:
		return nil, errors.New("invalid schema file format. Accept json or yaml")
	}

	return fileBuffer.Bytes(), nil
}

// WriteSchemaFile writes the NDC HTTP schema to file
func WriteSchemaFile(outputPath string, content any) error {
	format, err := schema.ParseSchemaFileFormat(strings.TrimLeft(filepath.Ext(outputPath), "."))
	if err != nil {
		return err
	}

	rawBytes, err := MarshalSchema(content, format)
	if err != nil {
		return err
	}

	return os.WriteFile(outputPath, rawBytes, 0664)
}

// ReadFileFromPath read file content from either file path or URL
func ReadFileFromPath(filePath string) ([]byte, error) {
	var result []byte

	fileURL, err := url.Parse(filePath)
	if err == nil && slices.Contains([]string{"http", "https"}, strings.ToLower(fileURL.Scheme)) {
		resp, err := http.DefaultClient.Get(filePath)
		if err != nil {
			return nil, err
		}

		if resp.Body != nil {
			defer resp.Body.Close()
			result, err = io.ReadAll(resp.Body)
			if err != nil {
				return nil, fmt.Errorf("failed to read content from %s: %w", filePath, err)
			}
		}
		if resp.StatusCode != http.StatusOK {
			errorMsg := string(result)
			if errorMsg == "" {
				errorMsg = resp.Status
			}

			return nil, fmt.Errorf("failed to download file from %s: %s", filePath, errorMsg)
		}
	} else {
		result, err = os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to read content from %s: %w", filePath, err)
		}
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("failed to read file from %s: no content", filePath)
	}

	return result, nil
}

// WalkFiles read one file or many files in a folder if the file path is a directory
func WalkFiles(filePath string, callback func(data []byte) error) error {
	fileURL, err := url.Parse(filePath)
	if err == nil && slices.Contains([]string{"http", "https"}, strings.ToLower(fileURL.Scheme)) {
		resp, err := http.DefaultClient.Get(filePath)
		if err != nil {
			return err
		}

		var result []byte
		if resp.Body != nil {
			defer resp.Body.Close()
			result, err = io.ReadAll(resp.Body)
			if err != nil {
				return fmt.Errorf("failed to read content from %s: %w", filePath, err)
			}
		}
		if resp.StatusCode != http.StatusOK {
			errorMsg := string(result)
			if errorMsg == "" {
				errorMsg = resp.Status
			}

			return fmt.Errorf("failed to download file from %s: %s", filePath, errorMsg)
		}
		if len(result) == 0 {
			return fmt.Errorf("failed to read file from %s: no content", filePath)
		}

		return callback(result)
	}

	stat, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("failed to read content from %s: %w", filePath, err)
	}

	readFunc := func(p string) error {
		result, err := os.ReadFile(p)
		if err != nil {
			return fmt.Errorf("failed to read content from %s: %w", p, err)
		}
		if len(result) == 0 {
			return fmt.Errorf("failed to read file from %s: no content", p)
		}

		return callback(result)
	}

	if !stat.IsDir() {
		return readFunc(filePath)
	}

	return filepath.WalkDir(filePath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// only read files in the root folder
		if d.IsDir() {
			return nil
		}

		return readFunc(path)
	})
}

// ResolveFilePath resolves file path with directory
func ResolveFilePath(dir string, filePath string) string {
	if !strings.HasPrefix(filePath, "/") && !strings.HasPrefix(filePath, "\\") && !strings.HasPrefix(filePath, "http") {
		return path.Join(dir, filePath)
	}

	return filePath
}
