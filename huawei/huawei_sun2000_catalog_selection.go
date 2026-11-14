package huawei

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/Project-Helianthus/helianthus-modbusreg/profiles"
)

// The SUN2000 catalog decision reads the generated inverter catalog (see
// docs/huawei-catalog.md) and decides which model row, if any, a routed
// inverter belongs to. It reads the editions, the model rows and the identity
// rows of each card; it decodes no register and grants no authority. The
// directory is the catalog's path inside the inverter part that package
// profiles embeds.
const huaweiSUN2000CatalogDirectory = "vendors/huawei/catalog"

// SUN2000EndpointScope is the address space in which the identity words
// were read. A model row is a candidate only within the scope of its card's
// map; no decision crosses scopes.
type SUN2000EndpointScope string

const (
	// SUN2000ScopeInverterMap is the inverter register map that carries
	// the model text at 30000 and the model identifier at 30070, read at an
	// inverter unit behind a logger, an EMMA or a dongle, or directly.
	SUN2000ScopeInverterMap SUN2000EndpointScope = "INVERTER_MAP"
	// SUN2000ScopeC81Map is the C81 address map, whose card documents no
	// model text at 30000 and its model identifier elsewhere.
	SUN2000ScopeC81Map SUN2000EndpointScope = "C81_MAP"
	// SUN2000ScopeGatewayNamespace is a gateway's own namespace. No
	// inverter model row belongs to it.
	SUN2000ScopeGatewayNamespace SUN2000EndpointScope = "GATEWAY_NAMESPACE"
)

// SUN2000CatalogOutcome is the outcome of the catalog decision.
type SUN2000CatalogOutcome string

// Catalog outcomes. Only SUN2000CatalogSelected carries a row.
const (
	// SUN2000CatalogSelected means exactly one candidate row admitted
	// the software text and the identifier was read.
	SUN2000CatalogSelected SUN2000CatalogOutcome = "SELECTED"
	// SUN2000CatalogNotSelected is the fail-closed outcome of every
	// decision that neither selected nor found two rows.
	SUN2000CatalogNotSelected SUN2000CatalogOutcome = "NOT_SELECTED"
	// SUN2000CatalogAmbiguous means two or more candidate rows
	// remained; none is chosen.
	SUN2000CatalogAmbiguous SUN2000CatalogOutcome = "AMBIGUOUS"
)

// SUN2000CatalogReason is the closed reason of a catalog decision.
type SUN2000CatalogReason string

