package internal

import (
	rest "github.com/hasura/ndc-rest/ndc-rest-schema/schema"
	"github.com/hasura/ndc-sdk-go/schema"
)

// MetadataCollection stores list of REST metadata with helper methods
type MetadataCollection []rest.NDCRestSchema

// GetFunction gets the NDC function by name
func (rms MetadataCollection) GetFunction(name string) (*rest.OperationInfo, *rest.NDCRestSettings, error) {
	for _, rm := range rms {
		fn := rm.GetFunction(name)
		if fn != nil {
			return fn, rm.Settings, nil
		}
	}
	return nil, nil, schema.UnprocessableContentError("unsupported query: "+name, nil)
}

// GetProcedure gets the NDC procedure by name
func (rms MetadataCollection) GetProcedure(name string) (*rest.OperationInfo, *rest.NDCRestSettings, error) {
	for _, rm := range rms {
		fn := rm.GetProcedure(name)
		if fn != nil {
			return fn, rm.Settings, nil
		}
	}
	return nil, nil, schema.UnprocessableContentError("unsupported query: "+name, nil)
}
