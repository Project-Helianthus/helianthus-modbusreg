package modbusreg

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// The SUN2000 catalogue decision reads the generated inverter catalogue (see
// docs/huawei-catalogue.md) and decides which model row, if any, a routed
// inverter belongs to. It reads the editions, the model rows and the identity
// rows of each card; it decodes no register and grants no authority.
//
//go:embed profiles/vendor/huawei/catalogue/editions.json profiles/vendor/huawei/catalogue/models.json profiles/vendor/huawei/catalogue/registers-*.json
var huaweiSUN2000CatalogueFiles embed.FS

const huaweiSUN2000CatalogueDirectory = "profiles/vendor/huawei/catalogue"

// HuaweiSUN2000EndpointScope is the address space in which the identity words
// were read. A model row is a candidate only within the scope of its card's
// map; no decision crosses scopes.
type HuaweiSUN2000EndpointScope string

const (
	// HuaweiSUN2000ScopeInverterMap is the inverter register map that carries
	// the model text at 30000 and the model identifier at 30070, read at an
	// inverter unit behind a logger, an EMMA or a dongle, or directly.
	HuaweiSUN2000ScopeInverterMap HuaweiSUN2000EndpointScope = "INVERTER_MAP"
	// HuaweiSUN2000ScopeC81Map is the C81 address map, whose card documents no
	// model text at 30000 and its model identifier elsewhere.
	HuaweiSUN2000ScopeC81Map HuaweiSUN2000EndpointScope = "C81_MAP"
	// HuaweiSUN2000ScopeGatewayNamespace is a gateway's own namespace. No
	// inverter model row belongs to it.
	HuaweiSUN2000ScopeGatewayNamespace HuaweiSUN2000EndpointScope = "GATEWAY_NAMESPACE"
)

// HuaweiSUN2000CatalogueOutcome is the outcome of the catalogue decision.
type HuaweiSUN2000CatalogueOutcome string

const (
	HuaweiSUN2000CatalogueSelected    HuaweiSUN2000CatalogueOutcome = "SELECTED"
	HuaweiSUN2000CatalogueNotSelected HuaweiSUN2000CatalogueOutcome = "NOT_SELECTED"
	HuaweiSUN2000CatalogueAmbiguous   HuaweiSUN2000CatalogueOutcome = "AMBIGUOUS"
)

// HuaweiSUN2000CatalogueReason is the closed reason of a catalogue decision.
type HuaweiSUN2000CatalogueReason string

const (
	// Reasons of a selected row: how the software text relates to its floor.
	HuaweiSUN2000CatalogueAtOrAboveFloor         HuaweiSUN2000CatalogueReason = "at_or_above_floor"
	HuaweiSUN2000CatalogueEqualOnCarriedParts    HuaweiSUN2000CatalogueReason = "equal_on_carried_parts"
	HuaweiSUN2000CatalogueNoDocumentedRelation   HuaweiSUN2000CatalogueReason = "no_documented_branch_relation"
	HuaweiSUN2000CatalogueFloorOutsideBranchForm HuaweiSUN2000CatalogueReason = "floor_outside_branch_form"
	HuaweiSUN2000CatalogueModelTextInvalid       HuaweiSUN2000CatalogueReason = "model_text_invalid"
	HuaweiSUN2000CatalogueScopeInvalid           HuaweiSUN2000CatalogueReason = "scope_invalid"
	HuaweiSUN2000CatalogueSoftwareMalformed      HuaweiSUN2000CatalogueReason = "software_malformed"
	HuaweiSUN2000CatalogueNoCatalogueRow         HuaweiSUN2000CatalogueReason = "no_catalogue_row"
	HuaweiSUN2000CatalogueIdentifierMismatch     HuaweiSUN2000CatalogueReason = "identifier_mismatch"
	HuaweiSUN2000CatalogueScopeMismatch          HuaweiSUN2000CatalogueReason = "scope_mismatch"
	HuaweiSUN2000CatalogueSoftwareAbsent         HuaweiSUN2000CatalogueReason = "software_absent"
	HuaweiSUN2000CatalogueIdentifierAbsent       HuaweiSUN2000CatalogueReason = "identifier_absent"
	HuaweiSUN2000CatalogueBelowFloor             HuaweiSUN2000CatalogueReason = "below_floor"
	HuaweiSUN2000CatalogueInvalid                HuaweiSUN2000CatalogueReason = "catalogue_invalid"
	HuaweiSUN2000CatalogueMultipleCandidateRows  HuaweiSUN2000CatalogueReason = "multiple_candidate_rows"
)