const (
	// SUN2000CatalogAtOrAboveFloor indicates software at or above the selected row floor.
	SUN2000CatalogAtOrAboveFloor SUN2000CatalogReason = "at_or_above_floor"
	// SUN2000CatalogEqualOnCarriedParts selects a row whose floor
	// carries more parts than the software text and agrees on the parts both
	// carry.
	SUN2000CatalogEqualOnCarriedParts SUN2000CatalogReason = "equal_on_carried_parts"
	// SUN2000CatalogNoDocumentedRelation selects a row whose floor is of
	// another product, line, release or branch; an unrelated build is never
	// refused.
	SUN2000CatalogNoDocumentedRelation SUN2000CatalogReason = "no_documented_branch_relation"
	// SUN2000CatalogFloorOutsideBranchForm selects a row whose floor
	// text cannot be parsed in the branch form and so was not compared.
	SUN2000CatalogFloorOutsideBranchForm SUN2000CatalogReason = "floor_outside_branch_form"
	// SUN2000CatalogModelTextInvalid means the model text is empty,
	// longer than the model read or not printable ASCII.
	SUN2000CatalogModelTextInvalid SUN2000CatalogReason = "model_text_invalid"
	// SUN2000CatalogScopeInvalid means the input scope is not one of the
	// declared endpoint scopes.
	SUN2000CatalogScopeInvalid SUN2000CatalogReason = "scope_invalid"
	// SUN2000CatalogSoftwareMalformed means a software or firmware text
	// was read but is not in the branch form.
	SUN2000CatalogSoftwareMalformed SUN2000CatalogReason = "software_malformed"
	// SUN2000CatalogNoCatalogRow means no row's token or scoped wire
	// alias equals the model text.
	SUN2000CatalogNoCatalogRow SUN2000CatalogReason = "no_catalog_row"
	// SUN2000CatalogIdentifierMismatch means rows matched the model text
	// but none carries the identifier read; the matched rows are the
	// candidates.
	SUN2000CatalogIdentifierMismatch SUN2000CatalogReason = "identifier_mismatch"
	// SUN2000CatalogScopeMismatch means no remaining row belongs to the
	// map of the input scope.
	SUN2000CatalogScopeMismatch SUN2000CatalogReason = "scope_mismatch"
	// SUN2000CatalogSoftwareAbsent means the identifier was read but
	// neither software nor firmware text was.
	SUN2000CatalogSoftwareAbsent SUN2000CatalogReason = "software_absent"
	// SUN2000CatalogIdentifierAbsent means a single row remained but no
	// identifier was read; the token alone never selects.
	SUN2000CatalogIdentifierAbsent SUN2000CatalogReason = "identifier_absent"
	// SUN2000CatalogBelowFloor means the software text is below the
	// floor of every remaining row within its branch.
	SUN2000CatalogBelowFloor SUN2000CatalogReason = "below_floor"
	// SUN2000CatalogInvalid means the embedded catalog failed to load
	// or holds no row; nothing can be selected.
	SUN2000CatalogInvalid SUN2000CatalogReason = "catalog_invalid"
	// SUN2000CatalogMultipleCandidateRows accompanies an ambiguous
	// decision.
	SUN2000CatalogMultipleCandidateRows SUN2000CatalogReason = "multiple_candidate_rows"
)

const (
	huaweiSUN2000IdentifierOffset          = 30070
	huaweiSUN2000CatalogFloorBasisDocument = "document"
	huaweiSUN2000CatalogModelTextMaxBytes  = 2 * huaweiSUN2000ModelQuantity
)

// SUN2000CatalogMatch states how the model text matched a row.
type SUN2000CatalogMatch string

const (
	// SUN2000CatalogMatchToken the model text equals the source token.
	SUN2000CatalogMatchToken SUN2000CatalogMatch = "TOKEN"
	// SUN2000CatalogMatchWireAlias the model text equals a wire alias
	// that the catalog records for the row, within the alias's scope.
	SUN2000CatalogMatchWireAlias SUN2000CatalogMatch = "WIRE_ALIAS"
)

// SUN2000FloorRelation is how a software text relates to a row floor
// under the catalog's branch form: a line (V), a release (R), a branch (C)
// and a build (SPC). Builds of one branch are ordered by their SPC number; no
// two branches are ordered.
type SUN2000FloorRelation string

// Floor relations. Only SUN2000FloorBelow removes a candidate row.
const (
	// SUN2000FloorAtOrAbove means the build is at or above the floor
	// within the same branch, or the floor stops at a release or branch that
	// the build shares.
	SUN2000FloorAtOrAbove SUN2000FloorRelation = "AT_OR_ABOVE"
	// SUN2000FloorEqualOnCarriedParts means the build carries fewer
	// parts than the floor and agrees on those it carries.
	SUN2000FloorEqualOnCarriedParts SUN2000FloorRelation = "EQUAL_ON_CARRIED_PARTS"
	// SUN2000FloorNoDocumentedRelation means the build and the floor
	// differ in product, line, release or branch, which are never ordered.
	SUN2000FloorNoDocumentedRelation SUN2000FloorRelation = "NO_DOCUMENTED_RELATION"
	// SUN2000FloorOutsideBranchForm means the floor text is not in the
	// branch form and cannot be compared.
	SUN2000FloorOutsideBranchForm SUN2000FloorRelation = "FLOOR_OUTSIDE_BRANCH_FORM"
	// SUN2000FloorBelow means the build has a lower SPC number than the
	// floor within the same branch.
	SUN2000FloorBelow SUN2000FloorRelation = "BELOW"
	// SUN2000FloorNotEvaluated marks a row that matched the model text
	// but was not compared with a build.
	SUN2000FloorNotEvaluated SUN2000FloorRelation = "NOT_EVALUATED"
)

