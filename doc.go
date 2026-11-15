// Package modbusreg is the public multi-vendor Modbus profile registry for
// Helianthus.
//
// The module root package owns the shared contracts every other package of
// the module builds on: immutable profile, codec, documentary address,
// dependency-set, source-observation, logical-view provenance, replay, and
// coherence contracts, versions, limits, the bounded JSON preflight and
// serialization rules, and the qualified-function registry. It imports no
// other package of the module and re-exports none of their names.
//
// Profile detection and offline fixture replay live in package detection;
// the bounded attempt ledger, which consumes exact opaque M1-06 runtime
// acquisitions through a caller-supplied transactional committer, and the
// completion records live in package ledger; each vendor family lives in its
// own package, and the vendor overlays of the SunSpec standard family import
// package sunspec. Offline fixture replay is explicitly nonpublishable. The
// module owns no framing, sockets, serial ports, scheduling, retry
// execution, durable storage, canonical semantics, gateway composition, or
// private bindings.
package modbusreg
