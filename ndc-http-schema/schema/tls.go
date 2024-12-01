package schema

import (
	"crypto/tls"
	"errors"
	"fmt"

	"github.com/hasura/ndc-sdk-go/utils"
)

// We should avoid that users unknowingly use a vulnerable TLS version.
// The defaults should be a safe configuration
const defaultMinTLSVersion = tls.VersionTLS12

// Uses the default MaxVersion from "crypto/tls" which is the maximum supported version
const defaultMaxTLSVersion = 0

// TLSConfig represents the transport layer security (LTS) configuration for the mutualTLS authentication
type TLSConfig struct {
	// Path to the TLS cert to use for TLS required connections.
	CertFile *utils.EnvString `json:"certFile,omitempty" mapstructure:"certFile" yaml:"certFile,omitempty"`
	// Alternative to cert_file. Provide the certificate contents as a base64-encoded string instead of a filepath.
	CertPem *utils.EnvString `json:"certPem,omitempty" mapstructure:"certPem" yaml:"certPem,omitempty"`
	// Path to the TLS key to use for TLS required connections.
	KeyFile *utils.EnvString `json:"keyFile,omitempty" mapstructure:"keyFile" yaml:"keyFile,omitempty"`
	// Alternative to key_file. Provide the key contents as a base64-encoded string instead of a filepath.
	KeyPem *utils.EnvString `json:"keyPem,omitempty" mapstructure:"keyPem" yaml:"keyPem,omitempty"`
	// Path to the CA cert. For a client this verifies the server certificate. For a server this verifies client certificates.
	// If empty uses system root CA.
	CAFile *utils.EnvString `json:"caFile,omitempty" mapstructure:"caFile" yaml:"caFile,omitempty"`
	// Alternative to ca_file. Provide the CA cert contents as a base64-encoded string instead of a filepath.
	CAPem *utils.EnvString `json:"caPem,omitempty" mapstructure:"caPem" yaml:"caPem,omitempty"`
	// Additionally you can configure TLS to be enabled but skip verifying the server's certificate chain.
	InsecureSkipVerify *utils.EnvBool `json:"insecureSkipVerify,omitempty" mapstructure:"insecureSkipVerify" yaml:"insecureSkipVerify,omitempty"`
	// Whether to load the system certificate authorities pool alongside the certificate authority.
	IncludeSystemCACertsPool *utils.EnvBool `json:"includeSystemCACertsPool,omitempty" mapstructure:"includeSystemCACertsPool" yaml:"includeSystemCACertsPool,omitempty"`
	// Minimum acceptable TLS version.
	MinVersion string `json:"minVersion,omitempty" mapstructure:"minVersion" yaml:"minVersion,omitempty"`
	// Maximum acceptable TLS version.
	MaxVersion string `json:"maxVersion,omitempty" mapstructure:"maxVersion" yaml:"maxVersion,omitempty"`
	// Explicit cipher suites can be set. If left blank, a safe default list is used.
	// See https://go.dev/src/crypto/tls/cipher_suites.go for a list of supported cipher suites.
	CipherSuites []string `json:"cipherSuites,omitempty" mapstructure:"cipherSuites" yaml:"cipherSuites,omitempty"`
	// ServerName requested by client for virtual hosting.
	// This sets the ServerName in the TLSConfig. Please refer to
	// https://godoc.org/crypto/tls#Config for more information. (optional)
	ServerName *utils.EnvString `json:"serverName,omitempty" mapstructure:"serverName" yaml:"serverName,omitempty"`
}

// Validate if the current instance is valid
func (tc TLSConfig) Validate() error {
	minTLS, err := tc.GetMinVersion()
	if err != nil {
		return fmt.Errorf("TLSConfig.minVersion: %w", err)
	}
	maxTLS, err := tc.GetMaxVersion()
	if err != nil {
		return fmt.Errorf("TLSConfig.maxVersion: %w", err)
	}

	if maxTLS < minTLS && maxTLS != defaultMaxTLSVersion {
		return errors.New("invalid TLS configuration: minVersion cannot be greater than max_version")
	}

	if tc.CAFile != nil && tc.CAPem != nil {
		caFile, err := tc.CAFile.GetOrDefault("")
		if err != nil {
			return fmt.Errorf("TLSConfig.caFile: %w", err)
		}
		caPem, err := tc.CAFile.GetOrDefault("")
		if err != nil {
			return fmt.Errorf("TLSConfig.caPem: %w", err)
		}

		if caFile != "" && caPem != "" {
			return errors.New("invalid TLS configuration: provide either a CA file or the PEM-encoded string, but not both")
		}
	}

	if tc.CertFile != nil && tc.CertPem != nil {
		certFile, err := tc.CertFile.GetOrDefault("")
		if err != nil {
			return fmt.Errorf("TLSConfig.certFile: %w", err)
		}
		certPem, err := tc.CertPem.GetOrDefault("")
		if err != nil {
			return fmt.Errorf("TLSConfig.caPem: %w", err)
		}

		if certFile != "" && certPem != "" {
			return errors.New("for auth via TLS, provide either a certificate or the PEM-encoded string, but not both")
		}
	}

	if tc.KeyFile != nil && tc.KeyPem != nil {
		keyFile, err := tc.KeyFile.GetOrDefault("")
		if err != nil {
			return fmt.Errorf("TLSConfig.keyFile: %w", err)
		}
		keyPem, err := tc.KeyPem.GetOrDefault("")
		if err != nil {
			return fmt.Errorf("TLSConfig.keyPem: %w", err)
		}

		if keyFile != "" && keyPem != "" {
			return errors.New("for auth via TLS, provide either a certificate or the PEM-encoded string, but not both")
		}
	}

	if tc.IncludeSystemCACertsPool != nil {
		_, err := tc.IncludeSystemCACertsPool.GetOrDefault(false)
		if err != nil {
			return err
		}
	}

	if tc.ServerName != nil {
		_, err := tc.ServerName.GetOrDefault("")
		if err != nil {
			return err
		}
	}

	return nil
}

// GetMinVersion parses the minx TLS version from string.
func (tc TLSConfig) GetMinVersion() (uint16, error) {
	return tc.convertTLSVersion(tc.MinVersion, defaultMinTLSVersion)
}

// GetMaxVersion parses the max TLS version from string.
func (tc TLSConfig) GetMaxVersion() (uint16, error) {
	return tc.convertTLSVersion(tc.MinVersion, defaultMaxTLSVersion)
}

func (tc TLSConfig) convertTLSVersion(v string, defaultVersion uint16) (uint16, error) {
	// Use a default that is explicitly defined
	if v == "" {
		return defaultVersion, nil
	}
	val, ok := tlsVersions[v]
	if !ok {
		return 0, fmt.Errorf("unsupported TLS version: %q", v)
	}

	return val, nil
}

var tlsVersions = map[string]uint16{
	"1.0": tls.VersionTLS10,
	"1.1": tls.VersionTLS11,
	"1.2": tls.VersionTLS12,
	"1.3": tls.VersionTLS13,
}
