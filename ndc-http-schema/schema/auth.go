package schema

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/hasura/ndc-sdk-go/utils"
	"github.com/invopop/jsonschema"
	orderedmap "github.com/wk8/go-ordered-map/v2"
)

const (
	HTTPAuthSchemeBearer = "bearer"
	AuthorizationHeader  = "Authorization"
)

var (
	errSecuritySchemerRequired = errors.New("SecuritySchemer is required")
)

// SecuritySchemeType represents the authentication scheme enum
type SecuritySchemeType string

const (
	APIKeyScheme        SecuritySchemeType = "apiKey"
	BasicAuthScheme     SecuritySchemeType = "basic"
	CookieAuthScheme    SecuritySchemeType = "cookie"
	HTTPAuthScheme      SecuritySchemeType = "http"
	OAuth2Scheme        SecuritySchemeType = "oauth2"
	OpenIDConnectScheme SecuritySchemeType = "openIdConnect"
	MutualTLSScheme     SecuritySchemeType = "mutualTLS"
)

var securityScheme_enums = []SecuritySchemeType{
	APIKeyScheme,
	HTTPAuthScheme,
	BasicAuthScheme,
	CookieAuthScheme,
	OAuth2Scheme,
	OpenIDConnectScheme,
	MutualTLSScheme,
}

// JSONSchema is used to generate a custom jsonschema
func (j SecuritySchemeType) JSONSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type: "string",
		Enum: toAnySlice(securityScheme_enums),
	}
}

// UnmarshalJSON implements json.Unmarshaler.
func (j *SecuritySchemeType) UnmarshalJSON(b []byte) error {
	var rawResult string
	if err := json.Unmarshal(b, &rawResult); err != nil {
		return err
	}

	result, err := ParseSecuritySchemeType(rawResult)
	if err != nil {
		return err
	}

	*j = result
	return nil
}

// ParseSecuritySchemeType parses SecurityScheme from string
func ParseSecuritySchemeType(value string) (SecuritySchemeType, error) {
	result := SecuritySchemeType(value)
	if !slices.Contains(securityScheme_enums, result) {
		return result, fmt.Errorf("invalid SecuritySchemeType. Expected %+v, got <%s>", securityScheme_enums, value)
	}
	return result, nil
}

// ApiKeyLocation represents the location enum for apiKey auth
type APIKeyLocation string

const (
	APIKeyInHeader APIKeyLocation = "header"
	APIKeyInQuery  APIKeyLocation = "query"
	APIKeyInCookie APIKeyLocation = "cookie"
)

var apiKeyLocation_enums = []APIKeyLocation{APIKeyInHeader, APIKeyInQuery, APIKeyInCookie}

// JSONSchema is used to generate a custom jsonschema
func (j APIKeyLocation) JSONSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type: "string",
		Enum: toAnySlice(apiKeyLocation_enums),
	}
}

// UnmarshalJSON implements json.Unmarshaler.
func (j *APIKeyLocation) UnmarshalJSON(b []byte) error {
	var rawResult string
	if err := json.Unmarshal(b, &rawResult); err != nil {
		return err
	}

	result, err := ParseAPIKeyLocation(rawResult)
	if err != nil {
		return err
	}

	*j = result
	return nil
}

// ParseAPIKeyLocation parses APIKeyLocation from string
func ParseAPIKeyLocation(value string) (APIKeyLocation, error) {
	result := APIKeyLocation(value)
	if !slices.Contains(apiKeyLocation_enums, result) {
		return result, fmt.Errorf("invalid APIKeyLocation. Expected %+v, got <%s>", apiKeyLocation_enums, value)
	}
	return result, nil
}

// SecuritySchemer abstracts an interface of SecurityScheme.
type SecuritySchemer interface {
	GetType() SecuritySchemeType
	Validate() error
}

// SecurityScheme contains authentication configurations.
// The schema follows [OpenAPI 3] specification
//
// [OpenAPI 3]: https://swagger.io/docs/specification/authentication
type SecurityScheme struct {
	SecuritySchemer
}

