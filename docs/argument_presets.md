# Argument Presets

## Introduction

You can use argument presets to set default values to request arguments. Argument presets can be configured globally in the `settings` object or individual servers:

```json
{
  "settings": {
    "servers": [
      {
        "url": {
          "env": "PET_STORE_URL"
        },
        "argumentPresets": [
          {
            "path": "body.name",
            "value": {
              "type": "env",
              "name": "PET_NAME"
            },
            "targets": ["addPet"]
          }
        ]
      }
    ],
    "argumentPresets": [
      {
        "path": "status",
        "value": {
          "type": "forwardHeader",
          "name": "X-Pet-Status"
        },
        "targets": ["findPetsByStatus"]
      },
      {
        "path": "body.id",
        "value": {
          "type": "literal",
          "value": 1
        },
        "targets": ["addPet"]
      }
    ]
  }
}
```

The target argument field is removed from the `arguments` schema if the selector is the root field. If the path selects the nested field the target field becomes nullable.

## Configuration options

- `path`: The JSON path to the argument field.
- `targets`: List of function or procedure patterns in regular expressions.
- `value`: The value of argument preset. Supports 3 value types:
  - `literal`: Literal value.
  - `env`: Environment variable.
  - `forwardHeader`: Value forwarded from request headers. Require enabling [Header Forwarding](./authentication.md#headers-forwarding).
