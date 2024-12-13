package internal

import (
	"testing"

	"gotest.tools/v3/assert"
)

func TestEvalAcceptContentType(t *testing.T) {
	assert.Equal(t, "image/*", evalAcceptContentType("image/jpeg"))
	assert.Equal(t, "video/*", evalAcceptContentType("video/mp4"))
	assert.Equal(t, "application/json", evalAcceptContentType("application/json"))
}