// SUN2000BuildSource names the identity word that supplied the software
// text compared with the floor.
type SUN2000BuildSource string

// Build sources. The software text wins over the firmware text when both were
// read.
const (
	// SUN2000BuildNone means no software or firmware text was read.
	SUN2000BuildNone SUN2000BuildSource = ""
	// SUN2000BuildSoftwareText is the software text read at 30050.
	SUN2000BuildSoftwareText SUN2000BuildSource = "SOFTWARE_TEXT_30050"
	// SUN2000BuildFirmwareText is the firmware text read at 30035, used
	// only when no software text was read.
	SUN2000BuildFirmwareText SUN2000BuildSource = "FIRMWARE_TEXT_30035"
)

// SUN2000CatalogEdition is the identity of a catalog edition: its
// catalog key and its title, issue and date as the edition states them.
type SUN2000CatalogEdition struct {
	ID    string
	Title string
	Issue string
	Date  string
}

// SUN2000CatalogRow is one model row of a card edition as the decision
// saw it.
type SUN2000CatalogRow struct {
	Card       string
	Scope      SUN2000EndpointScope
	Edition    SUN2000CatalogEdition
	Page       int
	Token      string
	Identifier int
	// HasIdentifier is false for a row numbered by a sequence ordinal.
	HasIdentifier bool
	MatchedBy     SUN2000CatalogMatch
	// WireAlias and AliasEvidence are set when the row matched by its alias.
	WireAlias     string
	AliasEvidence string
	// Floor is the minimum software of the row; FloorBasis is "row" or
	// "document", and DocumentRequirement names the requirement of a document
	// floor.
	Floor               string
	FloorBasis          string
	DocumentRequirement string
	FloorRelation       SUN2000FloorRelation
	Quarantine          []string
}

func (r SUN2000CatalogRow) clone() SUN2000CatalogRow {
	r.Quarantine = append([]string(nil), r.Quarantine...)
	return r
}

// DocumentFloor reports a floor taken from a document-wide requirement
// because the row states none.
func (r SUN2000CatalogRow) DocumentFloor() bool {
	return r.FloorBasis == huaweiSUN2000CatalogFloorBasisDocument
}

// SUN2000CatalogInput is the identity evidence already read at one
// endpoint, decoded, together with the scope it was read in. It carries no
// endpoint, session or transport state.
type SUN2000CatalogInput struct {
	Scope SUN2000EndpointScope
	// ModelText is the model text read at 30000.
	ModelText string
	// ModelIdentifier is the word read at 30070, or nil when it was not read.
	ModelIdentifier *uint16
	// SoftwareText is the software text read at 30050, or empty.
	SoftwareText string
	// FirmwareText is the firmware text read at 30035, or empty. It is
	// compared with the floor only when no software text was read.
	FirmwareText string
	// ProtocolWords are the words read at 30068. They are recorded and never
	// compared.
	ProtocolWords []uint16
}

// SUN2000CatalogDecision is the result of the catalog decision. A
// row is selected only when exactly one candidate row admits the software
// text; order, priority and card name never choose a winner.
type SUN2000CatalogDecision struct {
	outcome        SUN2000CatalogOutcome
	reason         SUN2000CatalogReason
	scope          SUN2000EndpointScope
	modelText      string
	identifier     uint16
	identifierRead bool
	build          string
	buildSource    SUN2000BuildSource
	firmwareText   string
	protocolWords  []uint16
	selected       *SUN2000CatalogRow
	candidates     []SUN2000CatalogRow
}

// Outcome returns the decision outcome.
func (d SUN2000CatalogDecision) Outcome() SUN2000CatalogOutcome { return d.outcome }

// Reason returns the closed reason: the floor relation of a selected row, or
// the stage at which nothing was selected.
func (d SUN2000CatalogDecision) Reason() SUN2000CatalogReason { return d.reason }