const (
	huaweiSUN2000IdentifierOffset            = 30070
	huaweiSUN2000CatalogueFloorBasisDocument = "document"
	huaweiSUN2000CatalogueModelTextMaxBytes  = 2 * huaweiSUN2000ModelQuantity
)

// HuaweiSUN2000CatalogueMatch states how the model text matched a row.
type HuaweiSUN2000CatalogueMatch string

const (
	// HuaweiSUN2000CatalogueMatchToken: the model text equals the source token.
	HuaweiSUN2000CatalogueMatchToken HuaweiSUN2000CatalogueMatch = "TOKEN"
	// HuaweiSUN2000CatalogueMatchWireAlias: the model text equals a wire alias
	// that the catalogue records for the row, within the alias's scope.
	HuaweiSUN2000CatalogueMatchWireAlias HuaweiSUN2000CatalogueMatch = "WIRE_ALIAS"
)

// HuaweiSUN2000FloorRelation is how a software text relates to a row floor
// under the catalogue's branch form: a line (V), a release (R), a branch (C)
// and a build (SPC). Builds of one branch are ordered by their SPC number; no
// two branches are ordered.
type HuaweiSUN2000FloorRelation string

const (
	HuaweiSUN2000FloorAtOrAbove            HuaweiSUN2000FloorRelation = "AT_OR_ABOVE"
	HuaweiSUN2000FloorEqualOnCarriedParts  HuaweiSUN2000FloorRelation = "EQUAL_ON_CARRIED_PARTS"
	HuaweiSUN2000FloorNoDocumentedRelation HuaweiSUN2000FloorRelation = "NO_DOCUMENTED_RELATION"
	HuaweiSUN2000FloorOutsideBranchForm    HuaweiSUN2000FloorRelation = "FLOOR_OUTSIDE_BRANCH_FORM"
	HuaweiSUN2000FloorBelow                HuaweiSUN2000FloorRelation = "BELOW"
	HuaweiSUN2000FloorNotEvaluated         HuaweiSUN2000FloorRelation = "NOT_EVALUATED"
)

// HuaweiSUN2000BuildSource names the identity word that supplied the software
// text compared with the floor.
type HuaweiSUN2000BuildSource string

const (
	HuaweiSUN2000BuildNone         HuaweiSUN2000BuildSource = ""
	HuaweiSUN2000BuildSoftwareText HuaweiSUN2000BuildSource = "SOFTWARE_TEXT_30050"
	HuaweiSUN2000BuildFirmwareText HuaweiSUN2000BuildSource = "FIRMWARE_TEXT_30035"
)

// HuaweiSUN2000CatalogueEdition is the identity of a catalogue edition: its
// catalogue key and its title, issue and date as the edition states them.
type HuaweiSUN2000CatalogueEdition struct {
	ID    string
	Title string
	Issue string
	Date  string
}

