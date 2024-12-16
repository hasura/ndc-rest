package contenttype

import (
	"encoding/json"
	"io"
	"net/url"
	"slices"
	"strings"
	"testing"

	rest "github.com/hasura/ndc-http/ndc-http-schema/schema"
	"github.com/hasura/ndc-sdk-go/utils"
	"gotest.tools/v3/assert"
)

func TestEvalQueryParameterURL(t *testing.T) {
	testCases := []struct {
		name     string
		param    *rest.RequestParameter
		keys     []Key
		values   []string
		expected string
	}{
		{
			name:     "empty",
			param:    &rest.RequestParameter{},
			keys:     []Key{NewKey("")},
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
			keys:     []Key{},
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
			keys:     []Key{NewKey("")},
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
			keys:     []Key{NewKey("")},
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
			keys:     []Key{NewKey("")},
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
			keys:     []Key{NewKey("")},
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
			keys:     []Key{NewKey("")},
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
			keys:     []Key{NewKey("")},
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
			keys:     []Key{NewKey("")},
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
			keys:     []Key{NewKey("role")},
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
			keys:     []Key{NewKey("role")},
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
			keys:     []Key{NewKey("role")},
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
			keys:     []Key{NewKey("role"), NewKey(""), NewKey("user"), NewKey("")},
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
			keys:     []Key{NewKey("role"), NewKey(""), NewKey("user"), NewKey("")},
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
			keys:     []Key{NewKey("role"), NewKey(""), NewKey("user"), NewKey("")},
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
			keys:     []Key{NewKey("role"), NewKey(""), NewKey("user"), NewKey("")},
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
			keys:     []Key{NewKey("role"), NewKey(""), NewKey("user"), NewKey("")},
			values:   []string{"admin", "anonymous"},
			expected: "id[role][][user][]=admin&id[role][][user][]=anonymous",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			qValues := make(url.Values)
			EvalQueryParameterURL(&qValues, tc.param.Name, tc.param.EncodingObject, tc.keys, tc.values)
			assert.Equal(t, tc.expected, EncodeQueryValues(qValues, true))
		})
	}
}

