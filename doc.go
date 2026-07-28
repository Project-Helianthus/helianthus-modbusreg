// Package modbusreg is the public multi-vendor Modbus profile registry for
// Helianthus.
//
// The M2-01 surface defines immutable profile, codec, documentary address,
// dependency-set, source-observation, logical-view provenance, replay, and
// coherence contracts, plus deterministic serialization and mandatory sample
// admission through explicit restart state. It consumes successful logical views from
// helianthus-modbus but owns no framing, sockets, serial ports, scheduling,
// retries, detector execution, qualification, canonical semantics, gateway
// composition, or private bindings.
package modbusreg