// JSONSchema is used to generate a custom jsonschema
func (j SecurityScheme) JSONSchema() *jsonschema.Schema {
	envStringRef := &jsonschema.Schema{
		Ref: "#/$defs/EnvString",
	}
	apiKeySchema := orderedmap.New[string, *jsonschema.Schema]()
	apiKeySchema.Set("type", &jsonschema.Schema{
		Type: "string",
		Enum: []any{APIKeyScheme},
	})
	apiKeySchema.Set("value", envStringRef)
	apiKeySchema.Set("in", (APIKeyLocation("")).JSONSchema())
	apiKeySchema.Set("name", &jsonschema.Schema{
		Type: "string",
	})

	httpAuthSchema := orderedmap.New[string, *jsonschema.Schema]()
	httpAuthSchema.Set("type", &jsonschema.Schema{
		Type: "string",
		Enum: []any{HTTPAuthScheme},
	})
	httpAuthSchema.Set("value", envStringRef)
	httpAuthSchema.Set("header", &jsonschema.Schema{
		Type: "string",
	})
	httpAuthSchema.Set("scheme", &jsonschema.Schema{
		Type: "string",
	})

	basicAuthSchema := orderedmap.New[string, *jsonschema.Schema]()
	basicAuthSchema.Set("type", &jsonschema.Schema{
		Type: "string",
		Enum: []any{BasicAuthScheme},
	})
	basicAuthSchema.Set("username", envStringRef)
	basicAuthSchema.Set("password", envStringRef)
	httpAuthSchema.Set("header", &jsonschema.Schema{
		Description: "Request contains a header field in the form of Authorization: Basic <credentials>",
		OneOf: []*jsonschema.Schema{
			{Type: "string"},
			{Type: "null"},
		},
	})

	oauth2Schema := orderedmap.New[string, *jsonschema.Schema]()
	oauth2Schema.Set("type", &jsonschema.Schema{
		Type: "string",
		Enum: []any{OAuth2Scheme},
	})
	oauth2Schema.Set("flows", &jsonschema.Schema{
		Type:                 "object",
		AdditionalProperties: &jsonschema.Schema{},
	})

	oidcSchema := orderedmap.New[string, *jsonschema.Schema]()
	oidcSchema.Set("type", &jsonschema.Schema{
		Type: "string",
		Enum: []any{OpenIDConnectScheme},
	})
	oidcSchema.Set("openIdConnectUrl", &jsonschema.Schema{
		Type: "string",
	})

	cookieSchema := orderedmap.New[string, *jsonschema.Schema]()
	cookieSchema.Set("type", &jsonschema.Schema{
		Type: "string",
		Enum: []any{CookieAuthScheme},
	})

	mutualTLSSchema := orderedmap.New[string, *jsonschema.Schema]()
	mutualTLSSchema.Set("type", &jsonschema.Schema{
		Type: "string",
		Enum: []any{MutualTLSScheme},
	})

	return &jsonschema.Schema{
		OneOf: []*jsonschema.Schema{
			{
				Type:       "object",
				Required:   []string{"type", "value", "in", "name"},
				Properties: apiKeySchema,
			},
			{
				Type:       "object",
				Properties: basicAuthSchema,
				Required:   []string{"type", "username", "password"},
			},
			{
				Type:       "object",
				Properties: httpAuthSchema,
				Required:   []string{"type", "value", "header", "scheme"},
			},
			{
				Type:       "object",
				Properties: oauth2Schema,
				Required:   []string{"type", "flows"},
			},
			{
				Type:       "object",
				Properties: oidcSchema,
				Required:   []string{"type", "openIdConnectUrl"},
			},
			{
				Type:       "object",
				Properties: cookieSchema,
				Required:   []string{"type"},
			},
			{
				Type:       "object",
				Properties: mutualTLSSchema,
				Required:   []string{"type"},
			},
		},
	}
}

type rawSecurityScheme struct {
	Type SecuritySchemeType `json:"type" yaml:"type"`
}

