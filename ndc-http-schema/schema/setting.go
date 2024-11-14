package schema

import (
	"encoding/json"
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

	headers map[string]string
}

// UnmarshalJSON implements json.Unmarshaler.
func (j *NDCHttpSettings) UnmarshalJSON(b []byte) error {
	type Plain NDCHttpSettings

	var raw Plain
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}

	result := NDCHttpSettings(raw)
	_ = result.Validate()

	*j = result
	return nil
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

	headers, err := getHeadersFromEnv(rs.Headers)
	if err != nil {
		return err
	}
	rs.headers = headers

	return nil
}

// Validate if the current instance is valid
func (rs NDCHttpSettings) GetHeaders() map[string]string {
	if rs.headers != nil {
		return rs.headers
	}

	return getHeadersFromEnvUnsafe(rs.Headers)
}

// ServerConfig contains server configurations
type ServerConfig struct {
	URL             utils.EnvString            `json:"url"                       mapstructure:"url"             yaml:"url"`
	ID              string                     `json:"id,omitempty"              mapstructure:"id"              yaml:"id,omitempty"`
	Headers         map[string]utils.EnvString `json:"headers,omitempty"         mapstructure:"headers"         yaml:"headers,omitempty"`
	SecuritySchemes map[string]SecurityScheme  `json:"securitySchemes,omitempty" mapstructure:"securitySchemes" yaml:"securitySchemes,omitempty"`
	Security        AuthSecurities             `json:"security,omitempty"        mapstructure:"security"        yaml:"security,omitempty"`
	TLS             *TLSConfig                 `json:"tls,omitempty"             mapstructure:"tls"             yaml:"tls,omitempty"`

	// cached values that are loaded from environment variables
	url     *url.URL
	headers map[string]string
}

// UnmarshalJSON implements json.Unmarshaler.
func (j *ServerConfig) UnmarshalJSON(b []byte) error {
	type Plain ServerConfig

	var raw Plain
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}

	result := ServerConfig(raw)
	_ = result.Validate()

	*j = result

	return nil
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

	urlValue, err := parseHttpURL(rawURL)
	if err != nil {
		return fmt.Errorf("server url: %w", err)
	}

	ss.url = urlValue

	headers, err := getHeadersFromEnv(ss.Headers)
	if err != nil {
		return err
	}
	ss.headers = headers

	return nil
}

// Validate if the current instance is valid
func (ss ServerConfig) GetURL() (url.URL, error) {
	if ss.url != nil {
		return *ss.url, nil
	}

	rawURL, err := ss.URL.Get()
	if err != nil {
		return url.URL{}, err
	}
	urlValue, err := parseHttpURL(rawURL)
	if err != nil {
		return url.URL{}, fmt.Errorf("server url: %w", err)
	}

	return *urlValue, nil
}

// Validate if the current instance is valid
func (ss ServerConfig) GetHeaders() map[string]string {
	if ss.headers != nil {
		return ss.headers
	}

	return getHeadersFromEnvUnsafe(ss.Headers)
}

// parseHttpURL parses and validate if the URL has HTTP scheme
func parseHttpURL(input string) (*url.URL, error) {
	if !strings.HasPrefix(input, "https://") && !strings.HasPrefix(input, "http://") {
		return nil, errors.New("invalid HTTP URL " + input)
	}

	return url.Parse(input)
}

func parseRelativeOrHttpURL(input string) (*url.URL, error) {
	if strings.HasPrefix(input, "/") {
		return &url.URL{Path: input}, nil
	}
	return parseHttpURL(input)
}

// TLSConfig represents the transport layer security (LTS) configuration for the mutualTLS authentication
type TLSConfig struct {
	// Path to the TLS cert to use for TLS required connections.
	CertFile *utils.EnvString `json:"certFile,omitempty" mapstructure:"certFile" yaml:"certFile,omitempty"`
	// Alternative to cert_file. Provide the certificate contents as a string instead of a filepath.
	CertPem *utils.EnvString `json:"certPem,omitempty" mapstructure:"certPem" yaml:"certPem,omitempty"`
	// Path to the TLS key to use for TLS required connections.
	KeyFile *utils.EnvString `json:"keyFile,omitempty" mapstructure:"keyFile" yaml:"keyFile,omitempty"`
	// Alternative to key_file. Provide the key contents as a string instead of a filepath.
	KeyPem *utils.EnvString `json:"keyPem,omitempty" mapstructure:"keyPem" yaml:"keyPem,omitempty"`
	// Path to the CA cert. For a client this verifies the server certificate. For a server this verifies client certificates.
	// If empty uses system root CA.
	CAFile *utils.EnvString `json:"caFile,omitempty" mapstructure:"caFile" yaml:"caFile,omitempty"`
	// Alternative to ca_file. Provide the CA cert contents as a string instead of a filepath.
	CAPem *utils.EnvString `json:"caPem,omitempty" mapstructure:"caPem" yaml:"caPem,omitempty"`
	// Additionally you can configure TLS to be enabled but skip verifying the server's certificate chain.
	InsecureSkipVerify *utils.EnvBool `json:"insecureSkipVerify,omitempty" mapstructure:"insecureSkipVerify" yaml:"insecureSkipVerify,omitempty"`
	// Whether to load the system certificate authorities pool alongside the certificate authority.
	IncludeSystemCACertsPool *utils.EnvBool `json:"includeSystemCACertsPool,omitempty" mapstructure:"includeSystemCACertsPool" yaml:"includeSystemCACertsPool,omitempty"`
	// Minimum acceptable TLS version.
	MinVersion *utils.EnvString `json:"minVersion,omitempty" mapstructure:"minVersion" yaml:"minVersion,omitempty"`
	// Maximum acceptable TLS version.
	MaxVersion *utils.EnvString `json:"maxVersion,omitempty" mapstructure:"maxVersion" yaml:"maxVersion,omitempty"`
	// Explicit cipher suites can be set. If left blank, a safe default list is used.
	// See https://go.dev/src/crypto/tls/cipher_suites.go for a list of supported cipher suites.
	CipherSuites []string `json:"cipherSuites,omitempty" mapstructure:"cipherSuites" yaml:"cipherSuites,omitempty"`
	// Specifies the duration after which the certificate will be reloaded. If not set, it will never be reloaded.
	// The interval unit is minute
	ReloadInterval *utils.EnvInt `json:"reloadInterval,omitempty" mapstructure:"reloadInterval" yaml:"reloadInterval,omitempty"`
}

// Validate if the current instance is valid
func (ss TLSConfig) Validate() error {
	return nil
}

func getHeadersFromEnv(headers map[string]utils.EnvString) (map[string]string, error) {
	results := make(map[string]string)
	for key, header := range headers {
		value, err := header.Get()
		if err != nil {
			return nil, fmt.Errorf("headers[%s]: %w", key, err)
		}
		if value != "" {
			results[key] = value
		}
	}

	return results, nil
}

func getHeadersFromEnvUnsafe(headers map[string]utils.EnvString) map[string]string {
	results := make(map[string]string)
	for key, header := range headers {
		value, _ := header.Get()
		if value != "" {
			results[key] = value
		}
	}

	return results
}
