package main

import "time"

// Background monitors (wake automation, bass offset, center level, Front EQ)
// share the action gate with HTTP controls. They start together and tick
// every one or two seconds, so their ticks fire at the same instant. With a
// bare TryLock the first goroutine to run won every shared tick and the
// others skipped theirs, possibly forever: on the bar the Front EQ monitor
// stayed idle after the firmware dropped the EQ at wake, leaving
// "reset required" with Standard Bluetooth playing.
//
// retryGate retries briefly instead of skipping the tick. A monitor holds the
// gate for milliseconds, so the next one gets it on its first retry; a
// running HTTP control still makes the monitor skip this tick.
var (
	gateRetryDelay = 50 * time.Millisecond
	gateRetries    = 8
)

func retryGate(try func() bool) bool {
	for i := 0; ; i++ {
		if try() {
			return true
		}
		if i == gateRetries {
			return false
		}
		time.Sleep(gateRetryDelay)
	}
}