// HuaweiSUN2000CatalogueRow is one model row of a card edition as the decision
// saw it.
type HuaweiSUN2000CatalogueRow struct {
	Card       string
	Scope      HuaweiSUN2000EndpointScope
	Edition    HuaweiSUN2000CatalogueEdition
	Page       int
	Token      string
	Identifier int
	// HasIdentifier is false for a row numbered by a sequence ordinal.
	HasIdentifier bool
	MatchedBy     HuaweiSUN2000CatalogueMatch
	// WireAlias and AliasEvidence are set when the row matched by its alias.
	WireAlias     string
	AliasEvidence string
	// Floor is the minimum software of the row; FloorBasis is "row" or
	// "document", and DocumentRequirement names the requirement of a document
	// floor.
	Floor               string
	FloorBasis          string
	DocumentRequirement string
	FloorRelation       HuaweiSUN2000FloorRelation
	Quarantine          []string
}

func (r HuaweiSUN2000CatalogueRow) clone() HuaweiSUN2000CatalogueRow {
	r.Quarantine = append([]string(nil), r.Quarantine...)
	return r
}

// DocumentFloor reports a floor taken from a document-wide requirement
// because the row states none.
func (r HuaweiSUN2000CatalogueRow) DocumentFloor() bool {
	return r.FloorBasis == huaweiSUN2000CatalogueFloorBasisDocument
}

