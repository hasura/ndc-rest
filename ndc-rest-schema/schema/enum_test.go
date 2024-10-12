package schema

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestSchemaSpecType(t *testing.T) {
	rawValue := "oas2"
	var got SchemaSpecType
	if err := json.Unmarshal([]byte(fmt.Sprintf(`"%s"`, rawValue)), &got); err != nil {
		t.Fatal(err.Error())
	}
	if got != SchemaSpecType(rawValue) {
		t.Fatalf("expected %s, got: %s", rawValue, got)
	}
	if got.JSONSchema().Type != "string" {
		t.Fatalf("expected string, got: %s", got.JSONSchema().Type)
	}
}

func TestRequestType(t *testing.T) {
	rawValue := "rest"
	var got RequestType
	if err := json.Unmarshal([]byte(fmt.Sprintf(`"%s"`, rawValue)), &got); err != nil {
		t.Fatal(err.Error())
	}
	if got != RequestType(rawValue) {
		t.Fatalf("expected %s, got: %s", rawValue, got)
	}
}

func TestSchemaFileFormat(t *testing.T) {
	rawValue := "yaml"
	var got SchemaFileFormat
	if err := json.Unmarshal([]byte(fmt.Sprintf(`"%s"`, rawValue)), &got); err != nil {
		t.Fatal(err.Error())
	}
	if got != SchemaFileFormat(rawValue) {
		t.Fatalf("expected %s, got: %s", rawValue, got)
	}
	if got.JSONSchema().Type != "string" {
		t.Fatalf("expected string, got: %s", got.JSONSchema().Type)
	}
}

func TestParameterLocation(t *testing.T) {
	rawValue := "cookie"
	var got ParameterLocation
	if err := json.Unmarshal([]byte(fmt.Sprintf(`"%s"`, rawValue)), &got); err != nil {
		t.Fatal(err.Error())
	}
	if got != ParameterLocation(rawValue) {
		t.Fatalf("expected %s, got: %s", rawValue, got)
	}
	if got.JSONSchema().Type != "string" {
		t.Fatalf("expected string, got: %s", got.JSONSchema().Type)
	}
}

func TestParameterEncodingStyle(t *testing.T) {
	rawValue := "matrix"
	var got ParameterEncodingStyle
	if err := json.Unmarshal([]byte(fmt.Sprintf(`"%s"`, rawValue)), &got); err != nil {
		t.Fatal(err.Error())
	}
	if got != ParameterEncodingStyle(rawValue) {
		t.Fatalf("expected %s, got: %s", rawValue, got)
	}
	if got.JSONSchema().Type != "string" {
		t.Fatalf("expected string, got: %s", got.JSONSchema().Type)
	}
}
