package tesla

import (
	"fmt"
	"strings"
)

const maxTeslaGen3HSCVersionEvidence = 252

// Gen3HSCVersionDisposition is the evidence state of a Gen3 version.
type Gen3HSCVersionDisposition string

// Version dispositions. KnownObservation: the trimmed version is one of the
// enumerated observed labels. CompatibleCandidate: another nonempty label with
// all three capability predicates declared; it is a candidate, not a
// qualification. Unknown: anything else, including an empty label.
const (
	Gen3HSCVersionKnownObservation    Gen3HSCVersionDisposition = "known_observation"
	Gen3HSCVersionCompatibleCandidate Gen3HSCVersionDisposition = "compatible_candidate"
	Gen3HSCVersionUnknown             Gen3HSCVersionDisposition = "unknown"
)

// Gen3HSCProfileConfig declares independently evaluated Gen3 predicates.
type Gen3HSCProfileConfig struct {
	Enabled bool
	Version string
	// VersionEvidence is the bounded native version payload associated with Version.
	// It is retained byte-for-byte; callers may use an empty value when none exists.
	VersionEvidence        []byte
	ActivationCapable      bool
	PrivateFunctionCapable bool
	OperationCapable       bool
}

// Gen3HSCProfile never derives a capability from a version label alone.
type Gen3HSCProfile struct {
	config      Gen3HSCProfileConfig
	disposition Gen3HSCVersionDisposition
}

// NewGen3HSCProfile creates a distinct Gen3 capability profile.
func NewGen3HSCProfile(config Gen3HSCProfileConfig) (Gen3HSCProfile, error) {
	if len(config.VersionEvidence) > maxTeslaGen3HSCVersionEvidence {
		return Gen3HSCProfile{}, fmt.Errorf("tesla Gen3 HSC version evidence exceeds %d bytes", maxTeslaGen3HSCVersionEvidence)
	}
	config.VersionEvidence = append([]byte(nil), config.VersionEvidence...)
	disposition := Gen3HSCVersionUnknown
	version := strings.TrimSpace(config.Version)
	if version != "" {
		switch version {
		case "24.28.3", "24.44.3", "wc3_24_28_3", "wc3_24_44_3":
			disposition = Gen3HSCVersionKnownObservation
		default:
			if config.ActivationCapable && config.PrivateFunctionCapable && config.OperationCapable {
				disposition = Gen3HSCVersionCompatibleCandidate
			}
		}
	}
	return Gen3HSCProfile{config: config, disposition: disposition}, nil
}

// Version returns the supplied version label without discarding native context.
func (profile Gen3HSCProfile) Version() string { return profile.config.Version }

// VersionEvidence returns a defensive copy of the bounded native version payload.
func (profile Gen3HSCProfile) VersionEvidence() []byte {
	return append([]byte(nil), profile.config.VersionEvidence...)
}

// VersionDisposition returns the separate version-evidence state.
func (profile Gen3HSCProfile) VersionDisposition() Gen3HSCVersionDisposition {
	return profile.disposition
}

// ExchangeEligible requires every independently declared Gen3 predicate.
func (profile Gen3HSCProfile) ExchangeEligible() bool {
	return profile.config.Enabled && profile.disposition != Gen3HSCVersionUnknown &&
		profile.config.ActivationCapable && profile.config.PrivateFunctionCapable &&
		profile.config.OperationCapable
}