// HuaweiSUN2000CatalogueInput is the identity evidence already read at one
// endpoint, decoded, together with the scope it was read in. It carries no
// endpoint, session or transport state.
type HuaweiSUN2000CatalogueInput struct {
	Scope HuaweiSUN2000EndpointScope
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

// HuaweiSUN2000CatalogueDecision is the result of the catalogue decision. A
// row is selected only when exactly one candidate row admits the software
// text; order, priority and card name never choose a winner.
type HuaweiSUN2000CatalogueDecision struct {
	outcome        HuaweiSUN2000CatalogueOutcome
	reason         HuaweiSUN2000CatalogueReason
	scope          HuaweiSUN2000EndpointScope
	modelText      string
	identifier     uint16
	identifierRead bool
	build          string
	buildSource    HuaweiSUN2000BuildSource
	firmwareText   string
	protocolWords  []uint16
	selected       *HuaweiSUN2000CatalogueRow
	candidates     []HuaweiSUN2000CatalogueRow
}

func (d HuaweiSUN2000CatalogueDecision) Outcome() HuaweiSUN2000CatalogueOutcome { return d.outcome }
func (d HuaweiSUN2000CatalogueDecision) Reason() HuaweiSUN2000CatalogueReason   { return d.reason }
func (d HuaweiSUN2000CatalogueDecision) Scope() HuaweiSUN2000EndpointScope      { return d.scope }
func (d HuaweiSUN2000CatalogueDecision) ModelText() string                      { return d.modelText }

// Identifier is the model identifier that was read, if one was.
func (d HuaweiSUN2000CatalogueDecision) Identifier() (uint16, bool) {
	return d.identifier, d.identifierRead
}

// Build is the software text compared with the floor and the word it came
// from.
func (d HuaweiSUN2000CatalogueDecision) Build() (string, HuaweiSUN2000BuildSource) {
	return d.build, d.buildSource
}
func (d HuaweiSUN2000CatalogueDecision) FirmwareText() string { return d.firmwareText }
func (d HuaweiSUN2000CatalogueDecision) ProtocolWords() []uint16 {
	return append([]uint16(nil), d.protocolWords...)
}

// Grade is the grade of the selected row's facts: Hypothesis until an
// observation grades them, and Unknown when nothing is selected.
func (d HuaweiSUN2000CatalogueDecision) Grade() HuaweiEvidenceGrade {
	if d.outcome == HuaweiSUN2000CatalogueSelected {
		return HuaweiGradeHypothesis
	}
	return HuaweiGradeUnknown
}

// Row is the selected row, or nothing.
func (d HuaweiSUN2000CatalogueDecision) Row() (HuaweiSUN2000CatalogueRow, bool) {
	if d.selected == nil {
		return HuaweiSUN2000CatalogueRow{}, false
	}
	return d.selected.clone(), true
}

// Candidates are the rows that the decision named: every admitting row of an
// ambiguous decision, the selected row, or the rows that the closing stage of
// a decision that selected nothing considered.
func (d HuaweiSUN2000CatalogueDecision) Candidates() []HuaweiSUN2000CatalogueRow {
	out := make([]HuaweiSUN2000CatalogueRow, len(d.candidates))
	for index, row := range d.candidates {
		out[index] = row.clone()
	}
	return out
}

func (HuaweiSUN2000CatalogueDecision) LiveQualified() bool  { return false }
func (HuaweiSUN2000CatalogueDecision) SupportClaim() bool   { return false }
func (HuaweiSUN2000CatalogueDecision) WriteAuthority() bool { return false }

// huaweiSUN2000CatalogueEntry is one model row of a card edition.
type huaweiSUN2000CatalogueEntry struct {
	row     HuaweiSUN2000CatalogueRow
	aliases []huaweiSUN2000CatalogueAlias
}

type huaweiSUN2000CatalogueAlias struct {
	token    string
	evidence string
	scope    HuaweiSUN2000EndpointScope
}

type huaweiSUN2000Catalogue struct {
	entries []huaweiSUN2000CatalogueEntry
	err     error
}

var (
	huaweiSUN2000CatalogueOnce   sync.Once
	huaweiSUN2000CatalogueLoaded huaweiSUN2000Catalogue

	errHuaweiSUN2000CatalogueInvalid = errors.New("invalid SUN2000 catalogue")

	// huaweiSUN2000VersionForm is the catalogue's branch form: an optional
	// product prefix, then the line, the release, the branch and the build.
	huaweiSUN2000VersionForm = regexp.MustCompile(`^(?:(SUN[0-9]+[A-Z]*) )?V([0-9]{3})R([0-9]{3})(?:C([0-9]{2})(?:SPC([0-9]{3}))?)?$`)
	// huaweiSUN2000AliasIdentifier finds the exact identifier an alias scope names.
	huaweiSUN2000AliasIdentifier = regexp.MustCompile(`\bID([0-9]+)\b`)
)

func huaweiSUN2000LoadedCatalogue() huaweiSUN2000Catalogue {
	huaweiSUN2000CatalogueOnce.Do(func() {
		entries, err := loadHuaweiSUN2000Catalogue(func(name string) ([]byte, error) {
			return huaweiSUN2000CatalogueFiles.ReadFile(huaweiSUN2000CatalogueDirectory + "/" + name)
		}, func() ([]string, error) {
			found, err := huaweiSUN2000CatalogueFiles.ReadDir(huaweiSUN2000CatalogueDirectory)
			if err != nil {
				return nil, err
			}
			names := make([]string, 0, len(found))
			for _, entry := range found {
				names = append(names, entry.Name())
			}
			return names, nil
		})
		huaweiSUN2000CatalogueLoaded = huaweiSUN2000Catalogue{entries: entries, err: err}
	})
	return huaweiSUN2000CatalogueLoaded
}

func decodeHuaweiSUN2000CatalogueFile(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if decoder.More() {
		return errHuaweiSUN2000CatalogueInvalid
	}
	return nil
}

// loadHuaweiSUN2000Catalogue reads the card editions, their model rows and the
// identity rows of each card. The map scope of a card comes from its identity
// rows: a card that documents the model text at 30000 and the model
// identifier at 30070 at the inverter endpoint is the inverter map; a card
// that documents its model identifier at another inverter offset and no model
// text at 30000 is the C81 map. Any other shape is refused.
func loadHuaweiSUN2000Catalogue(read func(string) ([]byte, error), list func() ([]string, error)) ([]huaweiSUN2000CatalogueEntry, error) {
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
		if err := decodeHuaweiSUN2000CatalogueFile(data, target); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
	}
	names, err := list()
	if err != nil {
		return nil, err
	}
	type cardMap struct {
		edition HuaweiSUN2000CatalogueEdition
		scope   HuaweiSUN2000EndpointScope
	}
	cards := map[string]cardMap{}
	byEdition := map[string]string{}
	for _, edition := range editions.Editions {
		if edition.Card == "" {
			continue
		}
		if _, repeated := cards[edition.Card]; repeated || edition.ID == "" {
			return nil, fmt.Errorf("%w: card %q", errHuaweiSUN2000CatalogueInvalid, edition.Card)
		}
		cards[edition.Card] = cardMap{edition: HuaweiSUN2000CatalogueEdition{ID: edition.ID, Title: edition.Title, Issue: edition.Issue, Date: edition.Date}}
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
		if err := decodeHuaweiSUN2000CatalogueFile(data, &registers); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		card, ok := cards[registers.Card]
		if !ok || card.edition.ID != registers.Edition || card.scope != "" {
			return nil, fmt.Errorf("%w: %s does not name one card edition", errHuaweiSUN2000CatalogueInvalid, name)
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
			card.scope = HuaweiSUN2000ScopeInverterMap
		case !modelText && !modelIdentifier && otherIdentifier:
			card.scope = HuaweiSUN2000ScopeC81Map
		default:
			return nil, fmt.Errorf("%w: %s has no identity map", errHuaweiSUN2000CatalogueInvalid, name)
		}
		cards[registers.Card] = card
	}
	for name, card := range cards {
		if card.scope == "" {
			return nil, fmt.Errorf("%w: card %s has no registers", errHuaweiSUN2000CatalogueInvalid, name)
		}
	}
	var entries []huaweiSUN2000CatalogueEntry
	for _, source := range models.Rows {
		cardName, ok := byEdition[source.Edition]
		if !ok {
			continue
		}
		card := cards[cardName]
		if source.Token == "" || source.MinimumSoftware == "" || (source.FloorBasis == huaweiSUN2000CatalogueFloorBasisDocument) != (source.DocumentRequirement != "") {
			return nil, fmt.Errorf("%w: row %s %q", errHuaweiSUN2000CatalogueInvalid, source.Edition, source.Token)
		}
		entry := huaweiSUN2000CatalogueEntry{row: HuaweiSUN2000CatalogueRow{
			Card: cardName, Scope: card.scope, Edition: card.edition, Page: source.Page, Token: source.Token,
			Floor: source.MinimumSoftware, FloorBasis: source.FloorBasis, DocumentRequirement: source.DocumentRequirement,
			Quarantine: append([]string(nil), source.Quarantine...),
		}}
		if source.Identifier != nil {
			if *source.Identifier < 0 || *source.Identifier > 0xffff {
				return nil, fmt.Errorf("%w: identifier %d", errHuaweiSUN2000CatalogueInvalid, *source.Identifier)
			}
			entry.row.Identifier, entry.row.HasIdentifier = *source.Identifier, true
		}
		for _, alias := range source.WireAliases {
			// An alias holds only within the scope that the catalogue records
			// for it: the exact identifier of its row and the inverter endpoint.
			// An alias whose scope does not state both is never applied.
			named := huaweiSUN2000AliasIdentifier.FindStringSubmatch(alias.Scope)
			if alias.Token == "" || !entry.row.HasIdentifier || named == nil || named[1] != strconv.Itoa(entry.row.Identifier) ||
				!strings.Contains(alias.Scope, "inverter endpoint") || card.scope != HuaweiSUN2000ScopeInverterMap {
				continue
			}
			entry.aliases = append(entry.aliases, huaweiSUN2000CatalogueAlias{token: alias.Token, evidence: alias.Evidence, scope: HuaweiSUN2000ScopeInverterMap})
		}
		entries = append(entries, entry)
	}
	if len(entries) == 0 {
		return nil, errHuaweiSUN2000CatalogueInvalid
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
func huaweiSUN2000FloorRelationOf(build huaweiSUN2000Version, floor string) HuaweiSUN2000FloorRelation {
	minimum, ok := parseHuaweiSUN2000Version(floor)
	switch {
	case !ok:
		return HuaweiSUN2000FloorOutsideBranchForm
	case minimum.product != "" && build.product != "" && minimum.product != build.product,
		minimum.line != build.line, minimum.release != build.release:
		return HuaweiSUN2000FloorNoDocumentedRelation
	case !minimum.hasBranch:
		// The floor names the release as a whole.
		return HuaweiSUN2000FloorAtOrAbove
	case !build.hasBranch:
		return HuaweiSUN2000FloorEqualOnCarriedParts
	case build.branch != minimum.branch:
		return HuaweiSUN2000FloorNoDocumentedRelation
	case !minimum.hasBuild:
		return HuaweiSUN2000FloorAtOrAbove
	case !build.hasBuild:
		return HuaweiSUN2000FloorEqualOnCarriedParts
	case build.build >= minimum.build:
		return HuaweiSUN2000FloorAtOrAbove
	}
	return HuaweiSUN2000FloorBelow
}

func huaweiSUN2000SelectedReason(relation HuaweiSUN2000FloorRelation) HuaweiSUN2000CatalogueReason {
	switch relation {
	case HuaweiSUN2000FloorAtOrAbove:
		return HuaweiSUN2000CatalogueAtOrAboveFloor
	case HuaweiSUN2000FloorEqualOnCarriedParts:
		return HuaweiSUN2000CatalogueEqualOnCarriedParts
	case HuaweiSUN2000FloorNoDocumentedRelation:
		return HuaweiSUN2000CatalogueNoDocumentedRelation
	}
	return HuaweiSUN2000CatalogueFloorOutsideBranchForm
}

func validHuaweiSUN2000ModelText(text string) bool {
	if text == "" || len(text) > huaweiSUN2000CatalogueModelTextMaxBytes {
		return false
	}
	for index := 0; index < len(text); index++ {
		if text[index] < 0x20 || text[index] > 0x7e {
			return false
		}
	}
	return true
}

// DecideHuaweiSUN2000CatalogueSelection decides which catalogue model row the
// identity words belong to. A candidate row is a row of a card edition whose
// exact token, or a wire alias recorded for it within its scope, equals the
// model text, whose identifier equals the identifier read, and whose card map
// is the scope of the read. The software text, or the firmware text when no
// software text was read, is compared with each candidate's floor; a text below
// a floor within its branch removes that row, and a text with no documented
// relation to a floor keeps it. Exactly one remaining row is selected, graded
// Hypothesis; two or more are ambiguous and none is selected. Without an
// identifier no row is selected: the token alone does not identify a model.
func DecideHuaweiSUN2000CatalogueSelection(input HuaweiSUN2000CatalogueInput) HuaweiSUN2000CatalogueDecision {
	return decideHuaweiSUN2000Catalogue(huaweiSUN2000LoadedCatalogue(), input)
}

func decideHuaweiSUN2000Catalogue(catalogue huaweiSUN2000Catalogue, input HuaweiSUN2000CatalogueInput) HuaweiSUN2000CatalogueDecision {
	decision := HuaweiSUN2000CatalogueDecision{
		outcome: HuaweiSUN2000CatalogueNotSelected, scope: input.Scope, modelText: input.ModelText,
		firmwareText: input.FirmwareText, protocolWords: append([]uint16(nil), input.ProtocolWords...),
	}
	if input.ModelIdentifier != nil {
		decision.identifier, decision.identifierRead = *input.ModelIdentifier, true
	}
	switch {
	case input.SoftwareText != "":
		decision.build, decision.buildSource = input.SoftwareText, HuaweiSUN2000BuildSoftwareText
	case input.FirmwareText != "":
		decision.build, decision.buildSource = input.FirmwareText, HuaweiSUN2000BuildFirmwareText
	}
	notSelected := func(reason HuaweiSUN2000CatalogueReason, candidates []HuaweiSUN2000CatalogueRow) HuaweiSUN2000CatalogueDecision {
		decision.reason, decision.candidates = reason, candidates
		return decision
	}
	if !validHuaweiSUN2000ModelText(input.ModelText) {
		return notSelected(HuaweiSUN2000CatalogueModelTextInvalid, nil)
	}
	switch input.Scope {
	case HuaweiSUN2000ScopeInverterMap, HuaweiSUN2000ScopeC81Map, HuaweiSUN2000ScopeGatewayNamespace:
	default:
		return notSelected(HuaweiSUN2000CatalogueScopeInvalid, nil)
	}
	build, buildRead := huaweiSUN2000Version{}, decision.buildSource != HuaweiSUN2000BuildNone
	if buildRead {
		parsed, ok := parseHuaweiSUN2000Version(decision.build)
		if !ok {
			return notSelected(HuaweiSUN2000CatalogueSoftwareMalformed, nil)
		}
		build = parsed
	}
	if catalogue.err != nil || len(catalogue.entries) == 0 {
		return notSelected(HuaweiSUN2000CatalogueInvalid, nil)
	}

	var matched []HuaweiSUN2000CatalogueRow
	for _, entry := range catalogue.entries {
		row := entry.row
		if row.Token == input.ModelText {
			row.MatchedBy = HuaweiSUN2000CatalogueMatchToken
		} else {
			for _, alias := range entry.aliases {
				if alias.token == input.ModelText && alias.scope == row.Scope {
					row.MatchedBy, row.WireAlias, row.AliasEvidence = HuaweiSUN2000CatalogueMatchWireAlias, alias.token, alias.evidence
					break
				}
			}
		}
		if row.MatchedBy == "" {
			continue
		}
		row.FloorRelation = HuaweiSUN2000FloorNotEvaluated
		matched = append(matched, row.clone())
	}
	if len(matched) == 0 {
		return notSelected(HuaweiSUN2000CatalogueNoCatalogueRow, nil)
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
			return notSelected(HuaweiSUN2000CatalogueIdentifierMismatch, matched)
		}
	}
	inScope := make([]HuaweiSUN2000CatalogueRow, 0, len(candidates))
	for _, row := range candidates {
		if row.Scope == input.Scope {
			inScope = append(inScope, row)
		}
	}
	if len(inScope) == 0 {
		return notSelected(HuaweiSUN2000CatalogueScopeMismatch, candidates)
	}
	if !buildRead {
		switch {
		case decision.identifierRead:
			return notSelected(HuaweiSUN2000CatalogueSoftwareAbsent, inScope)
		case len(inScope) > 1:
			decision.outcome = HuaweiSUN2000CatalogueAmbiguous
			return notSelected(HuaweiSUN2000CatalogueMultipleCandidateRows, inScope)
		}
		return notSelected(HuaweiSUN2000CatalogueIdentifierAbsent, inScope)
	}
	var admitted, below []HuaweiSUN2000CatalogueRow
	for _, row := range inScope {
		row.FloorRelation = huaweiSUN2000FloorRelationOf(build, row.Floor)
		if row.FloorRelation == HuaweiSUN2000FloorBelow {
			below = append(below, row)
			continue
		}
		admitted = append(admitted, row)
	}
	switch {
	case len(admitted) == 0:
		return notSelected(HuaweiSUN2000CatalogueBelowFloor, below)
	case len(admitted) > 1:
		decision.outcome = HuaweiSUN2000CatalogueAmbiguous
		return notSelected(HuaweiSUN2000CatalogueMultipleCandidateRows, admitted)
	case !decision.identifierRead:
		return notSelected(HuaweiSUN2000CatalogueIdentifierAbsent, admitted)
	}
	selected := admitted[0]
	decision.outcome, decision.reason = HuaweiSUN2000CatalogueSelected, huaweiSUN2000SelectedReason(selected.FloorRelation)
	decision.selected, decision.candidates = &selected, []HuaweiSUN2000CatalogueRow{selected.clone()}
	return decision
}
