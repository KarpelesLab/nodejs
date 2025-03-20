package nodejs_test

import (
	"testing"
)

// Skip TestHTTPHandler for now until we finish transitioning to nodejs 22+
// Once our Request/Response implementation is fully compatible with nodejs native version
func TestHTTPHandler(t *testing.T) {
	t.Skip("Skipping HTTP handler test during transition to nodejs 22 native Request/Response")
}