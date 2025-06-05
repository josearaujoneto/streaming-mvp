package log

import (
	"bytes"
	"io"
	"os"
	"testing"
)

func TestLoggerOutput(t *testing.T) {

	r, w, _ := os.Pipe()
	stdout := os.Stdout
	os.Stdout = w

	log := NewLogger(false)
	log.Error("Testing logger output")

	w.Close()
	os.Stdout = stdout

	var buf bytes.Buffer
	io.Copy(&buf, r)

	if !containsLogLevel(buf.String(), "ERROR") {
		t.Errorf("Expected log level ERROR, but got: %s", buf.String())
	}
}

func containsLogLevel(output, level string) bool {
	return bytes.Contains([]byte(output), []byte(level))
}