// UnmarshalJSON implements json.Unmarshaler.
func (j *SecurityScheme) UnmarshalJSON(b []byte) error {
	var rawScheme rawSecurityScheme
	if err := json.Unmarshal(b, &rawScheme); err != nil {
		return err
	}

	switch rawScheme.Type {
	case APIKeyScheme:
		var config APIKeyAuthConfig
		if err := json.Unmarshal(b, &config); err != nil {
			return err
		}
		_ = config.Validate()
		j.SecuritySchemer = &config
	case BasicAuthScheme:
		var config BasicAuthConfig
		if err := json.Unmarshal(b, &config); err != nil {
			return err
		}
		_ = config.Validate()
		j.SecuritySchemer = &config
	case HTTPAuthScheme:
		var config HTTPAuthConfig
		if err := json.Unmarshal(b, &config); err != nil {
			return err
		}
		_ = config.Validate()
		j.SecuritySchemer = &config
	case OAuth2Scheme:
		var config OAuth2Config
		if err := json.Unmarshal(b, &config); err != nil {
			return err
		}
		_ = config.Validate()
		j.SecuritySchemer = &config
	case OpenIDConnectScheme:
		var config OpenIDConnectConfig
		if err := json.Unmarshal(b, &config); err != nil {
			return err
		}
		_ = config.Validate()
		j.SecuritySchemer = &config
	case CookieAuthScheme:
		j.SecuritySchemer = &CookieAuthConfig{
			Type: rawScheme.Type,
		}
	case MutualTLSScheme:
		j.SecuritySchemer = &MutualTLSAuthConfig{
			Type: rawScheme.Type,
		}
	}

	return nil
}

// MarshalJSON implements json.Marshaler.
func (j SecurityScheme) MarshalJSON() ([]byte, error) {
	return json.Marshal(j.SecuritySchemer)
}

// Validate if the current instance is valid
func (ss *SecurityScheme) Validate() error {
	if ss.SecuritySchemer == nil {
		return errSecuritySchemerRequired
	}

	return ss.SecuritySchemer.Validate()
}

// APIKeyAuthConfig contains configurations for [apiKey authentication]
//
// [apiKey authentication]: https://swagger.io/docs/specification/authentication/api-keys/
type APIKeyAuthConfig struct {
	Type  SecuritySchemeType `json:"type"  mapstructure:"type"  yaml:"type"`
	In    APIKeyLocation     `json:"in"    mapstructure:"in"    yaml:"in"`
	Name  string             `json:"name"  mapstructure:"name"  yaml:"name"`
	Value utils.EnvString    `json:"value" mapstructure:"value" yaml:"value"`

	// cached values
	value *string
}

var _ SecuritySchemer = &APIKeyAuthConfig{}

// NewAPIKeyAuthConfig creates a new APIKeyAuthConfig instance.
func NewAPIKeyAuthConfig(name string, in APIKeyLocation, value utils.EnvString) *APIKeyAuthConfig {
	return &APIKeyAuthConfig{
		Type:  APIKeyScheme,
		Name:  name,
		In:    in,
		Value: value,
	}
}

// UnmarshalJSON implements json.Unmarshaler.
func (j *APIKeyAuthConfig) UnmarshalJSON(b []byte) error {
	type Plain APIKeyAuthConfig

	var raw Plain
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}

	result := APIKeyAuthConfig(raw)
	_ = result.Validate()
	*j = result

	return nil
}

// Validate if the current instance is valid
func (ss *APIKeyAuthConfig) Validate() error {
	if ss.Name == "" {
		return errors.New("name is required for apiKey security")
	}
	if _, err := ParseAPIKeyLocation(string(ss.In)); err != nil {
		return err
	}

	value, err := ss.Value.Get()
	if err != nil {
		return fmt.Errorf("APIKeyAuthConfig.Value: %w", err)
	}
	if value != "" {
		ss.value = &value
	}

	return nil
}

// GetValue get the authentication credential value
func (ss APIKeyAuthConfig) GetType() SecuritySchemeType {
	return ss.Type
}

// GetValue get the authentication credential value
func (ss APIKeyAuthConfig) GetValue() string {
	if ss.value != nil {
		return *ss.value
	}

	value, _ := ss.Value.Get()

	return value
}

