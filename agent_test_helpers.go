//go:build test || integration

package agent

import "sync"

// Reset resets the global agent state for testing purposes.
// This function is only available when building with the 'test' or 'integration' build tag.
//
// WARNING: This function is NOT safe for concurrent use. Call it only from a single
// goroutine (e.g., via t.Cleanup or defer), after all goroutines spawned by the
// previous test have exited.
//
// Note: This does NOT call Shutdown(). If you need to flush telemetry data
// before resetting, call Shutdown() first.
func Reset() {
	globalAgent.Store(nil)
	once = sync.Once{}
}
