package auth

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"

	"github.com/hasura/ndc-http/ndc-http-schema/schema"
)

// BasicCredential represents the basic authentication credential
type BasicCredential struct {
	UserInfo *url.Userinfo
	Header   string

	client *http.Client
}

var _ Credential = &BasicCredential{}

// NewBasicCredential creates a new BasicCredential instance.
func NewBasicCredential(client *http.Client, config *schema.BasicAuthConfig) (*BasicCredential, error) {
	user, err := config.Username.Get()
	if err != nil {
		return nil, fmt.Errorf("BasicAuthConfig.Username: %w", err)
	}

	password, err := config.Password.Get()
	if err != nil {
		return nil, fmt.Errorf("BasicAuthConfig.Password: %w", err)
	}

	result := &BasicCredential{
		client: client,
	}

	if password != "" {
		result.UserInfo = url.UserPassword(user, password)
	} else {
		result.UserInfo = url.User(user)
	}

	return result, nil
}

// GetClient gets the HTTP client that is compatible with the current credential.
func (bc BasicCredential) GetClient() *http.Client {
	return bc.client
}

// Inject the credential into the incoming request
func (bc BasicCredential) Inject(req *http.Request) (bool, error) {
	if bc.UserInfo == nil {
		return false, nil
	}

	return bc.inject(req, *bc.UserInfo)
}

// InjectMock injects the mock credential into the incoming request for explain APIs.
func (bc BasicCredential) InjectMock(req *http.Request) bool {
	if bc.UserInfo == nil {
		return false
	}
	_, _ = bc.inject(req, *url.UserPassword("xxx", "xxx"))

	return true
}

func (bc BasicCredential) inject(req *http.Request, userInfo url.Userinfo) (bool, error) {
	if bc.Header != "" {
		b64Value := base64.StdEncoding.EncodeToString([]byte(userInfo.String()))
		req.Header.Set(bc.Header, "Basic "+b64Value)
	} else {
		req.URL.User = &userInfo
	}

	return true, nil
}