// Scope returns the input scope as given, valid or not.
func (d SUN2000CatalogDecision) Scope() SUN2000EndpointScope { return d.scope }

// ModelText returns the input model text as given, valid or not.
func (d SUN2000CatalogDecision) ModelText() string { return d.modelText }

// Identifier is the model identifier that was read, if one was.
func (d SUN2000CatalogDecision) Identifier() (uint16, bool) {
	return d.identifier, d.identifierRead
}

// Build is the software text compared with the floor and the word it came
// from.
func (d SUN2000CatalogDecision) Build() (string, SUN2000BuildSource) {
	return d.build, d.buildSource
}

// FirmwareText returns the firmware text as read, recorded even when the
// software text was the one compared.
func (d SUN2000CatalogDecision) FirmwareText() string { return d.firmwareText }

// ProtocolWords returns a copy of the protocol words as read; they are
// recorded and never compared.
func (d SUN2000CatalogDecision) ProtocolWords() []uint16 {
	return append([]uint16(nil), d.protocolWords...)
}

// Grade is the grade of the selected row's facts: Hypothesis until an
// observation grades them, and Unknown when nothing is selected.
func (d SUN2000CatalogDecision) Grade() EvidenceGrade {
	if d.outcome == SUN2000CatalogSelected {
		return GradeHypothesis
	}
	return GradeUnknown
}

// Row is the selected row, or nothing.
func (d SUN2000CatalogDecision) Row() (SUN2000CatalogRow, bool) {
	if d.selected == nil {
		return SUN2000CatalogRow{}, false
	}
	return d.selected.clone(), true
}

// Candidates are the rows that the decision named: every admitting row of an
// ambiguous decision, the selected row, or the rows that the closing stage of
// a decision that selected nothing considered.
func (d SUN2000CatalogDecision) Candidates() []SUN2000CatalogRow {
	out := make([]SUN2000CatalogRow, len(d.candidates))
	for index, row := range d.candidates {
		out[index] = row.clone()
	}
	return out
}

// LiveQualified is always false: a catalog row is documentary evidence.
func (SUN2000CatalogDecision) LiveQualified() bool { return false }

// SupportClaim is always false: a selected row is not a support claim.
func (SUN2000CatalogDecision) SupportClaim() bool { return false }

// WriteAuthority is always false: the decision grants no authority.
func (SUN2000CatalogDecision) WriteAuthority() bool { return false }

// huaweiSUN2000CatalogEntry is one model row of a card edition.
type huaweiSUN2000CatalogEntry struct {
	row     SUN2000CatalogRow
	aliases []huaweiSUN2000CatalogAlias
}

type huaweiSUN2000CatalogAlias struct {
	token    string
	evidence string
	scope    SUN2000EndpointScope
}

type huaweiSUN2000Catalog struct {
	entries []huaweiSUN2000CatalogEntry
	err     error
}

var (
	huaweiSUN2000CatalogOnce   sync.Once
	huaweiSUN2000CatalogLoaded huaweiSUN2000Catalog

	errHuaweiSUN2000CatalogInvalid = errors.New("invalid SUN2000 catalog")

	// huaweiSUN2000VersionForm is the catalog's branch form: an optional
	// product prefix, then the line, the release, the branch and the build.
	huaweiSUN2000VersionForm = regexp.MustCompile(`^(?:(SUN[0-9]+[A-Z]*) )?V([0-9]{3})R([0-9]{3})(?:C([0-9]{2})(?:SPC([0-9]{3}))?)?$`)
	// huaweiSUN2000AliasIdentifier finds the exact identifier an alias scope names.
	huaweiSUN2000AliasIdentifier = regexp.MustCompile(`\bID([0-9]+)\b`)
)

