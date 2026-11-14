// Package qualifiedfunctiontest holds the test support shared by the tests of
// the qualified-function registry in the module root package and the tests of
// the vendor packages that dispatch through it: an in-memory exchange that
// answers each private-function request with RTU response ADUs built from
// configured payloads, without a socket, serial port or device.
//
// Only test files import this package; it imports no package of this module.
package qualifiedfunctiontest
