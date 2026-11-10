package productinfo

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

//go:embed protocol.schema.json
var manifestSchema []byte
var schemaOnce sync.Once
var compiledSchema *jsonschema.Schema
var schemaError error

func validateSchema(raw []byte) error {
	schemaOnce.Do(func() {
		c := jsonschema.NewCompiler()
		c.AssertFormat = true
		c.LoadURL = func(string) (io.ReadCloser, error) { return nil, fmt.Errorf("external schema disabled") }
		const id = "urn:powerx:deployment-fingerprint:v1"
		schemaError = c.AddResource(id, bytes.NewReader(manifestSchema))
		if schemaError == nil {
			compiledSchema, schemaError = c.Compile(id + "#/$defs/Manifest")
		}
	})
	if schemaError != nil {
		return schemaError
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return err
	}
	return compiledSchema.Validate(v)
}
