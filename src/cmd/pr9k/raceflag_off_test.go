//go:build !race

package main

// raceDetectorEnabled is false in non-race builds; see raceflag_on_test.go.
const raceDetectorEnabled = false
