package rest

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"

	rest "github.com/hasura/ndc-rest-schema/schema"
	"github.com/hasura/ndc-rest/rest/internal"
	"github.com/hasura/ndc-sdk-go/schema"
	"github.com/hasura/ndc-sdk-go/utils"
	"gotest.tools/v3/assert"
)

func TestEvalQueryParameterURL(t *testing.T) {
	testCases := []struct {
		name     string
		param    *rest.RequestParameter
		keys     []internal.Key
		values   []string
		expected string
	}{
		{
			name:     "empty",
			param:    &rest.RequestParameter{},
			keys:     []internal.Key{internal.NewKey("")},
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
			keys:     []internal.Key{},
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
			keys:     []internal.Key{internal.NewKey("")},
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
			keys:     []internal.Key{internal.NewKey("")},
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
			keys:     []internal.Key{internal.NewKey("")},
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
			keys:     []internal.Key{internal.NewKey("")},
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
			keys:     []internal.Key{internal.NewKey("")},
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
			keys:     []internal.Key{internal.NewKey("")},
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
			keys:     []internal.Key{internal.NewKey("")},
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
			keys:     []internal.Key{internal.NewKey("role")},
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
			keys:     []internal.Key{internal.NewKey("role")},
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
			keys:     []internal.Key{internal.NewKey("role")},
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
			keys:     []internal.Key{internal.NewKey("role"), internal.NewKey(""), internal.NewKey("user"), internal.NewKey("")},
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
			keys:     []internal.Key{internal.NewKey("role"), internal.NewKey(""), internal.NewKey("user"), internal.NewKey("")},
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
			keys:     []internal.Key{internal.NewKey("role"), internal.NewKey(""), internal.NewKey("user"), internal.NewKey("")},
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
			keys:     []internal.Key{internal.NewKey("role"), internal.NewKey(""), internal.NewKey("user"), internal.NewKey("")},
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
			keys:     []internal.Key{internal.NewKey("role"), internal.NewKey(""), internal.NewKey("user"), internal.NewKey("")},
			values:   []string{"admin", "anonymous"},
			expected: "id[role][][user][]=admin&id[role][][user][]=anonymous",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			qValues := make(url.Values)
			evalQueryParameterURL(&qValues, tc.param.Name, tc.param.EncodingObject, tc.keys, tc.values)
			assert.Equal(t, tc.expected, encodeQueryValues(qValues, true))
		})
	}
}

