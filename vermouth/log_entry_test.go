package vermouth

import (
	"bytes"
	"encoding/gob"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDecode(t *testing.T) {
	body := addHttpProxyBody{
		Port: 7181,
		PrefixType: "hashPrefix",
	}

	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(&body)
	assert.NoError(t, err)

	data := buf.Bytes()

	copyBody := &addHttpProxyBody{}
	err = decode(data, copyBody)
	assert.NoError(t, err)

	assert.Equal(t, int64(7181), copyBody.Port)
	assert.Equal(t, "hashPrefix", copyBody.PrefixType)
}