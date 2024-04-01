package rest

import (
	"net/url"
	"testing"

	rest "github.com/hasura/ndc-rest-schema/schema"
	"github.com/hasura/ndc-rest/rest/internal"
	"github.com/hasura/ndc-sdk-go/schema"
	"github.com/hasura/ndc-sdk-go/utils"
	"github.com/stretchr/testify/assert"
)

func TestEvalQueryParameterURL(t *testing.T) {
	testCases := []struct {
		name     string
		param    *rest.RequestParameter
		keys     []string
		values   []string
		expected string
	}{
		{
			name:     "empty",
			param:    &rest.RequestParameter{},
			keys:     []string{""},
			values:   []string{},
			expected: "",
		},
		{
			name: "form_explode_single",
			param: &rest.RequestParameter{
				Name: "id",
				EncodingObject: rest.EncodingObject{
					Explode: utils.ToPtr(true),
					Style:   rest.EncodingStyleForm,
				},
			},
			keys:     []string{},
			values:   []string{"3"},
			expected: "id=3",
		},
		{
			name: "form_single",
			param: &rest.RequestParameter{
				Name: "id",
				EncodingObject: rest.EncodingObject{
					Explode: utils.ToPtr(false),
					Style:   rest.EncodingStyleForm,
				},
			},
			keys:     []string{""},
			values:   []string{"3"},
			expected: "id=3",
		},
		{
			name: "form_explode_multiple",
			param: &rest.RequestParameter{
				Name: "id",
				EncodingObject: rest.EncodingObject{
					Explode: utils.ToPtr(true),
					Style:   rest.EncodingStyleForm,
				},
			},
			keys:     []string{""},
			values:   []string{"3", "4", "5"},
			expected: "id=3&id=4&id=5",
		},
		{
			name: "spaceDelimited_multiple",
			param: &rest.RequestParameter{
				Name: "id",
				EncodingObject: rest.EncodingObject{
					Explode: utils.ToPtr(false),
					Style:   rest.EncodingStyleSpaceDelimited,
				},
			},
			keys:     []string{""},
			values:   []string{"3", "4", "5"},
			expected: "id=3 4 5",
		},
		{
			name: "spaceDelimited_explode_multiple",
			param: &rest.RequestParameter{
				Name: "id",
				EncodingObject: rest.EncodingObject{
					Explode: utils.ToPtr(true),
					Style:   rest.EncodingStyleSpaceDelimited,
				},
			},
			keys:     []string{""},
			values:   []string{"3", "4", "5"},
			expected: "id=3&id=4&id=5",
		},

		{
			name: "pipeDelimited_multiple",
			param: &rest.RequestParameter{
				Name: "id",
				EncodingObject: rest.EncodingObject{
					Explode: utils.ToPtr(false),
					Style:   rest.EncodingStylePipeDelimited,
				},
			},
			keys:     []string{""},
			values:   []string{"3", "4", "5"},
			expected: "id=3|4|5",
		},
		{
			name: "pipeDelimited_explode_multiple",
			param: &rest.RequestParameter{
				Name: "id",
				EncodingObject: rest.EncodingObject{
					Explode: utils.ToPtr(true),
					Style:   rest.EncodingStylePipeDelimited,
				},
			},
			keys:     []string{""},
			values:   []string{"3", "4", "5"},
			expected: "id=3&id=4&id=5",
		},
		{
			name: "deepObject_explode_multiple",
			param: &rest.RequestParameter{
				Name: "id",
				EncodingObject: rest.EncodingObject{
					Explode: utils.ToPtr(true),
					Style:   rest.EncodingStyleDeepObject,
				},
			},
			keys:     []string{""},
			values:   []string{"3", "4", "5"},
			expected: "id[]=3&id[]=4&id[]=5",
		},
		{
			name: "form_object",
			param: &rest.RequestParameter{
				Name: "id",
				EncodingObject: rest.EncodingObject{
					Explode: utils.ToPtr(false),
					Style:   rest.EncodingStyleForm,
				},
			},
			keys:     []string{"role"},
			values:   []string{"admin"},
			expected: "id=role,admin",
		},
		{
			name: "form_explode_object",
			param: &rest.RequestParameter{
				Name: "id",
				EncodingObject: rest.EncodingObject{
					Explode: utils.ToPtr(true),
					Style:   rest.EncodingStyleForm,
				},
			},
			keys:     []string{"role"},
			values:   []string{"admin"},
			expected: "role=admin",
		},
		{
			name: "deepObject_explode_object",
			param: &rest.RequestParameter{
				Name: "id",
				EncodingObject: rest.EncodingObject{
					Explode: utils.ToPtr(true),
					Style:   rest.EncodingStyleDeepObject,
				},
			},
			keys:     []string{"role"},
			values:   []string{"admin"},
			expected: "id[role]=admin",
		},
		{
			name: "form_array_object",
			param: &rest.RequestParameter{
				Name: "id",
				EncodingObject: rest.EncodingObject{
					Explode: utils.ToPtr(false),
					Style:   rest.EncodingStyleForm,
				},
			},
			keys:     []string{"role", "", "user", ""},
			values:   []string{"admin"},
			expected: "id=role[][user],admin",
		},
		{
			name: "form_explode_array_object",
			param: &rest.RequestParameter{
				Name: "id",
				EncodingObject: rest.EncodingObject{
					Explode: utils.ToPtr(true),
					Style:   rest.EncodingStyleForm,
				},
			},
			keys:     []string{"role", "", "user", ""},
			values:   []string{"admin"},
			expected: "role[][user]=admin",
		},
		{
			name: "form_explode_array_object_multiple",
			param: &rest.RequestParameter{
				Name: "id",
				EncodingObject: rest.EncodingObject{
					Explode: utils.ToPtr(true),
					Style:   rest.EncodingStyleForm,
				},
			},
			keys:     []string{"role", "", "user", ""},
			values:   []string{"admin", "anonymous"},
			expected: "id[role][][user]=admin&id[role][][user]=anonymous",
		},
		{
			name: "deepObject_explode_array_object",
			param: &rest.RequestParameter{
				Name: "id",
				EncodingObject: rest.EncodingObject{
					Explode: utils.ToPtr(true),
					Style:   rest.EncodingStyleDeepObject,
				},
			},
			keys:     []string{"role", "", "user", ""},
			values:   []string{"admin"},
			expected: "id[role][][user][]=admin",
		},
		{
			name: "deepObject_explode_array_object_multiple",
			param: &rest.RequestParameter{
				Name: "id",
				EncodingObject: rest.EncodingObject{
					Explode: utils.ToPtr(true),
					Style:   rest.EncodingStyleDeepObject,
				},
			},
			keys:     []string{"role", "", "user", ""},
			values:   []string{"admin", "anonymous"},
			expected: "id[role][][user][]=admin&id[role][][user][]=anonymous",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			qValues := make(url.Values)
			evalQueryParameterURL(&qValues, tc.param, tc.keys, tc.values)
			assert.Equal(t, tc.expected, encodeQueryValues(qValues, true))
		})
	}
}

func TestEncodeParameterValues(t *testing.T) {
	testCases := []struct {
		name         string
		param        *rest.RequestParameter
		argumentType schema.Type
		value        any
		expected     internal.StringSlicePairs
		errorMsg     string
	}{
		// {
		// 	name: "/accounts/6HnaGHbBVR/people?ending_before=HhiGxs4p8E&expand[]=yH6KUbPKiZ&limit=7262&relationship[director]=true&relationship[executive]=false&relationship[legal_guardian]=true&relationship[owner]=true&relationship[representative]=true&starting_after=1qRvRTbUNd",
		// },
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			c := RESTConnector{}
			result, err := c.encodeParameterValues(tc.param, tc.argumentType, tc.value)
			if tc.errorMsg != "" {
				assert.ErrorContains(t, err, tc.errorMsg)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expected, result)
			}
		})
	}

}
