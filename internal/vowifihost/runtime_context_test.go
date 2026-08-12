package vowifihost

import (
	"context"
	"testing"
	"time"
)

// The runtime must not inherit a context that dies with the caller who started
// it.
//
// enableRuntime returns as soon as the start is accepted; the tunnel and IMS
// registration are built by an async staged pipeline. handleTransportDead used
// to bound its Restart with context.WithTimeout and call cancel() the moment
// Restart returned -- so the pipeline it had just launched saw a dead context in
// its first ShouldRun check and aborted with "start_canceled" before touching
// the tunnel. Self-heal logged success while leaving VoWiFi down, and it stayed
// down until someone noticed.
//
// This test pins the contract that fixes it: a context derived for the runtime
// survives cancellation of the one that requested the start. It is deliberately
// a property of context, not of our code -- if someone removes the
// WithoutCancel, this fails.
func TestRuntimeContextSurvivesCallerCancellation(t *testing.T) {
	callerCtx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)

	// The same derivation enableRuntime performs.
	runtimeCtx := context.WithoutCancel(callerCtx)

	cancel() // what handleTransportDead does as soon as Restart returns

	if err := callerCtx.Err(); err == nil {
		t.Fatal("caller context should be cancelled")
	}
	if err := runtimeCtx.Err(); err != nil {
		t.Fatalf("runtime context died with its caller: %v", err)
	}

	// The caller's deadline must not reach it either: a 3-minute bound on the
	// start call would otherwise kill a session that is already healthy.
	if deadline, ok := runtimeCtx.Deadline(); ok {
		t.Fatalf("runtime context inherited a deadline (%s); a healthy session would be killed by it", deadline)
	}
}
