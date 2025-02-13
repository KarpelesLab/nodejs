package nodejs_test

import (
	"context"
	"testing"

	"github.com/KarpelesLab/nodejs"
)

func TestFactory(t *testing.T) {
	f, err := nodejs.New()
	if err != nil {
		t.Errorf("failed to get factory: %s", err)
		return
	}

	proc, err := f.New()
	if err != nil {
		t.Errorf("failed to get process: %s", err)
		return
	}
	defer proc.Close()

	v, err := proc.Eval(context.Background(), "\"This Is a STRING\".toLowerCase()", map[string]any{"filename": "test.js"})
	if err != nil {
		t.Errorf("failed to run test: %s", err)
		return
	}
	if v != "this is a string" {
		t.Errorf("result is not good: %s", v)
	}
}