func huaweiSUN2000LoadedCatalog() huaweiSUN2000Catalog {
	huaweiSUN2000CatalogOnce.Do(func() {
		entries, err := loadHuaweiSUN2000Catalog(func(name string) ([]byte, error) {
			return profiles.HuaweiInverterCatalog().ReadFile(huaweiSUN2000CatalogDirectory + "/" + name)
		}, func() ([]string, error) {
			found, err := profiles.HuaweiInverterCatalog().ReadDir(huaweiSUN2000CatalogDirectory)
			if err != nil {
				return nil, err
			}
			names := make([]string, 0, len(found))
			for _, entry := range found {
				names = append(names, entry.Name())
			}
			return names, nil
		})
		huaweiSUN2000CatalogLoaded = huaweiSUN2000Catalog{entries: entries, err: err}
	})
	return huaweiSUN2000CatalogLoaded
}

func decodeHuaweiSUN2000CatalogFile(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if decoder.More() {
		return errHuaweiSUN2000CatalogInvalid
	}
	return nil
}

// loadHuaweiSUN2000Catalog reads the card editions, their model rows and the
// identity rows of each card. The map scope of a card comes from its identity
// rows: a card that documents the model text at 30000 and the model
// identifier at 30070 at the inverter endpoint is the inverter map; a card
// that documents its model identifier at another inverter offset and no model
// text at 30000 is the C81 map. Any other shape is refused.
func loadHuaweiSUN2000Catalog(read func(string) ([]byte, error), list func() ([]string, error)) ([]huaweiSUN2000CatalogEntry, error) {
	var editions struct {
		Editions []struct {
			ID    string `json:"id"`
			Title string `json:"title"`
			Issue string `json:"issue"`
			Date  string `json:"date"`
			Card  string `json:"card"`
		} `json:"editions"`
	}
	var models struct {
		Rows []struct {
			Edition             string `json:"edition"`
			Page                int    `json:"page"`
			Token               string `json:"token"`
			Identifier          *int   `json:"identifier"`
			MinimumSoftware     string `json:"minimum_software"`
			FloorBasis          string `json:"floor_basis"`
			DocumentRequirement string `json:"document_requirement"`
			WireAliases         []struct {
				Token    string `json:"token"`
				Evidence string `json:"evidence"`
				Scope    string `json:"scope"`
			} `json:"wire_aliases"`
			Quarantine []string `json:"quarantine"`
		} `json:"rows"`
	}
	for name, target := range map[string]any{"editions.json": &editions, "models.json": &models} {
		data, err := read(name)
		if err != nil {
			return nil, err
		}
		if err := decodeHuaweiSUN2000CatalogFile(data, target); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
	}
	names, err := list()
	if err != nil {
		return nil, err
	}
	type cardMap struct {
		edition SUN2000CatalogEdition
		scope   SUN2000EndpointScope
	}
	cards := map[string]cardMap{}
	byEdition := map[string]string{}
	for _, edition := range editions.Editions {
		if edition.Card == "" {
			continue
		}
		if _, repeated := cards[edition.Card]; repeated || edition.ID == "" {
			return nil, fmt.Errorf("%w: card %q", errHuaweiSUN2000CatalogInvalid, edition.Card)
		}
		cards[edition.Card] = cardMap{edition: SUN2000CatalogEdition{ID: edition.ID, Title: edition.Title, Issue: edition.Issue, Date: edition.Date}}
		byEdition[edition.ID] = edition.Card
	}
	for _, name := range names {
		if !strings.HasPrefix(name, "registers-") || !strings.HasSuffix(name, ".json") {
			continue
		}
		var registers struct {
			Card    string `json:"card"`
			Edition string `json:"edition"`
			Rows    []struct {
				Offset   *int   `json:"offset"`
				Quantity int    `json:"quantity"`
				Endpoint string `json:"endpoint"`
				Key      string `json:"key"`
			} `json:"rows"`
		}
		data, err := read(name)
		if err != nil {
			return nil, err
		}
		if err := decodeHuaweiSUN2000CatalogFile(data, &registers); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		card, ok := cards[registers.Card]
		if !ok || card.edition.ID != registers.Edition || card.scope != "" {
			return nil, fmt.Errorf("%w: %s does not name one card edition", errHuaweiSUN2000CatalogInvalid, name)
		}
		var modelText, modelIdentifier, otherIdentifier bool
		for _, row := range registers.Rows {
			if row.Endpoint != "inverter" || row.Offset == nil {
				continue
			}
			switch {
			case *row.Offset == huaweiSUN2000ModelOffset && row.Quantity == huaweiSUN2000ModelQuantity:
				modelText = true
			case *row.Offset == huaweiSUN2000IdentifierOffset && row.Quantity == 1:
				modelIdentifier = true
			case strings.HasSuffix(row.Key, ".modelid") && row.Quantity == 1:
				otherIdentifier = true
			}
		}
		switch {
		case modelText && modelIdentifier:
			card.scope = SUN2000ScopeInverterMap
		case !modelText && !modelIdentifier && otherIdentifier:
			card.scope = SUN2000ScopeC81Map
		default:
			return nil, fmt.Errorf("%w: %s has no identity map", errHuaweiSUN2000CatalogInvalid, name)
		}
		cards[registers.Card] = card
	}
	for name, card := range cards {
		if card.scope == "" {
			return nil, fmt.Errorf("%w: card %s has no registers", errHuaweiSUN2000CatalogInvalid, name)
		}
	}
	var entries []huaweiSUN2000CatalogEntry
	for _, source := range models.Rows {
		cardName, ok := byEdition[source.Edition]
		if !ok {
			continue
		}
		card := cards[cardName]
		if source.Token == "" || source.MinimumSoftware == "" || (source.FloorBasis == huaweiSUN2000CatalogFloorBasisDocument) != (source.DocumentRequirement != "") {
			return nil, fmt.Errorf("%w: row %s %q", errHuaweiSUN2000CatalogInvalid, source.Edition, source.Token)
		}
		entry := huaweiSUN2000CatalogEntry{row: SUN2000CatalogRow{
			Card: cardName, Scope: card.scope, Edition: card.edition, Page: source.Page, Token: source.Token,
			Floor: source.MinimumSoftware, FloorBasis: source.FloorBasis, DocumentRequirement: source.DocumentRequirement,
			Quarantine: append([]string(nil), source.Quarantine...),
		}}
		if source.Identifier != nil {
			if *source.Identifier < 0 || *source.Identifier > 0xffff {
				return nil, fmt.Errorf("%w: identifier %d", errHuaweiSUN2000CatalogInvalid, *source.Identifier)
			}
			entry.row.Identifier, entry.row.HasIdentifier = *source.Identifier, true
		}
		for _, alias := range source.WireAliases {
			// An alias holds only within the scope that the catalog records
			// for it: the exact identifier of its row and the inverter endpoint.
			// An alias whose scope does not state both is never applied.
			named := huaweiSUN2000AliasIdentifier.FindStringSubmatch(alias.Scope)
			if alias.Token == "" || !entry.row.HasIdentifier || named == nil || named[1] != strconv.Itoa(entry.row.Identifier) ||
				!strings.Contains(alias.Scope, "inverter endpoint") || card.scope != SUN2000ScopeInverterMap {
				continue
			}
			entry.aliases = append(entry.aliases, huaweiSUN2000CatalogAlias{token: alias.Token, evidence: alias.Evidence, scope: SUN2000ScopeInverterMap})
		}
		entries = append(entries, entry)
	}
	if len(entries) == 0 {
		return nil, errHuaweiSUN2000CatalogInvalid
	}
	return entries, nil
}

