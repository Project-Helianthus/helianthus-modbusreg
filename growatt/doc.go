// Package growatt owns the Growatt family profiles of the registry: the
// Protocol II identity decoder and its RTU observer, the Protocol II FC04
// telemetry decoder and observer behind their applicability gate, the FC04
// proposed-record validation, the BMS RS-485 read-only, typed and
// multi-group diagnostic decoders with their RTU observer, the aggregate
// cycle retention, provider port and offline provider, and the diagnostic
// commit check.
//
// The package depends only on the contracts of the module root package and
// imports no other vendor package.
package growatt
