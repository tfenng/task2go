package core

import (
	"bytes"
	"encoding/json"
)

func jsonUnmarshalNumber(data []byte, target any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	return dec.Decode(target)
}