type huaweiSUN2000Version struct {
	product, line, release, branch string
	build                          int
	hasBranch, hasBuild            bool
}

func parseHuaweiSUN2000Version(text string) (huaweiSUN2000Version, bool) {
	parts := huaweiSUN2000VersionForm.FindStringSubmatch(text)
	if parts == nil {
		return huaweiSUN2000Version{}, false
	}
	version := huaweiSUN2000Version{product: parts[1], line: parts[2], release: parts[3], branch: parts[4], hasBranch: parts[4] != ""}
	if parts[5] != "" {
		build, err := strconv.Atoi(parts[5])
		if err != nil {
			return huaweiSUN2000Version{}, false
		}
		version.build, version.hasBuild = build, true
	}
	return version, true
}

// huaweiSUN2000FloorRelationOf compares a software text with a floor. The
// texts are compared only within one branch, and only on the parts both carry;
// two different products, lines, releases or branches have no documented
// relation and the text is never refused for that reason. A floor outside the
// branch form cannot be compared.
func huaweiSUN2000FloorRelationOf(build huaweiSUN2000Version, floor string) SUN2000FloorRelation {
	minimum, ok := parseHuaweiSUN2000Version(floor)
	switch {
	case !ok:
		return SUN2000FloorOutsideBranchForm
	case minimum.product != "" && build.product != "" && minimum.product != build.product,
		minimum.line != build.line, minimum.release != build.release:
		return SUN2000FloorNoDocumentedRelation
	case !minimum.hasBranch:
		// The floor names the release as a whole.
		return SUN2000FloorAtOrAbove
	case !build.hasBranch:
		return SUN2000FloorEqualOnCarriedParts
	case build.branch != minimum.branch:
		return SUN2000FloorNoDocumentedRelation
	case !minimum.hasBuild:
		return SUN2000FloorAtOrAbove
	case !build.hasBuild:
		return SUN2000FloorEqualOnCarriedParts
	case build.build >= minimum.build:
		return SUN2000FloorAtOrAbove
	}
	return SUN2000FloorBelow
}

