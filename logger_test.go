package main

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLogger(t *testing.T) {
	var buf1 bytes.Buffer
	writer := io.MultiWriter(&buf1)
	testLogger := createLogger(writer, "info")

	testLogger.Info("test")
	testLogger.Warn("test")

	assert.Regexp(t, "INFO .* test\nWARNING .* test", buf1.String())
}
