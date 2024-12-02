package schema

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/hasura/ndc-sdk-go/utils"
)

// NDCHttpSettings represent global settings of the HTTP API, including base URL, headers, etc...
type NDCHttpSettings struct {
	Servers         []ServerConfig             `json:"servers"                   mapstructure:"servers"         yaml:"servers"`
	Headers         map[string]utils.EnvString `json:"headers,omitempty"         mapstructure:"headers"         yaml:"headers,omitempty"`
	SecuritySchemes map[string]SecurityScheme  `json:"securitySchemes,omitempty" mapstructure:"securitySchemes" yaml:"securitySchemes,omitempty"`
	Security        AuthSecurities             `json:"security,omitempty"        mapstructure:"security"        yaml:"security,omitempty"`
	Version         string                     `json:"version,omitempty"         mapstructure:"version"         yaml:"version,omitempty"`
	TLS             *TLSConfig                 `json:"tls,omitempty"             mapstructure:"tls"             yaml:"tls,omitempty"`
}

// Validate if the current instance is valid
func (rs *NDCHttpSettings) Validate() error {
	for _, server := range rs.Servers {
		if err := server.Validate(); err != nil {
			return err
		}
	}

	for key, scheme := range rs.SecuritySchemes {
		if err := scheme.Validate(); err != nil {
			return fmt.Errorf("securityScheme %s: %w", key, err)
		}
	}

	if rs.TLS != nil {
		if err := rs.TLS.Validate(); err != nil {
			return err
		}
	}

	return nil
}

// ServerConfig contains server configurations.
type ServerConfig struct {
	URL             utils.EnvString            `json:"url"                       mapstructure:"url"             yaml:"url"`
	ID              string                     `json:"id,omitempty"              mapstructure:"id"              yaml:"id,omitempty"`
	Headers         map[string]utils.EnvString `json:"headers,omitempty"         mapstructure:"headers"         yaml:"headers,omitempty"`
	SecuritySchemes map[string]SecurityScheme  `json:"securitySchemes,omitempty" mapstructure:"securitySchemes" yaml:"securitySchemes,omitempty"`
	Security        AuthSecurities             `json:"security,omitempty"        mapstructure:"security"        yaml:"security,omitempty"`
	TLS             *TLSConfig                 `json:"tls,omitempty"             mapstructure:"tls"             yaml:"tls,omitempty"`
}

// Validate if the current instance is valid
func (ss *ServerConfig) Validate() error {
	rawURL, err := ss.URL.Get()
	if err != nil {
		return fmt.Errorf("server url: %w", err)
	}

	if rawURL == "" {
		return errors.New("url is required for server")
	}

	_, err = parseHttpURL(rawURL)
	if err != nil {
		return fmt.Errorf("server url: %w", err)
	}

	if ss.TLS != nil {
		if err := ss.TLS.Validate(); err != nil {
			return fmt.Errorf("tls: %w", err)
		}
	}

	return nil
}

// Validate if the current instance is valid
func (ss ServerConfig) GetURL() (*url.URL, error) {
	rawURL, err := ss.URL.Get()
	if err != nil {
		return nil, err
	}
	urlValue, err := parseHttpURL(rawURL)
	if err != nil {
		return nil, fmt.Errorf("server url: %w", err)
	}

	return urlValue, nil
}

// parseHttpURL parses and validate if the URL has HTTP scheme
func parseHttpURL(input string) (*url.URL, error) {
	if !strings.HasPrefix(input, "https://") && !strings.HasPrefix(input, "http://") {
		return nil, errors.New("invalid HTTP URL " + input)
	}

	return url.Parse(input)
}

func ParseRelativeOrHttpURL(input string) (*url.URL, error) {
	if strings.HasPrefix(input, "/") {
		return &url.URL{Path: input}, nil
	}

	return parseHttpURL(input)
}