// HTTPAuthConfig contains configurations for http authentication
// If the scheme is [bearer], the authenticator follows OpenAPI 3 specification.
//
// [bearer]: https://swagger.io/docs/specification/authentication/bearer-authentication
type HTTPAuthConfig struct {
	Type   SecuritySchemeType `json:"type"   mapstructure:"type"   yaml:"type"`
	Header string             `json:"header" mapstructure:"header" yaml:"header"`
	Scheme string             `json:"scheme" mapstructure:"scheme" yaml:"scheme"`
	Value  utils.EnvString    `json:"value"  mapstructure:"value"  yaml:"value"`

	// cached values
	value *string
}

var _ SecuritySchemer = &HTTPAuthConfig{}

// NewHTTPAuthConfig creates a new HTTPAuthConfig instance.
func NewHTTPAuthConfig(scheme string, header string, value utils.EnvString) *HTTPAuthConfig {
	return &HTTPAuthConfig{
		Type:   HTTPAuthScheme,
		Header: header,
		Scheme: scheme,
		Value:  value,
	}
}

// Validate if the current instance is valid
func (ss *HTTPAuthConfig) Validate() error {
	if ss.Scheme == "" {
		return errors.New("schema is required for http security")
	}

	value, err := ss.Value.Get()
	if err != nil {
		return fmt.Errorf("APIKeyAuthConfig.Value: %w", err)
	}
	if value != "" {
		ss.value = &value
	}

	return nil
}

// GetValue get the authentication credential value
func (ss HTTPAuthConfig) GetType() SecuritySchemeType {
	return ss.Type
}

// GetValue get the authentication credential value
func (ss HTTPAuthConfig) GetValue() string {
	if ss.value != nil {
		return *ss.value
	}

	value, _ := ss.Value.Get()

	return value
}

// BasicAuthConfig contains configurations for the [basic] authentication.
//
// [basic]: https://swagger.io/docs/specification/authentication/basic-authentication
type BasicAuthConfig struct {
	Type     SecuritySchemeType `json:"type"     mapstructure:"type"     yaml:"type"`
	Header   string             `json:"header"   mapstructure:"header"   yaml:"header"`
	Username utils.EnvString    `json:"username" mapstructure:"username" yaml:"username"`
	Password utils.EnvString    `json:"password" mapstructure:"password" yaml:"password"`

	// cached values
	username *string
	password *string
}

// NewBasicAuthConfig creates a new BasicAuthConfig instance.
func NewBasicAuthConfig(username, password utils.EnvString) *BasicAuthConfig {
	return &BasicAuthConfig{
		Type:     BasicAuthScheme,
		Username: username,
		Password: password,
	}
}

// Validate if the current instance is valid
func (ss *BasicAuthConfig) Validate() error {
	user, err := ss.Username.Get()
	if err != nil {
		return fmt.Errorf("BasicAuthConfig.User: %w", err)
	}

	// user and password can be empty.
	ss.username = &user

	password, err := ss.Password.Get()
	if err != nil {
		return fmt.Errorf("BasicAuthConfig.Password: %w", err)
	}
	ss.password = &password

	return nil
}

// GetValue get the authentication credential value
func (ss BasicAuthConfig) GetType() SecuritySchemeType {
	return ss.Type
}

// GetUsername get the username value
func (ss BasicAuthConfig) GetUsername() string {
	if ss.username != nil {
		return *ss.username
	}

	value, _ := ss.Username.Get()

	return value
}

// GetPassword get the password value
func (ss BasicAuthConfig) GetPassword() string {
	if ss.password != nil {
		return *ss.password
	}

	value, _ := ss.Password.Get()

	return value
}

// OAuthFlowType represents the OAuth flow type enum
type OAuthFlowType string

const (
	AuthorizationCodeFlow OAuthFlowType = "authorizationCode"
	ImplicitFlow          OAuthFlowType = "implicit"
	PasswordFlow          OAuthFlowType = "password"
	ClientCredentialsFlow OAuthFlowType = "clientCredentials"
)

var oauthFlow_enums = []OAuthFlowType{
	AuthorizationCodeFlow,
	ImplicitFlow,
	PasswordFlow,
	ClientCredentialsFlow,
}

// UnmarshalJSON implements json.Unmarshaler.
func (j *OAuthFlowType) UnmarshalJSON(b []byte) error {
	var rawResult string
	if err := json.Unmarshal(b, &rawResult); err != nil {
		return err
	}

	result, err := ParseOAuthFlowType(rawResult)
	if err != nil {
		return err
	}

	*j = result
	return nil
}

