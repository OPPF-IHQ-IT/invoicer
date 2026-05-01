package airtable

import "bytes"

func jsonReader(b []byte) *bytes.Reader {
	return bytes.NewReader(b)
}