func TestCreateFormURLEncoded(t *testing.T) {
	testCases := []struct {
		Name         string
		RawArguments string
		Expected     string
	}{
		{
			Name: "PostCheckoutSessions",
			RawArguments: `{
        "body": {
          "after_expiration": {
            "recovery": {
              "allow_promotion_codes": true,
              "enabled": true
            }
          },
          "allow_promotion_codes": true,
          "automatic_tax": {
            "enabled": false,
            "liability": {
              "account": "gW7D0WhP9C",
              "type": "self"
            }
          },
          "billing_address_collection": "auto",
          "cancel_url": "qpmWppPyIv",
          "client_reference_id": "ZcJeCf6JAa",
          "consent_collection": {
            "payment_method_reuse_agreement": {
              "position": "auto"
            },
            "promotions": "auto",
            "terms_of_service": "required"
          },
          "currency": "oVljMB8lon",
          "custom_fields": [
            {
              "dropdown": {
                "options": [
                  {
                    "label": "W3oysCi31d",
                    "value": "hXN8MppU0k"
                  }
                ]
              },
              "key": "5ZeyjIHLn8",
              "label": {
                "custom": "uabTz3xzdn",
                "type": "custom"
              },
              "numeric": {
                "maximum_length": 678468035,
                "minimum_length": 2134997439
              },
              "optional": false,
              "text": {
                "maximum_length": 331815114,
                "minimum_length": 1689246767
              },
              "type": "dropdown"
            }
          ],
          "custom_text": {
            "after_submit": {
              "message": "b7ifuedi9S"
            },
            "shipping_address": {
              "message": "XeD5TkmC8k"
            },
            "submit": {
              "message": "vGcSz5eSlo"
            },
            "terms_of_service_acceptance": {
              "message": "zGLTTZItPl"
            }
          },
          "customer": "mT4BKOSu9s",
          "customer_creation": "always",
          "customer_email": "1xiCJ8M7Pr",
          "customer_update": {
            "address": "never",
            "name": "never",
            "shipping": "auto"
          },
          "discounts": [
            {
              "coupon": "tOlEXiZKv9",
              "promotion_code": "Xknj8juRnm"
            }
          ],
          "expand": ["ZBxEXz7SN0"],
          "expires_at": 1756067225,
          "invoice_creation": {
            "enabled": true,
            "invoice_data": {
              "account_tax_ids": ["dev8vFF6xG"],
              "custom_fields": [
                {
                  "name": "LBlZjJ4gEy",
                  "value": "EWoKgkV3fg"
                }
              ],
              "description": "MiePp9LfkQ",
              "footer": "OAELqbYbKV",
              "issuer": {
                "account": "aqOwDzxnyg",
                "type": "account"
              },
              "metadata": null,
              "rendering_options": {
                "amount_tax_display": "exclude_tax"
              }
            }
          },
          "line_items": [
            {
              "adjustable_quantity": {
                "enabled": false,
                "maximum": 1665059759,
                "minimum": 905088217
              },
              "dynamic_tax_rates": ["jMMvH8TmQD"],
              "price": "fR6vnvprv8",
              "price_data": {
                "currency": "euIDO8C4A7",
                "product": "xilQ2QDVdA",
                "product_data": {
                  "description": "DQECtJEsLI",
                  "images": ["gE5K8MOzRc"],
                  "metadata": null,
                  "name": "ak6UVjXl1B",
                  "tax_code": "PzbIHvqWJp"
                },
                "recurring": {
                  "interval": "day",
                  "interval_count": 592739346
                },
                "tax_behavior": "inclusive",
                "unit_amount": 945322526,
                "unit_amount_decimal": "vkJPCvrn9Q"
              },
              "quantity": 968305911,
              "tax_rates": ["Ts1bPAoT0T"]
            }
          ],
          "locale": "auto",
          "metadata": null,
          "mode": "payment",
          "payment_intent_data": {
            "application_fee_amount": 2033958571,
            "capture_method": "manual",
            "description": "yoalRHw9ZG",
            "metadata": null,
            "on_behalf_of": "mpkGzXu3st",
            "receipt_email": "LxJLYGjJ4r",
            "setup_future_usage": "off_session",
            "shipping": {
              "address": {
                "city": "v6nZI33cUt",
                "country": "O8MBVcia7c",
                "line1": "3YghEmysVn",
                "line2": "CM9x9Jizzu",
                "postal_code": "1aAilmcYiq",
                "state": "ILODDWP1IP"
              },
              "carrier": "P8mCJlEq1J",
              "name": "mJYqgRIh3S",
              "phone": "CWAbvZM4Kw",
              "tracking_number": "XGOZIrLZf0"
            },
            "statement_descriptor": "JCOo6lU8Fy",
            "statement_descriptor_suffix": "dtPJwyuc4i",
            "transfer_data": {
              "amount": 94957585,
              "destination": "LrcNMrJPkO"
            },
            "transfer_group": "XKfPQPVhOT"
          },
          "payment_method_collection": "always",
          "payment_method_configuration": "uwYSwIZP4V",
          "payment_method_options": {
            "acss_debit": {
              "currency": "usd",
              "mandate_options": {
                "custom_mandate_url": "FZwPtJKktL",
                "default_for": ["invoice"],
                "interval_description": "iMgay8S9If",
                "payment_schedule": "sporadic",
                "transaction_type": "business"
              },
              "setup_future_usage": "off_session",
              "verification_method": "instant"
            },
            "affirm": {
              "setup_future_usage": "none"
            },
            "afterpay_clearpay": {
              "setup_future_usage": "none"
            },
            "alipay": {
              "setup_future_usage": "none"
            },
            "au_becs_debit": {
              "setup_future_usage": "none"
            },
            "bacs_debit": {
              "setup_future_usage": "off_session"
            },
            "bancontact": {
              "setup_future_usage": "none"
            },
            "boleto": {
              "expires_after_days": 953467886,
              "setup_future_usage": "none"
            },
            "card": {
              "installments": {
                "enabled": true
              },
              "request_three_d_secure": "any",
              "setup_future_usage": "on_session",
              "statement_descriptor_suffix_kana": "ZvJtIONyDK",
              "statement_descriptor_suffix_kanji": "Y57zexRcIH"
            },
            "cashapp": {
              "setup_future_usage": "off_session"
            },
            "customer_balance": {
              "bank_transfer": {
                "eu_bank_transfer": {
                  "country": "mzrVWAjBTc"
                },
                "requested_address_types": ["iban"],
                "type": "gb_bank_transfer"
              },
              "funding_type": "bank_transfer",
              "setup_future_usage": "none"
            },
            "eps": {
              "setup_future_usage": "none"
            },
            "fpx": {
              "setup_future_usage": "none"
            },
            "giropay": {
              "setup_future_usage": "none"
            },
            "grabpay": {
              "setup_future_usage": "none"
            },
            "ideal": {
              "setup_future_usage": "none"
            },
            "klarna": {
              "setup_future_usage": "none"
            },
            "konbini": {
              "expires_after_days": 664583520,
              "setup_future_usage": "none"
            },
            "link": {
              "setup_future_usage": "none"
            },
            "mobilepay": {
              "setup_future_usage": "none"
            },
            "oxxo": {
              "expires_after_days": 1925345768,
              "setup_future_usage": "none"
            },
            "p24": {
              "setup_future_usage": "none",
              "tos_shown_and_accepted": true
            },
            "paynow": {
              "setup_future_usage": "none"
            },
            "paypal": {
              "capture_method": "manual",
              "preferred_locale": "cs-CZ",
              "reference": "ulLn2NXA1P",
              "risk_correlation_id": "fj1J6Nux6P",
              "setup_future_usage": "none"
            },
            "pix": {
              "expires_after_seconds": 191312234
            },
            "revolut_pay": {
              "setup_future_usage": "off_session"
            },
            "sepa_debit": {
              "setup_future_usage": "none"
            },
            "sofort": {
              "setup_future_usage": "none"
            },
            "swish": {
              "reference": "rXJq1EX4rc"
            },
            "us_bank_account": {
              "financial_connections": {
                "permissions": ["ownership"],
                "prefetch": ["transactions"]
              },
              "setup_future_usage": "none",
              "verification_method": "automatic"
            },
            "wechat_pay": {
              "app_id": "9Pu0d1pZ2r",
              "client": "ios",
              "setup_future_usage": "none"
            }
          },
          "payment_method_types": ["acss_debit"],
          "phone_number_collection": {
            "enabled": true
          },
          "redirect_on_completion": "never",
          "return_url": "YgIdKykEHC",
          "setup_intent_data": {
            "description": "U9qFTQnt1W",
            "metadata": null,
            "on_behalf_of": "165u5Fvodj"
          },
          "shipping_address_collection": {
            "allowed_countries": ["AC"]
          },
          "shipping_options": [
            {
              "shipping_rate": "5PAjqTpMjw",
              "shipping_rate_data": {
                "delivery_estimate": {
                  "maximum": {
                    "unit": "week",
                    "value": 479399576
                  },
                  "minimum": {
                    "unit": "day",
                    "value": 1640284987
                  }
                },
                "display_name": "PXozGQQnBA",
                "fixed_amount": {
                  "amount": 2040036333,
                  "currency": "KkRL3jvZMO",
                  "currency_options": null
                },
                "metadata": null,
                "tax_behavior": "exclusive",
                "tax_code": "NKSQxYdCfO",
                "type": "fixed_amount"
              }
            }
          ],
          "submit_type": "donate",
          "subscription_data": {
            "application_fee_percent": 1.7020678102144877,
            "billing_cycle_anchor": 1981798554,
            "default_tax_rates": ["b3jgFBJq4f"],
            "description": "7mpaD2E0jf",
            "invoice_settings": {
              "issuer": {
                "account": "axhiYamJKY",
                "type": "account"
              }
            },
            "metadata": null,
            "on_behalf_of": "oGsMnSifXV",
            "proration_behavior": "create_prorations",
            "transfer_data": {
              "amount_percent": 1.5805719275050356,
              "destination": "wzJ3U1Tyhd"
            },
            "trial_end": 606476058,
            "trial_period_days": 1684102049,
            "trial_settings": {
              "end_behavior": {
                "missing_payment_method": "create_invoice"
              }
            }
          },
          "success_url": "hDTwi34TAz",
          "tax_id_collection": {
            "enabled": true
          },
          "ui_mode": "hosted"
        }
      }`,
			Expected: "payment_method_options[bacs_debit][setup_future_usage]=off_session&payment_intent_data[shipping][address][line1]=3YghEmysVn&consent_collection[payment_method_reuse_agreement][position]=auto&payment_method_options[au_becs_debit][setup_future_usage]=none&payment_method_options[customer_balance][bank_transfer][eu_bank_transfer][country]=mzrVWAjBTc&payment_method_options[acss_debit][verification_method]=instant&payment_intent_data[transfer_group]=XKfPQPVhOT&line_items[0][price_data][currency]=euIDO8C4A7&invoice_creation[invoice_data][footer]=OAELqbYbKV&payment_method_options[us_bank_account][verification_method]=automatic&payment_method_options[cashapp][setup_future_usage]=off_session&payment_method_options[konbini][expires_after_days]=664583520&subscription_data[default_tax_rates][]=b3jgFBJq4f&line_items[0][price_data][product_data][tax_code]=PzbIHvqWJp&line_items[0][adjustable_quantity][minimum]=905088217&currency=oVljMB8lon&invoice_creation[invoice_data][issuer][account]=aqOwDzxnyg&success_url=hDTwi34TAz&payment_method_options[giropay][setup_future_usage]=none&payment_method_options[oxxo][setup_future_usage]=none&payment_method_options[acss_debit][currency]=usd&payment_method_options[acss_debit][mandate_options][payment_schedule]=sporadic&payment_intent_data[shipping][name]=mJYqgRIh3S&subscription_data[transfer_data][amount_percent]=1.5805719275050356&custom_fields[0][label][custom]=uabTz3xzdn&customer_update[name]=never&payment_method_options[acss_debit][mandate_options][interval_description]=iMgay8S9If&automatic_tax[liability][type]=self&payment_intent_data[capture_method]=manual&custom_fields[0][dropdown][options][0][label]=W3oysCi31d&custom_fields[0][text][maximum_length]=331815114&custom_text[terms_of_service_acceptance][message]=zGLTTZItPl&invoice_creation[enabled]=true&shipping_options[0][shipping_rate]=5PAjqTpMjw&shipping_options[0][shipping_rate_data][delivery_estimate][minimum][unit]=day&payment_method_types[]=acss_debit&payment_intent_data[application_fee_amount]=2033958571&custom_text[after_submit][message]=b7ifuedi9S&shipping_options[0][shipping_rate_data][delivery_estimate][minimum][value]=1640284987&payment_method_options[afterpay_clearpay][setup_future_usage]=none&payment_method_options[alipay][setup_future_usage]=none&payment_method_options[us_bank_account][financial_connections][permissions][]=ownership&mode=payment&line_items[0][quantity]=968305911&return_url=YgIdKykEHC&shipping_options[0][shipping_rate_data][delivery_estimate][maximum][value]=479399576&payment_method_options[paypal][reference]=ulLn2NXA1P&subscription_data[transfer_data][destination]=wzJ3U1Tyhd&customer=mT4BKOSu9s&submit_type=donate&payment_method_options[boleto][setup_future_usage]=none&payment_method_options[acss_debit][mandate_options][transaction_type]=business&payment_intent_data[shipping][address][city]=v6nZI33cUt&custom_fields[0][type]=dropdown&invoice_creation[invoice_data][custom_fields][0][name]=LBlZjJ4gEy&shipping_options[0][shipping_rate_data][tax_behavior]=exclusive&customer_email=1xiCJ8M7Pr&invoice_creation[invoice_data][issuer][type]=account&payment_method_options[customer_balance][funding_type]=bank_transfer&payment_intent_data[shipping][address][state]=ILODDWP1IP&subscription_data[proration_behavior]=create_prorations&line_items[0][price_data][product_data][name]=ak6UVjXl1B&invoice_creation[invoice_data][custom_fields][0][value]=EWoKgkV3fg&shipping_options[0][shipping_rate_data][type]=fixed_amount&payment_method_options[link][setup_future_usage]=none&expand[]=ZBxEXz7SN0&subscription_data[invoice_settings][issuer][type]=account&payment_method_collection=always&customer_update[address]=never&payment_method_options[wechat_pay][setup_future_usage]=none&customer_creation=always&payment_method_options[card][statement_descriptor_suffix_kanji]=Y57zexRcIH&payment_method_options[p24][setup_future_usage]=none&locale=auto&line_items[0][price_data][product_data][images][]=gE5K8MOzRc&payment_method_options[us_bank_account][setup_future_usage]=none&payment_intent_data[on_behalf_of]=mpkGzXu3st&custom_fields[0][label][type]=custom&custom_fields[0][optional]=false&line_items[0][price_data][tax_behavior]=inclusive&billing_address_collection=auto&invoice_creation[invoice_data][rendering_options][amount_tax_display]=exclude_tax&shipping_options[0][shipping_rate_data][fixed_amount][currency]=KkRL3jvZMO&payment_method_options[grabpay][setup_future_usage]=none&ui_mode=hosted&payment_intent_data[transfer_data][destination]=LrcNMrJPkO&shipping_options[0][shipping_rate_data][tax_code]=NKSQxYdCfO&payment_method_options[affirm][setup_future_usage]=none&payment_method_options[paypal][setup_future_usage]=none&payment_method_options[acss_debit][mandate_options][custom_mandate_url]=FZwPtJKktL&automatic_tax[liability][account]=gW7D0WhP9C&custom_fields[0][numeric][maximum_length]=678468035&custom_fields[0][text][minimum_length]=1689246767&line_items[0][price_data][recurring][interval_count]=592739346&client_reference_id=ZcJeCf6JAa&line_items[0][price_data][unit_amount]=945322526&line_items[0][adjustable_quantity][maximum]=1665059759&discounts[0][coupon]=tOlEXiZKv9&shipping_address_collection[allowed_countries][]=AC&payment_method_options[paypal][risk_correlation_id]=fj1J6Nux6P&payment_method_options[acss_debit][setup_future_usage]=off_session&payment_method_options[konbini][setup_future_usage]=none&payment_intent_data[statement_descriptor_suffix]=dtPJwyuc4i&payment_intent_data[setup_future_usage]=off_session&subscription_data[on_behalf_of]=oGsMnSifXV&allow_promotion_codes=true&custom_fields[0][key]=5ZeyjIHLn8&custom_text[submit][message]=vGcSz5eSlo&setup_intent_data[on_behalf_of]=165u5Fvodj&discounts[0][promotion_code]=Xknj8juRnm&customer_update[shipping]=auto&shipping_options[0][shipping_rate_data][delivery_estimate][maximum][unit]=week&payment_method_options[oxxo][expires_after_days]=1925345768&payment_intent_data[receipt_email]=LxJLYGjJ4r&subscription_data[trial_settings][end_behavior][missing_payment_method]=create_invoice&after_expiration[recovery][enabled]=true&payment_method_configuration=uwYSwIZP4V&invoice_creation[invoice_data][account_tax_ids][]=dev8vFF6xG&shipping_options[0][shipping_rate_data][fixed_amount][amount]=2040036333&payment_method_options[paypal][capture_method]=manual&payment_method_options[paypal][preferred_locale]=cs-CZ&payment_intent_data[shipping][address][country]=O8MBVcia7c&after_expiration[recovery][allow_promotion_codes]=true&custom_text[shipping_address][message]=XeD5TkmC8k&line_items[0][price_data][recurring][interval]=day&line_items[0][price_data][product]=xilQ2QDVdA&line_items[0][dynamic_tax_rates][]=jMMvH8TmQD&payment_method_options[card][setup_future_usage]=on_session&payment_method_options[customer_balance][bank_transfer][type]=gb_bank_transfer&payment_method_options[sepa_debit][setup_future_usage]=none&automatic_tax[enabled]=false&consent_collection[terms_of_service]=required&payment_method_options[fpx][setup_future_usage]=none&payment_method_options[us_bank_account][financial_connections][prefetch][]=transactions&payment_intent_data[transfer_data][amount]=94957585&payment_method_options[bancontact][setup_future_usage]=none&payment_intent_data[statement_descriptor]=JCOo6lU8Fy&line_items[0][tax_rates][]=Ts1bPAoT0T&line_items[0][price]=fR6vnvprv8&setup_intent_data[description]=U9qFTQnt1W&redirect_on_completion=never&shipping_options[0][shipping_rate_data][display_name]=PXozGQQnBA&payment_method_options[card][installments][enabled]=true&payment_method_options[p24][tos_shown_and_accepted]=true&payment_method_options[wechat_pay][app_id]=9Pu0d1pZ2r&payment_method_options[wechat_pay][client]=ios&payment_method_options[boleto][expires_after_days]=953467886&payment_method_options[eps][setup_future_usage]=none&payment_method_options[acss_debit][mandate_options][default_for][]=invoice&subscription_data[trial_end]=606476058&custom_fields[0][numeric][minimum_length]=2134997439&line_items[0][price_data][product_data][description]=DQECtJEsLI&consent_collection[promotions]=auto&payment_method_options[swish][reference]=rXJq1EX4rc&payment_intent_data[shipping][carrier]=P8mCJlEq1J&payment_intent_data[shipping][tracking_number]=XGOZIrLZf0&payment_method_options[paynow][setup_future_usage]=none&payment_method_options[revolut_pay][setup_future_usage]=off_session&payment_method_options[klarna][setup_future_usage]=none&payment_intent_data[shipping][address][postal_code]=1aAilmcYiq&subscription_data[invoice_settings][issuer][account]=axhiYamJKY&subscription_data[trial_period_days]=1684102049&subscription_data[description]=7mpaD2E0jf&cancel_url=qpmWppPyIv&payment_method_options[card][statement_descriptor_suffix_kana]=ZvJtIONyDK&payment_method_options[pix][expires_after_seconds]=191312234&custom_fields[0][dropdown][options][0][value]=hXN8MppU0k&tax_id_collection[enabled]=true&payment_method_options[sofort][setup_future_usage]=none&payment_method_options[customer_balance][setup_future_usage]=none&payment_method_options[ideal][setup_future_usage]=none&payment_intent_data[description]=yoalRHw9ZG&payment_intent_data[shipping][phone]=CWAbvZM4Kw&expires_at=1756067225&line_items[0][adjustable_quantity][enabled]=false&invoice_creation[invoice_data][description]=MiePp9LfkQ&payment_method_options[card][request_three_d_secure]=any&payment_method_options[customer_balance][bank_transfer][requested_address_types][]=iban&line_items[0][price_data][unit_amount_decimal]=vkJPCvrn9Q&phone_number_collection[enabled]=true&payment_intent_data[shipping][address][line2]=CM9x9Jizzu&subscription_data[billing_cycle_anchor]=1981798554&subscription_data[application_fee_percent]=1.7020678102144877",
		},
		{
			Name: "PostCheckoutSessions",
			RawArguments: `{
				"body": {
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
								"type": "self"
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
				}
			}`,
			Expected: "automatic_tax[enabled]=false&automatic_tax[liability][type]=self&subscription_data[description]=nyxWwjZ0JY&subscription_data[invoice_settings][issuer][type]=self&subscription_data[trial_period_days]=27623&subscription_data[trial_settings][end_behavior][missing_payment_method]=cancel",
		},
	}

	ndcSchema := createMockSchema(t)
	parseQueryAndSort := func(input string) []string {
		items := strings.Split(input, "&")
		slices.Sort(items)

		return items
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			var info *rest.OperationInfo
			for key, f := range ndcSchema.Procedures {
				if key == tc.Name {
					info = &f
					break
				}
			}
			assert.Assert(t, info != nil)
			var arguments map[string]any
			assert.NilError(t, json.Unmarshal([]byte(tc.RawArguments), &arguments))
			argumentInfo := info.Arguments["body"]
			builder := NewURLParameterEncoder(ndcSchema, rest.ContentTypeFormURLEncoded)
			buf, _, err := builder.Encode(&argumentInfo, arguments["body"])
			assert.NilError(t, err)
			result, err := io.ReadAll(buf)
			assert.NilError(t, err)
			assert.DeepEqual(t, parseQueryAndSort(tc.Expected), parseQueryAndSort(string(result)))
		})
	}
}