func huaweiSUN2000SelectedReason(relation SUN2000FloorRelation) SUN2000CatalogReason {
	switch relation {
	case SUN2000FloorAtOrAbove:
		return SUN2000CatalogAtOrAboveFloor
	case SUN2000FloorEqualOnCarriedParts:
		return SUN2000CatalogEqualOnCarriedParts
	case SUN2000FloorNoDocumentedRelation:
		return SUN2000CatalogNoDocumentedRelation
	}
	return SUN2000CatalogFloorOutsideBranchForm
}

func validHuaweiSUN2000ModelText(text string) bool {
	if text == "" || len(text) > huaweiSUN2000CatalogModelTextMaxBytes {
		return false
	}
	for index := 0; index < len(text); index++ {
		if text[index] < 0x20 || text[index] > 0x7e {
			return false
		}
	}
	return true
}

// DecideHuaweiSUN2000CatalogSelection decides which catalog model row the
// identity words belong to. A candidate row is a row of a card edition whose
// exact token, or a wire alias recorded for it within its scope, equals the
// model text, whose identifier equals the identifier read, and whose card map
// is the scope of the read. The software text, or the firmware text when no
// software text was read, is compared with each candidate's floor; a text below
// a floor within its branch removes that row, and a text with no documented
// relation to a floor keeps it. Exactly one remaining row is selected, graded
// Hypothesis; two or more are ambiguous and none is selected. Without an
// identifier no row is selected: the token alone does not identify a model.
func DecideHuaweiSUN2000CatalogSelection(input SUN2000CatalogInput) SUN2000CatalogDecision {
	return decideHuaweiSUN2000Catalog(huaweiSUN2000LoadedCatalog(), input)
}

