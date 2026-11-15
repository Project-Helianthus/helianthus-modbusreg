// Package fronius owns the Fronius overlay of the SunSpec standard family:
// the observed SunSpec flavor definitions, their evaluation and exact
// selection over a terminal chain snapshot, the documentary qualification card
// and the qualification observation it derives, the retained commit of
// qualified observations, and the offline provider cycle with its provider
// port.
//
// The package reads SunSpec chains, decoder registries and capability
// decisions only through the exported SunSpec surface of the module root
// package, depends on no other module package and imports no other vendor
// package.
package fronius
