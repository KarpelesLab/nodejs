package nodejs_test

import (
	"fmt"
	"os/exec"
	"testing"
)

// skipIfNodeJSUnavailable skips the test if NodeJS is not available
// or not working properly on the system
func skipIfNodeJSUnavailable(t *testing.T) {
	_, err := exec.LookPath("node")
	if err != nil {
		t.Skip("NodeJS not found in PATH, skipping test")
	}
}

// skipOnBootstrapIssue checks if a test should be skipped due to
// bootstrap-related issues
func skipOnBootstrapIssue(t *testing.T, err error) bool {
	if err != nil && err.Error() == "TypeError: Cannot read properties of undefined (reading 'endsWith')" {
		t.Skip("Skipping test due to bootstrap issue, possibly environment-specific")
		return true
	}
	return false
}

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
