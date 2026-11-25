package tesla

import (
	"strconv"
	"strings"
)

// ReadApplicabilityGrade describes read applicability, not operation qualification.
type ReadApplicabilityGrade string

// Read grades distinguish the baseline from compatible build hypotheses.
const (
	ReadGradeProven     ReadApplicabilityGrade = "Proven"
	ReadGradeHypothesis ReadApplicabilityGrade = "Hypothesis"
)

func teslaReadFirmware(label string) ([3]uint64, bool) {
	var version [3]uint64
	label = strings.TrimSpace(label)
	sep := "."
	if strings.HasPrefix(label, "wc3_") {
		label, sep = strings.TrimPrefix(label, "wc3_"), "_"
	}
	parts := strings.Split(label, sep)
	if len(parts) != len(version) {
		return version, false
	}
	for i, part := range parts {
		v, err := strconv.ParseUint(part, 10, 64)
		if err != nil {
			return version, false
		}
		version[i] = v
	}
	return version, true
}

var teslaReadFloor = [3]uint64{24, 44, 3}

func (profile Gen3HSCProfile) readApplicability() (ReadApplicabilityGrade, string) {
	if strings.TrimSpace(profile.Version()) == string(LegacyWallConnectorGenerationCandidate) {
		return "", "legacy_profile_required"
	}
	version, ok := teslaReadFirmware(profile.Version())
	// MID belongs to a separate firmware line (RS-485 Site Controller Protocol §1.5).
	if !ok && strings.TrimSpace(profile.Version()) != "MID 25.2.2" {
		return "", "version_unidentified"
	}
	if ok {
		for i, v := range version {
			if v < teslaReadFloor[i] {
				return "", "below_floor"
			}
			if v > teslaReadFloor[i] {
				break
			}
		}
	}
	if !profile.ExchangeEligible() {
		return "", "capability_incompatible"
	}
	if version == teslaReadFloor {
		return ReadGradeProven, ""
	}
	return ReadGradeHypothesis, ""
}

func (profile Gen3HSCProfile) documentedCurrentLimitLayout() bool {
	version, ok := teslaReadFirmware(profile.Version())
	return ok && (version == [3]uint64{26, 34, 0} || version == [3]uint64{26, 42, 0})
}
