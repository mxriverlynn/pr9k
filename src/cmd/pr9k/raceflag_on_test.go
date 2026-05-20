//go:build race

package main

// raceDetectorEnabled is true when the test binary is built with -race.
// Used to skip subprocess-timing acceptance tests that are environment-flaky
// under the race detector's 10–100× slowdown combined with full-suite
// package-level contention (their behaviour is unit-tested elsewhere).
const raceDetectorEnabled = true
