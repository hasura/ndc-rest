package openapi

import (
	"errors"

	"github.com/hasura/ndc-http/ndc-http-schema/openapi/internal"
	rest "github.com/hasura/ndc-http/ndc-http-schema/schema"
	"github.com/hasura/ndc-http/ndc-http-schema/utils"
	"github.com/pb33f/libopenapi"
)

type ConvertOptions internal.ConvertOptions

// OpenAPIv3ToNDCSchema converts OpenAPI v3 JSON bytes to NDC HTTP schema
func OpenAPIv3ToNDCSchema(input []byte, options ConvertOptions) (*rest.NDCHttpSchema, []error) {
	input = utils.RemoveYAMLSpecialCharacters(input)
	document, err := libopenapi.NewDocument(input)
	if err != nil {
		return nil, []error{err}
	}

	docModel, errs := document.BuildV3Model()
	// The errors won’t prevent the model from building
	if docModel == nil && len(errs) > 0 {
		return nil, errs
	}

	if docModel.Model.Paths == nil || docModel.Model.Paths.PathItems == nil || docModel.Model.Paths.PathItems.IsZero() {
		return nil, append(errs, errors.New("there is no API to be converted"))
	}

	converter := internal.NewOAS3Builder(rest.NewNDCHttpSchema(), internal.ConvertOptions(options))
	if err := converter.BuildDocumentModel(docModel); err != nil {
		return nil, append(errs, err)
	}

	return converter.Schema(), nil
}
