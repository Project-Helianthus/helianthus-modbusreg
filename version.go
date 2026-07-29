package modbusreg

import (
	"encoding/json"
	"fmt"
	"regexp"
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

func (version Version) valid() bool {
	return len(version.value) <= MaxContractStringBytes &&
		versionPattern.MatchString(version.value)
}

// MarshalText preserves versions as strings in deterministic records.
func (version Version) MarshalText() ([]byte, error) {
	if !version.valid() {
		return nil, fmt.Errorf("invalid zero contract version")
	}
	return []byte(version.value), nil
}

// MarshalJSON preserves the public serialized schema.
func (version Version) MarshalJSON() ([]byte, error) {
	if !version.valid() {
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

// PinnedRuntimeContractVersion returns the exact M1-04 runtime contract
// consumed by this package.
func PinnedRuntimeContractVersion() Version {
	return runtimeContractVersionV1
}

func cloneStrings(values []string) []string {
	if values == nil {
		return nil
	}
	return append([]string{}, values...)
}

func validIdentity(value string) bool {
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

func validDependencySetID(value string) bool {
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
		if validateBoundedString("string collection", value, true) != nil {
			return false
		}
		if _, exists := seen[value]; exists {
			return false
		}
		seen[value] = struct{}{}
	}
	return true
}
