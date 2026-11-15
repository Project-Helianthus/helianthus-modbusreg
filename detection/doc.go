// Package detection owns profile detection for the registry: the read-only
// probe plan with its read requests and validated read results, the
// immutable profile detector with its candidates, identity and firmware
// gates, options and limits, the detection decision with its outcome,
// reasons and candidate evidence and its bounded JSON serialization, and the
// offline fixture conformance corpus and fixture replay that run detection
// cases without creating a runtime capability or publication.
//
// A detection with more than one eligible candidate is ambiguous; no score,
// order or vendor name selects a winner.
//
// The package depends only on the contracts of the module root package and
// imports no vendor package.
package detection
