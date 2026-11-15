package modbusreg

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

var versionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
var dependencySetIDPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// Version is an immutable semantic contract version.
type Version struct {
	value string
}

// ParseVersion accepts the strict major.minor.patch form used by registry
// records. Pre-release aliases are deliberately not accepted as immutable
// contract identities.
func ParseVersion(value string) (Version, error) {
	if len(value) > MaxContractStringBytes || !versionPattern.MatchString(value) {
		return Version{}, fmt.Errorf("invalid contract version %q", value)
	}
	return Version{value: value}, nil
}

// MustParseVersion parses a version or panics. It is intended for static
// declarations.
func MustParseVersion(value string) Version {
	version, err := ParseVersion(value)
	if err != nil {
		panic(err)
	}
	return version
}

// String returns the canonical major.minor.patch representation.
func (version Version) String() string {
	return version.value
}

// Valid reports whether version is a contract version: the strict
// major.minor.patch form ParseVersion accepts, at most MaxContractStringBytes
// bytes long. The zero Version is not valid.
func (version Version) Valid() bool {
	return len(version.value) <= MaxContractStringBytes &&
		versionPattern.MatchString(version.value)
}

// valid is Valid for the root contract files that call the unexported name.
func (version Version) valid() bool { return version.Valid() }

// MarshalText preserves versions as strings in deterministic records.
func (version Version) MarshalText() ([]byte, error) {
	if !version.Valid() {
		return nil, fmt.Errorf("invalid zero contract version")
	}
	return []byte(version.value), nil
}

// MarshalJSON preserves the public serialized schema.
func (version Version) MarshalJSON() ([]byte, error) {
	if !version.Valid() {
		return nil, fmt.Errorf("invalid zero contract version")
	}
	return json.Marshal(version.value)
}

// UnmarshalJSON rejects unknown version syntax instead of coercing it.
func (version *Version) UnmarshalJSON(data []byte) error {
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	parsed, err := ParseVersion(value)
	if err != nil {
		return err
	}
	*version = parsed
	return nil
}

var (
	schemaVersionV1          = MustParseVersion("1.0.0")
	codecContractVersionV1   = MustParseVersion("1.0.0")
	runtimeContractVersionV1 = MustParseVersion("1.0.0")
)

// CurrentSchemaVersion returns the immutable M2-01 serialized schema authority.
func CurrentSchemaVersion() Version {
	return schemaVersionV1
}

// CurrentCodecContractVersion returns the immutable codec contract authority.
func CurrentCodecContractVersion() Version {
	return codecContractVersionV1
}

// PinnedRuntimeContractVersion returns the exact M1-06 runtime contract
// consumed by this package.
func PinnedRuntimeContractVersion() Version {
	return runtimeContractVersionV1
}

// CompareVersions orders two contract versions numerically by major, minor
// and patch component and returns -1, 0 or +1. An invalid version, including
// the zero Version, orders before every valid version and equal to any other
// invalid version.
func CompareVersions(first, second Version) int {
	switch firstValid, secondValid := first.Valid(), second.Valid(); {
	case !firstValid && !secondValid:
		return 0
	case !firstValid:
		return -1
	case !secondValid:
		return 1
	}
	firstParts := strings.Split(first.String(), ".")
	secondParts := strings.Split(second.String(), ".")
	for index := 0; index < 3; index++ {
		firstPart := strings.TrimLeft(firstParts[index], "0")
		secondPart := strings.TrimLeft(secondParts[index], "0")
		if firstPart == "" {
			firstPart = "0"
		}
		if secondPart == "" {
			secondPart = "0"
		}
		if len(firstPart) < len(secondPart) {
			return -1
		}
		if len(firstPart) > len(secondPart) {
			return 1
		}
		if firstPart < secondPart {
			return -1
		}
		if firstPart > secondPart {
			return 1
		}
	}
	return 0
}

// ValidIdentity reports whether value is a contract identity: a non-empty
// string of at most MaxContractStringBytes bytes made only of ASCII letters,
// digits and the characters '.', '-', '_' and ':'. Profile names, versions,
// sources and other identity fields of the registry contracts follow this rule.
func ValidIdentity(value string) bool {
	if value == "" || len(value) > MaxContractStringBytes {
		return false
	}
	for _, character := range value {
		switch {
		case character >= 'a' && character <= 'z':
		case character >= 'A' && character <= 'Z':
		case character >= '0' && character <= '9':
		case character == '.', character == '-', character == '_', character == ':':
		default:
			return false
		}
	}
	return true
}

// ValidDependencySetID reports whether value is a dependency-set identity:
// "sha256:" followed by 64 lower-case hexadecimal digits.
func ValidDependencySetID(value string) bool {
	return len(value) == len("sha256:")+64 &&
		dependencySetIDPattern.MatchString(value)
}

func stringsComplete(values []string) bool {
	if len(values) == 0 ||
		len(values) > MaxProfileEvidenceReferences ||
		!stringsDeclared(values) {
		return false
	}
	return true
}

func stringsDeclared(values []string) bool {
	if values == nil || len(values) > MaxProfileEvidenceReferences {
		return false
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if ValidateBoundedString("string collection", value, true) != nil {
			return false
		}
		if _, exists := seen[value]; exists {
			return false
		}
		seen[value] = struct{}{}
	}
	return true
}
