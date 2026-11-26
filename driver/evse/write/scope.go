package write

import (
	"strings"

	semreg "github.com/Project-Helianthus/helianthus-semreg/semreg/v1"
)

// identity is the ownership the session of the writer stamps on everything the
// writer declares and registers: the session's asset, source, epoch and
// generation replace the ones the reader was configured with.
type identity struct {
	asset      semreg.AssetID
	source     semreg.SourceID
	epoch      semreg.SourceEpochID
	generation semreg.Uint64
}

// scopeSuffixLength is the length of the digest of the source epoch that the
// session appends, after an at sign, to every identifier a driver chooses.
const scopeSuffixLength = 8

// scopedAs reports that got is the identifier base as the session names it: the
// identifier itself, or the identifier with the epoch suffix of the session.
func scopedAs[T ~string](base, got T) bool {
	if got == base {
		return true
	}
	suffix, ok := strings.CutPrefix(string(got), string(base)+"@")
	if !ok || len(suffix) != scopeSuffixLength {
		return false
	}
	for _, c := range suffix {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
