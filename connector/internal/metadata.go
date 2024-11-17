package internal

import (
	"github.com/hasura/ndc-http/ndc-http-schema/configuration"
	rest "github.com/hasura/ndc-http/ndc-http-schema/schema"
	"github.com/hasura/ndc-sdk-go/schema"
)

// MetadataCollection stores list of HTTP metadata with helper methods
type MetadataCollection []configuration.NDCHttpRuntimeSchema

// GetFunction gets the NDC function by name
func (rms MetadataCollection) GetFunction(name string) (*rest.OperationInfo, configuration.NDCHttpRuntimeSchema, error) {
	for _, rm := range rms {
		fn := rm.GetFunction(name)
		if fn != nil {
			return fn, rm, nil
		}
	}

	return nil, configuration.NDCHttpRuntimeSchema{}, schema.UnprocessableContentError("unsupported query: "+name, nil)
}

// GetProcedure gets the NDC procedure by name
func (rms MetadataCollection) GetProcedure(name string) (*rest.OperationInfo, configuration.NDCHttpRuntimeSchema, error) {
	for _, rm := range rms {
		fn := rm.GetProcedure(name)
		if fn != nil {
			return fn, rm, nil
		}
	}

	return nil, configuration.NDCHttpRuntimeSchema{}, schema.UnprocessableContentError("unsupported mutation: "+name, nil)
}
