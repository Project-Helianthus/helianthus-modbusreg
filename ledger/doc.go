// Package ledger owns the completion and chronology records of the
// registry: the bounded sample ledger with its attempt phases, claims,
// cancellation, audit tombstones, restart state and snapshot, the sample
// ledger state and its serialization, the observation factory that binds
// production attempts to one profile, ledger and transactional publication
// committer, and the immutable completion record of an evidence-qualified
// profile disposition with its serialization.
//
// The package depends only on the contracts of the module root package and
// imports no detection or vendor package.
package ledger
