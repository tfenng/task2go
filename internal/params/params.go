package params

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/tfenng/tasks/task2go/internal/security"
)

const (
	MaxParameterBytes = 256 * 1024
	MaxDepth          = 8
)

var supportedTypes = map[string]bool{
	"string": true, "integer": true, "number": true, "boolean": true,
	"object": true, "array": true, "null": true,
}

var supportedFormats = map[string]bool{
	"date-time": true, "password": true, "secret": true,
}

type ValidationError struct {
	Errors []FieldError
}

type FieldError struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *ValidationError) Error() string { return "invalid parameters" }

func validateSchemaShape(doc any) error {
	if err := walkShape(doc, 0); err != nil {
		return err
	}
	encoded, err := json.Marshal(doc)
	if err != nil {
		return &ValidationError{Errors: []FieldError{{Field: "$", Code: "type", Message: "schema must be object"}}}
	}
	if len(encoded) > MaxParameterBytes {
		return &ValidationError{Errors: []FieldError{{Field: "$", Code: "too_large", Message: "schema too large"}}}
	}
	return nil
}

func walkShape(value any, depth int) error {
	if depth > MaxDepth {
		return &ValidationError{Errors: []FieldError{{Field: "$", Code: "max_depth", Message: "schema too deep"}}}
	}
	switch v := value.(type) {
	case map[string]any:
		if ref, ok := v["$ref"]; ok {
			s, _ := ref.(string)
			if !strings.HasPrefix(s, "#") {
				return &ValidationError{Errors: []FieldError{{Field: "$ref", Code: "remote_ref", Message: "remote $ref is not allowed"}}}
			}
		}
		if t, ok := v["type"]; ok {
			switch tv := t.(type) {
			case string:
				if !supportedTypes[tv] {
					return &ValidationError{Errors: []FieldError{{Field: "type", Code: "unsupported_type", Message: "unsupported type " + tv}}}
				}
			case []any:
				for _, item := range tv {
					name, _ := item.(string)
					if !supportedTypes[name] {
						return &ValidationError{Errors: []FieldError{{Field: "type", Code: "unsupported_type", Message: "unsupported type " + name}}}
					}
				}
			}
		}
		if f, ok := v["format"].(string); ok && f != "" && !supportedFormats[f] {
			return &ValidationError{Errors: []FieldError{{Field: "format", Code: "unsupported_format", Message: "unsupported format " + f}}}
		}
		for _, item := range v {
			if err := walkShape(item, depth+1); err != nil {
				return err
			}
		}
	case []any:
		for _, item := range v {
			if err := walkShape(item, depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}

func compileSchema(schema map[string]any) (*jsonschema.Schema, error) {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(mustMarshal(schema)))
	if err != nil {
		return nil, err
	}
	if err := validateSchemaShape(doc); err != nil {
		return nil, err
	}
	compiler := jsonschema.NewCompiler()
	compiler.UseLoader(rejectLoader{})
	if err := compiler.AddResource("mem://schema.json", doc); err != nil {
		return nil, err
	}
	compiled, err := compiler.Compile("mem://schema.json")
	if err != nil {
		return nil, &ValidationError{Errors: []FieldError{{Field: "$", Code: "schema", Message: err.Error()}}}
	}
	return compiled, nil
}

type rejectLoader struct{}

func (rejectLoader) Load(string) (any, error) { return nil, fmt.Errorf("remote $ref is not allowed") }

func ValidateSchema(ctx context.Context, schema map[string]any) error {
	_, err := compileSchema(schema)
	return err
}

func ValidateParameters(ctx context.Context, schema map[string]any, value map[string]any) error {
	compiled, err := compileSchema(schema)
	if err != nil {
		return err
	}
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(mustMarshal(value)))
	if err != nil {
		return &ValidationError{Errors: []FieldError{{Field: "$", Code: "type", Message: "parameters must be an object"}}}
	}
	if err := compiled.Validate(instance); err != nil {
		return mapValidationError(err)
	}
	return nil
}

func mapValidationError(err error) error {
	ve, ok := err.(*jsonschema.ValidationError)
	if !ok {
		return &ValidationError{Errors: []FieldError{{Field: "$", Code: "invalid", Message: err.Error()}}}
	}
	var out []FieldError
	var walk func(*jsonschema.ValidationError)
	walk = func(e *jsonschema.ValidationError) {
		if len(e.Causes) > 0 {
			for _, cause := range e.Causes {
				walk(cause)
			}
			return
		}
		field := strings.Join(e.InstanceLocation, ".")
		if field == "" {
			field = "$"
		}
		code := "invalid"
		if path := e.ErrorKind.KeywordPath(); len(path) > 0 {
			code = path[len(path)-1]
		}
		out = append(out, FieldError{Field: field, Code: code, Message: e.Error()})
	}
	walk(ve)
	if len(out) == 0 {
		out = []FieldError{{Field: "$", Code: "invalid", Message: ve.Error()}}
	}
	return &ValidationError{Errors: out}
}

func ApplyDefaults(schema map[string]any, value map[string]any) map[string]any {
	result := deepCopyMap(value)
	props, _ := schema["properties"].(map[string]any)
	for name, raw := range props {
		def, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if _, exists := result[name]; !exists {
			if defaultValue, has := def["default"]; has {
				result[name] = deepCopyAny(defaultValue)
			}
		}
	}
	return result
}

func Redact(schema map[string]any, value map[string]any) map[string]any {
	redacted := deepCopyMap(value)
	props, _ := schema["properties"].(map[string]any)
	for name, raw := range props {
		def, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if isSecret(def) {
			if _, exists := redacted[name]; exists {
				redacted[name] = "***"
			}
		}
	}
	return redacted
}

func isSecret(def map[string]any) bool {
	secret, _ := def["x-secret"].(bool)
	writeOnly, _ := def["writeOnly"].(bool)
	return secret || writeOnly
}

func EncryptParameters(schema map[string]any, value map[string]any, runID string, box *security.SecretBox) (map[string]any, error) {
	result := deepCopyMap(value)
	props, _ := schema["properties"].(map[string]any)
	for name, raw := range props {
		def, ok := raw.(map[string]any)
		if !ok || !isSecret(def) {
			continue
		}
		if v, exists := result[name]; exists {
			plaintext := string(mustMarshal(v))
			encrypted, err := box.Encrypt(plaintext, fmt.Sprintf("run:%s:param:%s", runID, name))
			if err != nil {
				return nil, err
			}
			result[name] = encrypted
		}
	}
	return result, nil
}

func DecryptParameters(schema map[string]any, value map[string]any, runID string, box *security.SecretBox) (map[string]any, error) {
	result := deepCopyMap(value)
	props, _ := schema["properties"].(map[string]any)
	for name := range props {
		payload, ok := result[name].(map[string]any)
		if !ok {
			continue
		}
		if _, has := payload["__enc__"]; !has {
			continue
		}
		plaintext, err := box.Decrypt(payload, fmt.Sprintf("run:%s:param:%s", runID, name))
		if err != nil {
			return nil, err
		}
		decoded := json.NewDecoder(strings.NewReader(plaintext))
		decoded.UseNumber()
		var v any
		if err := decoded.Decode(&v); err != nil {
			return nil, err
		}
		result[name] = v
	}
	return result, nil
}

func SecretValues(schema map[string]any, value map[string]any) []string {
	var out []string
	props, _ := schema["properties"].(map[string]any)
	for name, raw := range props {
		def, ok := raw.(map[string]any)
		if !ok || !isSecret(def) {
			continue
		}
		if v, exists := value[name]; exists && v != nil {
			out = append(out, fmt.Sprint(v))
		}
	}
	return out
}

func CanonicalJSON(value any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "")
	if err := enc.Encode(canonicalize(value)); err != nil {
		return nil, err
	}
	return bytes.TrimSpace(buf.Bytes()), nil
}

func canonicalize(value any) any {
	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			out[key] = canonicalize(item)
		}
		return out
	case JSONMap:
		return canonicalize(map[string]any(v))
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = canonicalize(item)
		}
		return out
	default:
		return v
	}
}

// JSONMap is accepted to avoid importing store in callers that already have a map.
type JSONMap map[string]any

func mustMarshal(value any) []byte {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(value)
	return bytes.TrimSpace(buf.Bytes())
}

func deepCopyMap(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(value)
	dec := json.NewDecoder(bytes.NewReader(buf.Bytes()))
	dec.UseNumber()
	var out map[string]any
	_ = dec.Decode(&out)
	if out == nil {
		return map[string]any{}
	}
	return out
}

func deepCopyAny(value any) any {
	data := mustMarshal(value)
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var out any
	_ = dec.Decode(&out)
	return out
}

func SumSHA256(data []byte) string {
	return fmt.Sprintf("%x", sha256.Sum256(data))
}