// ParseOAuthFlowType parses OAuthFlowType from string
func ParseOAuthFlowType(value string) (OAuthFlowType, error) {
	result := OAuthFlowType(value)
	if !slices.Contains(oauthFlow_enums, result) {
		return result, fmt.Errorf("invalid OAuthFlowType. Expected %+v, got <%s>", oauthFlow_enums, value)
	}
	return result, nil
}

// OAuthFlow contains flow configurations for [OAuth 2.0] API specification
//
// [OAuth 2.0]: https://swagger.io/docs/specification/authentication/oauth2
type OAuthFlow struct {
	AuthorizationURL string            `json:"authorizationUrl,omitempty" mapstructure:"authorizationUrl" yaml:"authorizationUrl,omitempty"`
	TokenURL         string            `json:"tokenUrl,omitempty"         mapstructure:"tokenUrl"         yaml:"tokenUrl,omitempty"`
	RefreshURL       string            `json:"refreshUrl,omitempty"       mapstructure:"refreshUrl"       yaml:"refreshUrl,omitempty"`
	Scopes           map[string]string `json:"scopes,omitempty"           mapstructure:"scopes"           yaml:"scopes,omitempty"`
}

// Validate if the current instance is valid
func (ss OAuthFlow) Validate(flowType OAuthFlowType) error {
	if ss.AuthorizationURL == "" {
		if slices.Contains([]OAuthFlowType{ImplicitFlow, AuthorizationCodeFlow}, flowType) {
			return fmt.Errorf("authorizationUrl is required for oauth2 %s security", flowType)
		}
	} else if _, err := parseRelativeOrHttpURL(ss.AuthorizationURL); err != nil {
		return fmt.Errorf("authorizationUrl: %w", err)
	}

	if ss.TokenURL == "" {
		if slices.Contains([]OAuthFlowType{PasswordFlow, ClientCredentialsFlow, AuthorizationCodeFlow}, flowType) {
			return fmt.Errorf("tokenUrl is required for oauth2 %s security", flowType)
		}
	} else if _, err := parseRelativeOrHttpURL(ss.TokenURL); err != nil {
		return fmt.Errorf("tokenUrl: %w", err)
	}
	if ss.RefreshURL != "" {
		if _, err := parseRelativeOrHttpURL(ss.RefreshURL); err != nil {
			return fmt.Errorf("refreshUrl: %w", err)
		}
	}
	return nil
}

// OAuth2Config contains configurations for [OAuth 2.0] API specification
//
// [OAuth 2.0]: https://swagger.io/docs/specification/authentication/oauth2
type OAuth2Config struct {
	Type  SecuritySchemeType          `json:"type"  mapstructure:"type"  yaml:"type"`
	Flows map[OAuthFlowType]OAuthFlow `json:"flows" mapstructure:"flows" yaml:"flows"`
}

var _ SecuritySchemer = &OAuth2Config{}

// NewOAuth2Config creates a new OAuth2Config instance.
func NewOAuth2Config(flows map[OAuthFlowType]OAuthFlow) *OAuth2Config {
	return &OAuth2Config{
		Type:  OAuth2Scheme,
		Flows: flows,
	}
}

// GetValue get the authentication credential value
func (ss OAuth2Config) GetType() SecuritySchemeType {
	return ss.Type
}

// Validate if the current instance is valid
func (ss OAuth2Config) Validate() error {
	if len(ss.Flows) == 0 {
		return errors.New("require at least 1 flow for oauth2 security")
	}

	for key, flow := range ss.Flows {
		if err := flow.Validate(key); err != nil {
			return fmt.Errorf("%s: %w", key, err)
		}
	}

	return nil
}

// OpenIDConnectConfig contains configurations for [OpenID Connect] API specification
//
// [OpenID Connect]: https://swagger.io/docs/specification/authentication/openid-connect-discovery
type OpenIDConnectConfig struct {
	Type             SecuritySchemeType `json:"type"             mapstructure:"type"             yaml:"type"`
	OpenIDConnectURL string             `json:"openIdConnectUrl" mapstructure:"openIdConnectUrl" yaml:"openIdConnectUrl"`
}

