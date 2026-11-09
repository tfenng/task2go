package store

import (
	"bytes"
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

// JSONB keeps numbers as json.Number so values round-trip without float precision loss.
type JSONB map[string]any

func (j *JSONB) Scan(src any) error {
	if src == nil {
		*j = nil
		return nil
	}
	var data []byte
	switch v := src.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		return fmt.Errorf("cannot scan %T into JSONB", src)
	}
	if len(data) == 0 {
		*j = JSONB{}
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var out map[string]any
	if err := dec.Decode(&out); err != nil {
		return err
	}
	*j = JSONB(out)
	return nil
}

func (j JSONB) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return json.Marshal(map[string]any(j))
}

func (j JSONB) Map() map[string]any {
	if j == nil {
		return map[string]any{}
	}
	return map[string]any(j)
}