func decideHuaweiSUN2000Catalog(catalog huaweiSUN2000Catalog, input SUN2000CatalogInput) SUN2000CatalogDecision {
	decision := SUN2000CatalogDecision{
		outcome: SUN2000CatalogNotSelected, scope: input.Scope, modelText: input.ModelText,
		firmwareText: input.FirmwareText, protocolWords: append([]uint16(nil), input.ProtocolWords...),
	}
	if input.ModelIdentifier != nil {
		decision.identifier, decision.identifierRead = *input.ModelIdentifier, true
	}
	switch {
	case input.SoftwareText != "":
		decision.build, decision.buildSource = input.SoftwareText, SUN2000BuildSoftwareText
	case input.FirmwareText != "":
		decision.build, decision.buildSource = input.FirmwareText, SUN2000BuildFirmwareText
	}
	notSelected := func(reason SUN2000CatalogReason, candidates []SUN2000CatalogRow) SUN2000CatalogDecision {
		decision.reason, decision.candidates = reason, candidates
		return decision
	}
	if !validHuaweiSUN2000ModelText(input.ModelText) {
		return notSelected(SUN2000CatalogModelTextInvalid, nil)
	}
	switch input.Scope {
	case SUN2000ScopeInverterMap, SUN2000ScopeC81Map, SUN2000ScopeGatewayNamespace:
	default:
		return notSelected(SUN2000CatalogScopeInvalid, nil)
	}
	build, buildRead := huaweiSUN2000Version{}, decision.buildSource != SUN2000BuildNone
	if buildRead {
		parsed, ok := parseHuaweiSUN2000Version(decision.build)
		if !ok {
			return notSelected(SUN2000CatalogSoftwareMalformed, nil)
		}
		build = parsed
	}
	if catalog.err != nil || len(catalog.entries) == 0 {
		return notSelected(SUN2000CatalogInvalid, nil)
	}

	var matched []SUN2000CatalogRow
	for _, entry := range catalog.entries {
		row := entry.row
		if row.Token == input.ModelText {
			row.MatchedBy = SUN2000CatalogMatchToken
		} else {
			for _, alias := range entry.aliases {
				if alias.token == input.ModelText && alias.scope == row.Scope {
					row.MatchedBy, row.WireAlias, row.AliasEvidence = SUN2000CatalogMatchWireAlias, alias.token, alias.evidence
					break
				}
			}
		}
		if row.MatchedBy == "" {
			continue
		}
		row.FloorRelation = SUN2000FloorNotEvaluated
		matched = append(matched, row.clone())
	}
	if len(matched) == 0 {
		return notSelected(SUN2000CatalogNoCatalogRow, nil)
	}
	candidates := matched
	if decision.identifierRead {
		candidates = nil
		for _, row := range matched {
			if row.HasIdentifier && row.Identifier == int(decision.identifier) {
				candidates = append(candidates, row)
			}
		}
		if len(candidates) == 0 {
			return notSelected(SUN2000CatalogIdentifierMismatch, matched)
		}
	}
	inScope := make([]SUN2000CatalogRow, 0, len(candidates))
	for _, row := range candidates {
		if row.Scope == input.Scope {
			inScope = append(inScope, row)
		}
	}
	if len(inScope) == 0 {
		return notSelected(SUN2000CatalogScopeMismatch, candidates)
	}
	if !buildRead {
		switch {
		case decision.identifierRead:
			return notSelected(SUN2000CatalogSoftwareAbsent, inScope)
		case len(inScope) > 1:
			decision.outcome = SUN2000CatalogAmbiguous
			return notSelected(SUN2000CatalogMultipleCandidateRows, inScope)
		}
		return notSelected(SUN2000CatalogIdentifierAbsent, inScope)
	}
	var admitted, below []SUN2000CatalogRow
	for _, row := range inScope {
		row.FloorRelation = huaweiSUN2000FloorRelationOf(build, row.Floor)
		if row.FloorRelation == SUN2000FloorBelow {
			below = append(below, row)
			continue
		}
		admitted = append(admitted, row)
	}
	switch {
	case len(admitted) == 0:
		return notSelected(SUN2000CatalogBelowFloor, below)
	case len(admitted) > 1:
		decision.outcome = SUN2000CatalogAmbiguous
		return notSelected(SUN2000CatalogMultipleCandidateRows, admitted)
	case !decision.identifierRead:
		return notSelected(SUN2000CatalogIdentifierAbsent, admitted)
	}
	selected := admitted[0]
	decision.outcome, decision.reason = SUN2000CatalogSelected, huaweiSUN2000SelectedReason(selected.FloorRelation)
	decision.selected, decision.candidates = &selected, []SUN2000CatalogRow{selected.clone()}
	return decision
}
