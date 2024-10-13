package rest

import (
	rest "github.com/hasura/ndc-rest/ndc-rest-schema/schema"
	"github.com/hasura/ndc-sdk-go/schema"
)

// RESTMetadataCollection stores list of REST metadata with helper methods
type RESTMetadataCollection []RESTMetadata

// GetFunction gets the NDC function by name
func (rms RESTMetadataCollection) GetFunction(name string) (*rest.OperationInfo, *rest.NDCRestSettings, error) {
	for _, rm := range rms {
		fn := rm.GetFunction(name)
		if fn != nil {
			return fn, rm.settings, nil
		}
	}
	return nil, nil, schema.UnprocessableContentError("unsupported query: "+name, nil)
}

// GetProcedure gets the NDC procedure by name
func (rms RESTMetadataCollection) GetProcedure(name string) (*rest.OperationInfo, *rest.NDCRestSettings, error) {
	for _, rm := range rms {
		fn := rm.GetProcedure(name)
		if fn != nil {
			return fn, rm.settings, nil
		}
	}
	return nil, nil, schema.UnprocessableContentError("unsupported query: "+name, nil)
}

// RESTMetadata stores REST schema with handy methods to build requests
type RESTMetadata struct {
	settings   *rest.NDCRestSettings
	functions  map[string]rest.OperationInfo
	procedures map[string]rest.OperationInfo
}

// GetFunction gets the NDC function by name
func (rm RESTMetadata) GetFunction(name string) *rest.OperationInfo {
	fn, ok := rm.functions[name]
	if !ok {
		return nil
	}

	return &fn
}

// GetProcedure gets the NDC procedure by name
func (rm RESTMetadata) GetProcedure(name string) *rest.OperationInfo {
	fn, ok := rm.procedures[name]
	if !ok {
		return nil
	}

	return &fn
}
