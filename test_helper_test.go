package nodejs_test

import (
	"fmt"
	"testing"
)

// logIfVerbose outputs messages only if verbose testing is enabled
func logIfVerbose(t *testing.T, format string, args ...interface{}) {
	if testing.Verbose() {
		t.Logf(format, args...)
	}
}

// testErrorContains checks if an error contains the expected substring
func testErrorContains(t *testing.T, err error, expectedError string) {
	if err == nil {
		t.Fatalf("expected error containing '%s' but got nil", expectedError)
	}

	if !containsString(err.Error(), expectedError) {
		t.Errorf("expected error containing '%s' but got: %s", expectedError, err)
	}
}

// containsString checks if a string contains a substring
func containsString(s, substr string) bool {
	return s != "" && substr != "" && len(s) >= len(substr) && s[0:len(substr)] == substr
}

// formatError formats an error for test output
func formatError(err error) string {
	if err == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%v", err)
}
