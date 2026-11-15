// Package outback owns the OutBack overlay of the SunSpec standard family:
// the read-only flavor decision and decoder of the OutBack SunSpec interface
// models, which reject every source until a source qualifies them, the
// detached record of supplied diagnostic words with their present and missing
// spans, and the offline provider cycle with its provider port.
//
// The package reads SunSpec chains and occurrences only through the exported
// SunSpec surface of the module root package, depends on no other module
// package and imports no other vendor package.
package outback