func TestEncodeParameterValues(t *testing.T) {
	testCases := []struct {
		name               string
		rawProcedureSchema string
		rawArguments       string
		expectedURL        string
		errorMsg           string
	}{

		{
			name: "/param-array",
			rawProcedureSchema: `{
				"request": {
					"url": "/param-array",
					"method": "post",
					"parameters": [
						{
							"style": "deepObject",
							"explode": true,
							"name": "expand",
							"in": "query",
							"schema": {
								"type": "array",
								"nullable": true,
								"items": {
									"type": "string"
								}
							}
						}
					],
					"requestBody": {
						"contentType": "application/x-www-form-urlencoded"
					}
				},
				"arguments": {
					"expand": {
						"type": {
							"type": "nullable",
							"underlying_type": {
								"type": "array",
								"element_type": {
									"name": "String",
									"type": "named"
								}
							} 
						}
					}
				},
				"name": "PostCheckoutSessions",
				"result_type": {
					"name": "Order",
					"type": "named"
				}
			}`,
			rawArguments: `{
				"expand": ["foo"]
			}`,
			expectedURL: "/param-array?expand[]=foo",
		},
		{
			name: "/v1/checkout/sessions",
			rawProcedureSchema: `{
				"request": {
					"url": "/v1/checkout/sessions",
					"method": "post",
					"parameters": [
						{
							"style": "deepObject",
							"explode": true,
							"name": "automatic_tax",
							"in": "query",
							"schema": {
								"type": "object",
								"nullable": true,
								"properties": {
									"enabled": {
										"type": "boolean"
									},
									"liability": {
										"type": "object",
										"nullable": true,
										"properties": {
											"account": {
												"type": "string",
												"nullable": true
											},
											"type": {
												"type": "string",
												"enum": [
													"account",
													"self"
												]
											}
										}
									}
								}
							}
						},
						{
							"style": "deepObject",
							"explode": true,
							"name": "subscription_data",
							"in": "query",
							"schema": {
								"type": "object",
								"nullable": true,
								"properties": {
									"application_fee_percent": {
										"type": "number",
										"nullable": true
									},
									"billing_cycle_anchor": {
										"type": "integer",
										"format": "unix-time",
										"nullable": true
									},
									"default_tax_rates": {
										"type": "array",
										"nullable": true,
										"items": {
											"type": "string",
											"maxLength": 5000
										}
									},
									"description": {
										"type": "string",
										"nullable": true,
										"maxLength": 500
									},
									"invoice_settings": {
										"type": "object",
										"nullable": true,
										"properties": {
											"issuer": {
												"type": "object",
												"nullable": true,
												"properties": {
													"account": {
														"type": "string",
														"nullable": true
													},
													"type": {
														"type": "string",
														"enum": [
															"account",
															"self"
														]
													}
												}
											}
										}
									},
									"metadata": {
										"type": "JSON",
										"nullable": true
									},
									"on_behalf_of": {
										"type": "string",
										"nullable": true
									},
									"proration_behavior": {
										"type": "string",
										"nullable": true,
										"enum": [
											"create_prorations",
											"none"
										]
									},
									"transfer_data": {
										"type": "object",
										"nullable": true,
										"properties": {
											"amount_percent": {
												"type": "number",
												"nullable": true
											},
											"destination": {
												"type": "string"
											}
										}
									},
									"trial_end": {
										"type": "integer",
										"format": "unix-time",
										"nullable": true
									},
									"trial_period_days": {
										"type": "integer",
										"nullable": true
									},
									"trial_settings": {
										"type": "object",
										"nullable": true,
										"properties": {
											"end_behavior": {
												"type": "object",
												"properties": {
													"missing_payment_method": {
														"type": "string",
														"enum": [
															"cancel",
															"create_invoice",
															"pause"
														]
													}
												}
											}
										}
									}
								}
							}
						}
					],
					"requestBody": {
						"contentType": "application/x-www-form-urlencoded"
					}
				},
				"arguments": {
					"automatic_tax": {
						"description": "Settings for automatic tax lookup for this session and resulting payments, invoices, and subscriptions.",
						"type": {
							"type": "nullable",
							"underlying_type": {
								"name": "AutomaticTaxParams",
								"type": "named"
							}
						}
					},
					"subscription_data": {
						"type": {
							"type": "nullable",
							"underlying_type": {
								"name": "SubscriptionDataParams",
								"type": "named"
							}
						}
					}
				},
				"name": "PostCheckoutSessions",
				"result_type": {
					"name": "Order",
					"type": "named"
				}
			}`,
			rawArguments: `{
				"automatic_tax": {
					"enabled": false,
					"liability": {
						"type": "self",
						"country": "DE"
					}
				},
				"subscription_data": {
          "description": "nyxWwjZ0JY",
          "invoice_settings": {
            "issuer": {
              "type": "self",
              "country": "IT"
            }
          },
          "metadata": null,
          "trial_period_days": 27623,
          "trial_settings": {
            "end_behavior": {
              "missing_payment_method": "cancel"
            }
          }
        }
			}`,
			expectedURL: "/v1/checkout/sessions?automatic_tax[enabled]=false&automatic_tax[liability][type]=self&subscription_data[description]=nyxWwjZ0JY&subscription_data[invoice_settings][issuer][type]=self&subscription_data[trial_period_days]=27623&subscription_data[trial_settings][end_behavior][missing_payment_method]=cancel",
		},
		{
			name: "/v1/subscription_schedules",
			rawProcedureSchema: `{
				"request": {
					"url": "/v1/subscription_schedules",
					"method": "post",
					"parameters": [
						{
							"style": "deepObject",
							"explode": true,
							"name": "phases",
							"in": "query",
							"schema": {
								"type": "array",
								"nullable": true,
								"items": {
									"type": "object",
									"properties": {
										"add_invoice_items": {
											"type": "array",
											"nullable": true,
											"items": {
												"type": "object",
												"properties": {
													"price": {
														"type": "String",
														"nullable": true,
														"maxLength": 5000
													},
													"price_data": {
														"type": "object",
														"nullable": true,
														"properties": {
															"currency": {
																"type": "String"
															},
															"product": {
																"type": "String",
																"maxLength": 5000
															},
															"tax_behavior": {
																"type": "SubscriptionSchedulesTaxBehavior",
																"nullable": true
															},
															"unit_amount": {
																"type": "Int32",
																"nullable": true
															},
															"unit_amount_decimal": {
																"type": "String",
																"nullable": true
															}
														}
													},
													"quantity": {
														"type": "Int32",
														"nullable": true
													}
												}
											}
										},
										"items": {
											"type": "array",
											"items": {
												"type": "object",
												"properties": {
													"price": {
														"type": "String",
														"nullable": true,
														"maxLength": 5000
													},
													"price_data": {
														"type": "object",
														"nullable": true,
														"properties": {
															"currency": {
																"type": "String"
															},
															"product": {
																"type": "String",
																"maxLength": 5000
															},
															"recurring": {
																"type": "object",
																"properties": {
																	"interval": {
																		"type": "SubscriptionSchedulesInterval"
																	},
																	"interval_count": {
																		"type": "Int32",
																		"nullable": true
																	}
																}
															},
															"tax_behavior": {
																"type": "SubscriptionSchedulesTaxBehavior",
																"nullable": true
															},
															"unit_amount": {
																"type": "Int32",
																"nullable": true
															},
															"unit_amount_decimal": {
																"type": "String",
																"nullable": true
															}
														}
													},
													"quantity": {
														"type": "Int32",
														"nullable": true
													}
												}
											}
										}
									}
								}
							}
						}
					],
					"requestBody": {
						"contentType": "application/x-www-form-urlencoded"
					}
				},
				"arguments": {
					"phases": {
						"type": {
							"type": "nullable",
							"underlying_type": {
								"element_type": {
									"name": "PostSubscriptionSchedulesScheduleBodyPhases",
									"type": "named"
								},
								"type": "array"
							}
						}
					}
				},
				"name": "PostSubscriptionSchedulesSchedule",
				"result_type": {
					"name": "SubscriptionSchedule",
					"type": "named"
				}
			}`,
			rawArguments: `{
				"phases": [
					{
						"add_invoice_items": [
							{
								"price": "Brnx6F2SW3",
								"price_data": {
									"currency": "KutyN1a7f7",
									"product": "TS7Fs9Hy8B",
									"tax_behavior": "exclusive",
									"unit_amount": 2120841752,
									"unit_amount_decimal": "GxHaMm19uk"
								},
								"quantity": 1365407829
							}
						],
						"items": [
							{
								"price": "wsJvXbiSV8",
								"price_data": {
									"currency": "Se3SC2fQcl",
									"product": "W9pqDICERA",
									"recurring": {
										"interval": "month",
										"interval_count": 97217333
									},
									"tax_behavior": "inclusive",
									"unit_amount": 1972558655,
									"unit_amount_decimal": "uu9JdD5mJ0"
								},
								"quantity": 1961565488
							}
						]
					}
				]
			}`,
			expectedURL: "/v1/subscription_schedules?phases[0][add_invoice_items][0][price]=Brnx6F2SW3&phases[0][add_invoice_items][0][price_data][currency]=KutyN1a7f7&phases[0][add_invoice_items][0][price_data][product]=TS7Fs9Hy8B&phases[0][add_invoice_items][0][price_data][tax_behavior]=exclusive&phases[0][add_invoice_items][0][price_data][unit_amount]=2120841752&phases[0][add_invoice_items][0][price_data][unit_amount_decimal]=GxHaMm19uk&phases[0][add_invoice_items][0][quantity]=1365407829&phases[0][items][0][price]=wsJvXbiSV8&phases[0][items][0][price_data][currency]=Se3SC2fQcl&phases[0][items][0][price_data][product]=W9pqDICERA&phases[0][items][0][price_data][recurring][interval]=month&phases[0][items][0][price_data][recurring][interval_count]=97217333&phases[0][items][0][price_data][tax_behavior]=inclusive&phases[0][items][0][price_data][unit_amount]=1972558655&phases[0][items][0][price_data][unit_amount_decimal]=uu9JdD5mJ0&phases[0][items][0][quantity]=1961565488",
		},
	}

	connector := RESTConnector{
		schema: &schema.SchemaResponse{
			ScalarTypes: schema.SchemaResponseScalarTypes{
				"V1Type": schema.ScalarType{
					AggregateFunctions:  schema.ScalarTypeAggregateFunctions{},
					ComparisonOperators: make(map[string]schema.ComparisonOperatorDefinition),
					Representation: schema.NewTypeRepresentationEnum([]string{
						"account",
						"self",
					}).Encode(),
				},
				"String": schema.ScalarType{
					AggregateFunctions:  schema.ScalarTypeAggregateFunctions{},
					ComparisonOperators: make(map[string]schema.ComparisonOperatorDefinition),
					Representation:      schema.NewTypeRepresentationString().Encode(),
				},
				"JSON": schema.ScalarType{
					AggregateFunctions:  schema.ScalarTypeAggregateFunctions{},
					ComparisonOperators: make(map[string]schema.ComparisonOperatorDefinition),
					Representation:      schema.NewTypeRepresentationJSON().Encode(),
				},
				"Int32": schema.ScalarType{
					AggregateFunctions:  schema.ScalarTypeAggregateFunctions{},
					ComparisonOperators: make(map[string]schema.ComparisonOperatorDefinition),
					Representation:      schema.NewTypeRepresentationInt32().Encode(),
				},
			},
			ObjectTypes: schema.SchemaResponseObjectTypes{
				"AutomaticTaxParams": schema.ObjectType{
					Fields: schema.ObjectTypeFields{
						"enabled": schema.ObjectField{
							Type: schema.NewNamedType("Boolean").Encode(),
						},
						"liability": schema.ObjectField{
							Type: schema.NewNullableNamedType("Param").Encode(),
						},
					},
				},
				"Param": schema.ObjectType{
					Fields: schema.ObjectTypeFields{
						"account": schema.ObjectField{
							Type: schema.NewNullableNamedType("String").Encode(),
						},
						"type": schema.ObjectField{
							Type: schema.NewNamedType("V1Type").Encode(),
						},
					},
				},
				"SubscriptionDataParams": schema.ObjectType{
					Fields: schema.ObjectTypeFields{
						"description": {
							Type: schema.NewNullableNamedType("String").Encode(),
						},
						"invoice_settings": {
							Type: schema.NewNullableNamedType("SubscriptionDataInvoiceSettingsParams").Encode(),
						},
						"metadata": {
							Type: schema.NewNullableNamedType("JSON").Encode(),
						},
						"trial_period_days": {
							Type: schema.NewNullableNamedType("Int32").Encode(),
						},
					},
				},
				"SubscriptionDataInvoiceSettingsParams": schema.ObjectType{
					Fields: schema.ObjectTypeFields{
						"issuer": schema.ObjectField{
							Type: schema.NewNullableNamedType("Param").Encode(),
						},
					},
				},
				"PostSubscriptionSchedulesScheduleBodyPhases": schema.ObjectType{
					Fields: schema.ObjectTypeFields{
						"add_invoice_items": schema.ObjectField{
							Type: schema.NewNullableType(schema.NewArrayType(schema.NewNamedType("PostSubscriptionSchedulesScheduleBodyPhasesAddInvoiceItems"))).Encode(),
						},
						"items": schema.ObjectField{
							Type: schema.NewArrayType(schema.NewNamedType("PostSubscriptionSchedulesScheduleBodyPhasesItems")).Encode(),
						},
					},
				},
				"PostSubscriptionSchedulesScheduleBodyPhasesAddInvoiceItems": schema.ObjectType{
					Fields: schema.ObjectTypeFields{
						"price": schema.ObjectField{
							Type: schema.NewNullableNamedType("String").Encode(),
						},
						"price_data": schema.ObjectField{
							Type: schema.NewNullableNamedType("PostSubscriptionSchedulesScheduleBodyPhasesAddInvoiceItemsPriceData").Encode(),
						},
						"quantity": schema.ObjectField{
							Type: schema.NewNullableNamedType("Int32").Encode(),
						},
					},
				},
				"PostSubscriptionSchedulesScheduleBodyPhasesAddInvoiceItemsPriceData": schema.ObjectType{
					Fields: schema.ObjectTypeFields{
						"currency": schema.ObjectField{
							Type: schema.NewNamedType("String").Encode(),
						},
						"product": schema.ObjectField{
							Type: schema.NewNamedType("String").Encode(),
						},
						"tax_behavior": schema.ObjectField{
							Type: schema.NewNullableNamedType("SubscriptionSchedulesTaxBehavior").Encode(),
						},
						"unit_amount": schema.ObjectField{
							Type: schema.NewNullableNamedType("Int32").Encode(),
						},
						"unit_amount_decimal": schema.ObjectField{
							Type: schema.NewNullableNamedType("String").Encode(),
						},
					},
				},
				"PostSubscriptionSchedulesScheduleBodyPhasesItems": schema.ObjectType{
					Fields: schema.ObjectTypeFields{
						"price": schema.ObjectField{
							Type: schema.NewNullableNamedType("String").Encode(),
						},
						"price_data": schema.ObjectField{
							Type: schema.NewNullableNamedType("PostSubscriptionSchedulesScheduleBodyPhasesItemsPriceData").Encode(),
						},
						"quantity": schema.ObjectField{
							Type: schema.NewNullableNamedType("Int32").Encode(),
						},
					},
				},
				"PostSubscriptionSchedulesScheduleBodyPhasesItemsPriceData": schema.ObjectType{
					Fields: schema.ObjectTypeFields{
						"currency": schema.ObjectField{
							Type: schema.NewNamedType("String").Encode(),
						},
						"product": schema.ObjectField{
							Type: schema.NewNamedType("String").Encode(),
						},
						"recurring": schema.ObjectField{
							Type: schema.NewNamedType("PostSubscriptionSchedulesScheduleBodyPhasesItemsPriceDataRecurring").Encode(),
						},
						"tax_behavior": schema.ObjectField{
							Type: schema.NewNullableNamedType("SubscriptionSchedulesTaxBehavior").Encode(),
						},
						"unit_amount": schema.ObjectField{
							Type: schema.NewNullableNamedType("Int32").Encode(),
						},
						"unit_amount_decimal": schema.ObjectField{
							Type: schema.NewNullableNamedType("String").Encode(),
						},
					},
				},
				"PostSubscriptionSchedulesScheduleBodyPhasesItemsPriceDataRecurring": schema.ObjectType{
					Fields: schema.ObjectTypeFields{
						"interval": schema.ObjectField{
							Type: schema.NewNamedType("SubscriptionSchedulesInterval").Encode(),
						},
						"interval_count": schema.ObjectField{
							Type: schema.NewNullableNamedType("Int32").Encode(),
						},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var info rest.RESTProcedureInfo
			assert.NilError(t, json.Unmarshal([]byte(tc.rawProcedureSchema), &info))

			var arguments map[string]any
			assert.NilError(t, json.Unmarshal([]byte(tc.rawArguments), &arguments))

			result, _, err := connector.evalURLAndHeaderParameters(info.Request, info.Arguments, arguments, http.Header{})
			if tc.errorMsg != "" {
				assert.ErrorContains(t, err, tc.errorMsg)
			} else {
				assert.NilError(t, err)
				decodedValue, err := url.QueryUnescape(result)
				assert.NilError(t, err)
				assert.Equal(t, tc.expectedURL, decodedValue)
			}
		})
	}
}
