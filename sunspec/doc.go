// Package sunspec owns the vendor-neutral SunSpec standard family of the
// registry: the bounded read-only phase-one profile, decoder and observation,
// the chain plan and the replay-only chain walk with its terminal snapshot,
// the model catalog with the common, inverter, meter, MPPT, controls,
// environmental, extended and DER models and their nested layouts, the
// decoded values and facts, the structural projection, the vendor model
// namespace and the three-phase monitoring capability decision.
//
// The package depends only on the contracts of the module root package and
// imports no vendor package; the vendor overlays of the family import it.
package sunspec
