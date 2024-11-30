package security

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/hasura/ndc-http/ndc-http-schema/schema"
)

var systemCertPool = x509.SystemCertPool

// NewHTTPClientTLS creates a new HTTP Client with TLS configuration.
func NewHTTPClientTLS(baseClient *http.Client, tlsConfig *schema.TLSConfig) (*http.Client, error) {
	baseTransport, ok := baseClient.Transport.(*http.Transport)
	if !ok {
		baseTransport, _ = http.DefaultTransport.(*http.Transport)
	}

	tlsCfg, err := loadTLSConfig(tlsConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to load TLS config: %w", err)
	}

	transport := baseTransport.Clone()
	transport.TLSClientConfig = tlsCfg

	return &http.Client{
		Transport:     transport,
		CheckRedirect: baseClient.CheckRedirect,
		Jar:           baseClient.Jar,
		Timeout:       baseClient.Timeout,
	}, nil
}

// loadTLSConfig loads TLS certificates and returns a tls.Config.
// This will set the RootCAs and Certificates of a tls.Config.
func loadTLSConfig(tlsConfig *schema.TLSConfig) (*tls.Config, error) {
	certPool, err := loadCACertPool(tlsConfig)
	if err != nil {
		return nil, err
	}

	minTLS, err := tlsConfig.GetMinVersion()
	if err != nil {
		return nil, fmt.Errorf("invalid TLS min_version: %w", err)
	}
	maxTLS, err := tlsConfig.GetMaxVersion()
	if err != nil {
		return nil, fmt.Errorf("invalid TLS max_version: %w", err)
	}
	cipherSuites, err := convertCipherSuites(tlsConfig.CipherSuites)
	if err != nil {
		return nil, err
	}

	var serverName string
	if tlsConfig.ServerName != nil {
		serverName, err = tlsConfig.ServerName.GetOrDefault("")
		if err != nil {
			return nil, fmt.Errorf("failed to get TLS server name: %w", err)
		}
	}

	cert, err := loadCertificate(tlsConfig)
	if err != nil {
		return nil, err
	}

	var insecureSkipVerify bool
	if tlsConfig.InsecureSkipVerify != nil {
		insecureSkipVerify, err = tlsConfig.InsecureSkipVerify.GetOrDefault(false)
		if err != nil {
			return nil, fmt.Errorf("failed to parse insecureSkipVerify: %w", err)
		}
	}

	if cert == nil && !insecureSkipVerify {
		return nil, nil
	}

	result := &tls.Config{
		RootCAs:            certPool,
		Certificates:       []tls.Certificate{*cert},
		MinVersion:         minTLS,
		MaxVersion:         maxTLS,
		CipherSuites:       cipherSuites,
		ServerName:         serverName,
		InsecureSkipVerify: insecureSkipVerify,
	}

	return result, nil
}

func loadCACertPool(tlsConfig *schema.TLSConfig) (*x509.CertPool, error) {
	// There is no need to load the System Certs for RootCAs because
	// if the value is nil, it will default to checking against th System Certs.
	var err error
	var certPool *x509.CertPool
	var includeSystemCACertsPool bool

	if tlsConfig.IncludeSystemCACertsPool != nil {
		includeSystemCACertsPool, err = tlsConfig.IncludeSystemCACertsPool.GetOrDefault(false)
		if err != nil {
			return nil, fmt.Errorf("invalid includeSystemCACertsPool config: %w", err)
		}
	}

	if tlsConfig.CAPem != nil {
		caPem, err := tlsConfig.CAPem.GetOrDefault("")
		if err != nil {
			return nil, fmt.Errorf("failed to load CA CertPool PEM: %w", err)
		}

		if caPem != "" {
			return loadCertPem([]byte(caPem), includeSystemCACertsPool)
		}
	}

	if tlsConfig.CAFile != nil {
		caFile, err := tlsConfig.CAFile.GetOrDefault("")
		if err != nil {
			return nil, fmt.Errorf("failed to load CA CertPool File: %w", err)
		}

		if caFile != "" {
			return loadCertFile(caFile, includeSystemCACertsPool)
		}
	}

	return certPool, nil
}

func loadCertFile(certPath string, includeSystemCACertsPool bool) (*x509.CertPool, error) {
	certPem, err := os.ReadFile(filepath.Clean(certPath))
	if err != nil {
		return nil, fmt.Errorf("failed to load cert %s: %w", certPath, err)
	}

	return loadCertPem(certPem, includeSystemCACertsPool)
}

func loadCertPem(certPem []byte, includeSystemCACertsPool bool) (*x509.CertPool, error) {
	certPool := x509.NewCertPool()
	if includeSystemCACertsPool {
		scp, err := systemCertPool()
		if err != nil {
			return nil, err
		}
		if scp != nil {
			certPool = scp
		}
	}
	if !certPool.AppendCertsFromPEM(certPem) {
		return nil, errors.New("failed to parse cert")
	}

	return certPool, nil
}

func loadCertificate(tlsConfig *schema.TLSConfig) (*tls.Certificate, error) {
	var certData, keyData []byte
	var certPem, keyPem string
	var err error

	if tlsConfig.CertPem != nil {
		certPem, err = tlsConfig.CertPem.GetOrDefault("")
		if err != nil {
			return nil, fmt.Errorf("failed to load certificate PEM: %w", err)
		}
	}

	if certPem != "" {
		certData = []byte(certPem)
	} else if tlsConfig.CertFile != nil {
		certFile, err := tlsConfig.CertFile.GetOrDefault("")
		if err != nil {
			return nil, fmt.Errorf("failed to load certificate file: %w", err)
		}

		if certFile != "" {
			certData, err = os.ReadFile(certFile)
			if err != nil {
				return nil, fmt.Errorf("failed to read certificate file: %w", err)
			}
		}
	}

	if tlsConfig.KeyPem != nil {
		keyPem, err = tlsConfig.KeyPem.GetOrDefault("")
		if err != nil {
			return nil, fmt.Errorf("failed to load key PEM: %w", err)
		}
	}

	if keyPem != "" {
		keyData = []byte(keyPem)
	} else if tlsConfig.KeyFile != nil {
		keyFile, err := tlsConfig.KeyFile.GetOrDefault("")
		if err != nil {
			return nil, fmt.Errorf("failed to load key file: %w", err)
		}

		if keyFile != "" {
			keyData, err = os.ReadFile(keyFile)
			if err != nil {
				return nil, fmt.Errorf("failed to read key file: %w", err)
			}
		}
	}

	if len(keyData) == 0 && len(certData) == 0 {
		return nil, nil
	}

	if len(keyData) == 0 || len(certData) == 0 {
		return nil, errors.New("provide both certificate and key, or neither")
	}

	certificate, err := tls.X509KeyPair(certData, keyData)
	if err != nil {
		return nil, fmt.Errorf("failed to load TLS cert and key PEMs: %w", err)
	}

	return &certificate, err
}

func convertCipherSuites(cipherSuites []string) ([]uint16, error) {
	var result []uint16
	var errs []error
	for _, suite := range cipherSuites {
		found := false
		for _, supported := range tls.CipherSuites() {
			if suite == supported.Name {
				result = append(result, supported.ID)
				found = true

				break
			}
		}
		if !found {
			errs = append(errs, fmt.Errorf("invalid TLS cipher suite: %q", suite))
		}
	}

	return result, errors.Join(errs...)
}
