// Package sunspecfixture holds the synthetic SunSpec chain fixtures shared by
// the tests of the SunSpec overlay packages: a standard decoder registry,
// catalog-placed model words, admitted occurrences and terminal chain
// snapshots produced by walking a planned chain over synthetic logical views,
// without a socket, serial port or device.
//
// Every fixture is built through the exported SunSpec API of the module root
// package, the only package of this module it imports; the SunSpec tests of
// the root package keep their own in-package helpers. Only test files import
// this package.
package sunspecfixture
