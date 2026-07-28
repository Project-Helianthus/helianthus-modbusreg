// Package modbusreg is the public multi-vendor Modbus profile registry for
// Helianthus.
//
// The M2-01 surface defines immutable profile, codec, documentary address,
// dependency-set, source-observation, logical-view provenance, replay, and
// coherence contracts, plus bounded deterministic serialization, evidence-backed
// overlay deltas, and factory-issued sample admission through explicit O(1)
// revision/high-water state. It consumes successful logical views from
// helianthus-modbus but owns no framing, sockets, serial ports, scheduling,
// retry execution, durable storage, detector execution, qualification,
// canonical semantics, gateway composition, or private bindings.
package modbusreg
