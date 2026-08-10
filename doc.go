// Package modbusreg is the public multi-vendor Modbus profile registry for
// Helianthus.
//
// The M2-01 surface defines immutable profile, codec, documentary address,
// dependency-set, source-observation, logical-view provenance, replay, and
// coherence contracts. Production ingestion consumes exact opaque M1-06 runtime
// acquisitions through a bounded attempt ledger and caller-supplied transactional
// committer. Offline fixture replay is explicitly nonpublishable. The package
// owns no framing, sockets, serial ports, scheduling, retry execution, durable
// storage, detector execution, qualification, canonical semantics, gateway
// composition, or private bindings.
package modbusreg
