//go:build test || integration

package agent

import "sync"

// Reset resets the global agent state for testing purposes.
// This function is only available when building with the 'test' or 'integration' build tag.
//
// WARNING: This function is NOT safe for production use. It's intended only for
// unit/integration tests where you need to reset agent state between test cases.
//
// Example usage in tests:
//
//	func TestSomething(t *testing.T) {
//	    defer agent.Reset() // Clean up after test
//
//	    err := agent.Start()
//	    require.NoError(t, err)
//
//	    // Your test code here
//	}
//
// Note: This does NOT call Shutdown(). If you need to flush telemetry data
// before resetting, call Shutdown() first.
func Reset() {
	globalAgent = nil
	once = sync.Once{}
}
