package modbusreg

import "strings"

// TeslaGen3HSCVersionDisposition is the evidence state of a Gen3 version.
type TeslaGen3HSCVersionDisposition string

const (
	TeslaGen3HSCVersionKnownObservation    TeslaGen3HSCVersionDisposition = "known_observation"
	TeslaGen3HSCVersionCompatibleCandidate TeslaGen3HSCVersionDisposition = "compatible_candidate"
	TeslaGen3HSCVersionUnknown             TeslaGen3HSCVersionDisposition = "unknown"
)

// TeslaGen3HSCProfileConfig declares independently evaluated Gen3 predicates.
type TeslaGen3HSCProfileConfig struct {
	Enabled                bool
	Version                string
	ActivationCapable      bool
	PrivateFunctionCapable bool
	OperationCapable       bool
}

// TeslaGen3HSCProfile never derives a capability from a version label alone.
type TeslaGen3HSCProfile struct {
	config      TeslaGen3HSCProfileConfig
	disposition TeslaGen3HSCVersionDisposition
}

// NewTeslaGen3HSCProfile creates a distinct Gen3 capability profile.
func NewTeslaGen3HSCProfile(config TeslaGen3HSCProfileConfig) (TeslaGen3HSCProfile, error) {
	disposition := TeslaGen3HSCVersionUnknown
	if strings.TrimSpace(config.Version) != "" {
		switch config.Version {
		case "24.28.3", "24.44.3":
			disposition = TeslaGen3HSCVersionKnownObservation
		default:
			disposition = TeslaGen3HSCVersionCompatibleCandidate
		}
	}
	return TeslaGen3HSCProfile{config: config, disposition: disposition}, nil
}

// VersionDisposition returns the separate version-evidence state.
func (profile TeslaGen3HSCProfile) VersionDisposition() TeslaGen3HSCVersionDisposition {
	return profile.disposition
}

// ExchangeEligible requires every independently declared Gen3 predicate.
func (profile TeslaGen3HSCProfile) ExchangeEligible() bool {
	return profile.config.Enabled && profile.disposition != TeslaGen3HSCVersionUnknown &&
		profile.config.ActivationCapable && profile.config.PrivateFunctionCapable &&
		profile.config.OperationCapable
}
