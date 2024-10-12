package openapi

import (
	"errors"

	"github.com/hasura/ndc-rest/ndc-rest-schema/openapi/internal"
	rest "github.com/hasura/ndc-rest/ndc-rest-schema/schema"
	"github.com/pb33f/libopenapi"
)

// OpenAPIv2ToNDCSchema converts OpenAPI v2 JSON bytes to NDC REST schema
func OpenAPIv2ToNDCSchema(input []byte, options ConvertOptions) (*rest.NDCRestSchema, []error) {
	document, err := libopenapi.NewDocument(input)
	if err != nil {
		return nil, []error{err}
	}

	docModel, errs := document.BuildV2Model()
	// The errors won’t prevent the model from building
	if docModel == nil && len(errs) > 0 {
		return nil, errs
	}

	if docModel.Model.Paths == nil || docModel.Model.Paths.PathItems == nil || docModel.Model.Paths.PathItems.IsZero() {
		return nil, append(errs, errors.New("there is no API to be converted"))
	}

	converter := internal.NewOAS2Builder(rest.NewNDCRestSchema(), internal.ConvertOptions(options))
	if err := converter.BuildDocumentModel(docModel); err != nil {
		return nil, append(errs, err)
	}
	return converter.Schema(), nil
}