var _ SecuritySchemer = &OpenIDConnectConfig{}

// NewOpenIDConnectConfig creates a new OpenIDConnectConfig instance.
func NewOpenIDConnectConfig(oidcURL string) *OpenIDConnectConfig {
	return &OpenIDConnectConfig{
		Type:             OpenIDConnectScheme,
		OpenIDConnectURL: oidcURL,
	}
}

// GetValue get the authentication credential value
func (ss OpenIDConnectConfig) GetType() SecuritySchemeType {
	return ss.Type
}

// Validate if the current instance is valid
func (ss OpenIDConnectConfig) Validate() error {
	if ss.OpenIDConnectURL == "" {
		return errors.New("openIdConnectUrl is required for oidc security")
	}

	if _, err := parseRelativeOrHttpURL(ss.OpenIDConnectURL); err != nil {
		return fmt.Errorf("openIdConnectUrl: %w", err)
	}
	return nil
}

// CookieAuthConfig represents a cookie authentication configuration.
type CookieAuthConfig struct {
	Type SecuritySchemeType `json:"type" mapstructure:"type" yaml:"type"`
}

var _ SecuritySchemer = &CookieAuthConfig{}

// NewCookieAuthConfig creates a new CookieAuthConfig instance.
func NewCookieAuthConfig() *CookieAuthConfig {
	return &CookieAuthConfig{
		Type: CookieAuthScheme,
	}
}

// GetValue get the authentication credential value
func (ss CookieAuthConfig) GetType() SecuritySchemeType {
	return ss.Type
}

// Validate if the current instance is valid
func (ss CookieAuthConfig) Validate() error {
	return nil
}

// MutualTLSAuthConfig represents a mutualTLS authentication configuration.
type MutualTLSAuthConfig struct {
	Type SecuritySchemeType `json:"type" mapstructure:"type" yaml:"type"`
}

var _ SecuritySchemer = &MutualTLSAuthConfig{}

// NewMutualTLSAuthConfig creates a new MutualTLSAuthConfig instance.
func NewMutualTLSAuthConfig() *MutualTLSAuthConfig {
	return &MutualTLSAuthConfig{
		Type: MutualTLSScheme,
	}
}

// GetValue get the authentication credential value
func (ss MutualTLSAuthConfig) GetType() SecuritySchemeType {
	return ss.Type
}

// Validate if the current instance is valid
func (ss MutualTLSAuthConfig) Validate() error {
	return nil
}

// AuthSecurity wraps the raw security requirement with helpers
type AuthSecurity map[string][]string

// NewAuthSecurity creates an AuthSecurity instance from name and scope
func NewAuthSecurity(name string, scopes []string) AuthSecurity {
	return AuthSecurity{
		name: scopes,
	}
}

// Name returns the name of security requirement
func (as AuthSecurity) Name() string {
	if len(as) > 0 {
		for k := range as {
			return k
		}
	}
	return ""
}

// Scopes returns scopes of security requirement
func (as AuthSecurity) Scopes() []string {
	if len(as) > 0 {
		for _, scopes := range as {
			return scopes
		}
	}
	return []string{}
}

// IsOptional checks if the security is optional
func (as AuthSecurity) IsOptional() bool {
	return len(as) == 0
}

// AuthSecurities wraps list of security requirements with helpers
type AuthSecurities []AuthSecurity

// IsEmpty checks if there is no security
func (ass AuthSecurities) IsEmpty() bool {
	return len(ass) == 0
}

// IsOptional checks if the security is optional
func (ass AuthSecurities) IsOptional() bool {
	if ass.IsEmpty() {
		return true
	}
	for _, as := range ass {
		if as.IsOptional() {
			return true
		}
	}
	return false
}

// Add adds a security with name and scope
func (ass *AuthSecurities) Add(item AuthSecurity) {
	*ass = append(*ass, item)
}

// Get gets a security by name
func (ass AuthSecurities) Get(name string) AuthSecurity {
	for _, as := range ass {
		if as.Name() == name {
			return as
		}
	}
	return nil
}

// First returns the first security
func (ass AuthSecurities) First() AuthSecurity {
	for _, as := range ass {
		return as
	}
	return nil
}
