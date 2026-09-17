// Package osc implements the OSC value and packet subset used by VRCFT-Go and
// its plugins. It supports int32, float32, string, boolean, messages, and
// bundles. Decoding preserves message order but flattens nested bundles and
// does not schedule their timetags.
package osc
