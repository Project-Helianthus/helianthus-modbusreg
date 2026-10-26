package main

// Schema identifiers of the committed catalogue files.
const (
	editionsSchema     = "helianthus-modbusreg-huawei-catalogue-editions/v1"
	modelsSchema       = "helianthus-modbusreg-huawei-catalogue-models/v1"
	registersSchema    = "helianthus-modbusreg-huawei-catalogue-registers/v1"
	dictionariesSchema = "helianthus-modbusreg-huawei-catalogue-dictionaries/v1"
	quarantinesSchema  = "helianthus-modbusreg-huawei-catalogue-quarantines/v1"
)

// Register row classes. Only reviewed and conditional rows carry decode
// fields; raw-only and conflict rows keep their literal cells and never decode.
const (
	classReviewed    = "reviewed"
	classConditional = "conditional"
	classRawOnly     = "raw-only"
	classConflict    = "conflict"
)

// Control row classes. A readable-configuration row may be read with FC03 as
// raw evidence; a no-send row is never requested. Control rows never decode.
const (
	classReadableConfiguration = "readable-configuration"
	classNoSend                = "no-send"
)

// Model floor bases and literals.
const (
	floorBasisRow       = "row"
	floorBasisDocument  = "document"
	floorNotStated      = "not stated"
	sourceClassVendor   = "vendor_edition"
	sourceClassMirrored = "mirrored_edition"
	addressUnspecified  = "unspecified_in_source"
)

// Endpoints of a register row. EMMA rows never enter an inverter read map.
const (
	endpointInverter      = "inverter"
	endpointEMMA          = "emma"
	endpointEMMAAssociate = "emma-associated-device"
)

const attributionNote = "Register, model and dictionary facts are taken from an independently " +
	"prepared reference whose original tables, keys and labels are dedicated under CC0-1.0. " +
	"Vendor publications keep their own rights; the catalogue copies no vendor text, label " +
	"table, link, locator or digest, and it names each consulted edition by title, issue and date only."

type editionsDoc struct {
	Schema            string    `json:"schema"`
	ReferenceBaseline string    `json:"reference_baseline"`
	Attribution       string    `json:"attribution"`
	Editions          []edition `json:"editions"`
}

type edition struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Issue       string `json:"issue"`
	Date        string `json:"date"`
	Copy        int    `json:"copy,omitempty"`
	Family      string `json:"family,omitempty"`
	Branch      string `json:"branch,omitempty"`
	SourceClass string `json:"source_class"`
	Alias       string `json:"alias,omitempty"`
	Card        string `json:"card,omitempty"`
	ModelRows   int    `json:"model_rows"`
	// The fields below are written for the gateway editions only.
	Models              []string        `json:"models,omitempty"`
	ModelListExhaustive *bool           `json:"model_list_exhaustive,omitempty"`
	Floors              []softwareFloor `json:"floors,omitempty"`
	FloorPages          []int           `json:"floor_pages,omitempty"`
	SelfUnit            *int            `json:"self_unit,omitempty"`
	Namespaces          []namespaceRef  `json:"namespaces,omitempty"`
	RegisterRows        int             `json:"register_rows,omitempty"`
}

type modelsDoc struct {
	Schema               string                `json:"schema"`
	DocumentRequirements []documentRequirement `json:"document_requirements"`
	Rows                 []modelRow            `json:"rows"`
}

type documentRequirement struct {
	ID          string   `json:"id"`
	Edition     string   `json:"edition"`
	Page        int      `json:"page"`
	Section     string   `json:"section"`
	Minimum     string   `json:"minimum"`
	Inclusive   bool     `json:"inclusive"`
	Component   string   `json:"component"`
	ModelTokens []string `json:"model_tokens"`
}

type modelRow struct {
	Edition             string      `json:"edition"`
	Page                int         `json:"page"`
	Token               string      `json:"token"`
	Identifier          *int        `json:"identifier,omitempty"`
	SequenceNumber      string      `json:"sequence_number,omitempty"`
	RowMinimum          string      `json:"row_minimum"`
	DocumentRequirement string      `json:"document_requirement,omitempty"`
	MinimumSoftware     string      `json:"minimum_software"`
	FloorBasis          string      `json:"floor_basis"`
	WireAliases         []wireAlias `json:"wire_aliases,omitempty"`
	Quarantine          []string    `json:"quarantine,omitempty"`
}

type wireAlias struct {
	Token    string `json:"token"`
	Evidence string `json:"evidence"`
	Scope    string `json:"scope"`
}

type registersDoc struct {
	Schema            string            `json:"schema"`
	Card              string            `json:"card"`
	Edition           string            `json:"edition"`
	ModelPages        []int             `json:"model_pages"`
	SelectionNote     string            `json:"selection_note,omitempty"`
	Gates             []gate            `json:"gates,omitempty"`
	FunctionalClasses []functionalClass `json:"functional_classes,omitempty"`
	ReadExtents       []readExtent      `json:"read_extents"`
	AdditiveSupport   *additiveSupport  `json:"additive_support,omitempty"`
	Rows              []registerRow     `json:"rows"`
	Controls          []registerRow     `json:"controls"`
}

type readExtent struct {
	Start          int    `json:"start"`
	End            int    `json:"end"`
	Quantity       int    `json:"quantity"`
	SafeQuantities []int  `json:"safe_quantities"`
	Offsets        []int  `json:"offsets"`
	FollowingHole  []int  `json:"following_hole,omitempty"`
	Hole           string `json:"hole,omitempty"`
}

type additiveSupport struct {
	Edition string        `json:"edition"`
	Token   string        `json:"token"`
	Model   int           `json:"identifier"`
	Minimum string        `json:"minimum_software"`
	Page    int           `json:"page"`
	Rows    []additiveRow `json:"rows"`
	Note    string        `json:"condition,omitempty"`
}

type additiveRow struct {
	Offset   int    `json:"offset"`
	Quantity int    `json:"quantity"`
	Type     string `json:"type"`
	Meaning  string `json:"meaning"`
	Page     int    `json:"page"`
}

type gate struct {
	ID       string `json:"id"`
	Meaning  string `json:"meaning"`
	Pages    []int  `json:"pages,omitempty"`
	Kind     string `json:"kind,omitempty"`
	Operator string `json:"operator,omitempty"`
	Value    string `json:"value,omitempty"`
}

type functionalClass struct {
	ID    string `json:"id"`
	Class string `json:"class,omitempty"`
	Rule  string `json:"rule,omitempty"`
}

type registerRow struct {
	Key             string   `json:"key"`
	Endpoint        string   `json:"endpoint"`
	Offset          *int     `json:"offset,omitempty"`
	AddressStatus   string   `json:"address_status,omitempty"`
	Quantity        int      `json:"quantity"`
	Type            string   `json:"type"`
	Access          string   `json:"access"`
	Class           string   `json:"class"`
	FunctionalClass string   `json:"functional_class,omitempty"`
	Decode          *scale   `json:"decode,omitempty"`
	Literal         *scale   `json:"literal,omitempty"`
	Gates           []string `json:"gates,omitempty"`
	Condition       string   `json:"condition,omitempty"`
	Core            *coreRef `json:"core,omitempty"`
	Page            int      `json:"page"`
	Occurrence      int      `json:"occurrence,omitempty"`
	Quarantine      []string `json:"quarantine,omitempty"`
}

type scale struct {
	Gain string `json:"gain"`
	Unit string `json:"unit"`
}

type coreRef struct {
	Key  string `json:"key"`
	Gate string `json:"gate"`
}

type dictionariesDoc struct {
	Schema       string       `json:"schema"`
	Dictionaries []dictionary `json:"dictionaries"`
}

type dictionary struct {
	ID            string      `json:"id"`
	Kind          string      `json:"kind"`
	Card          string      `json:"card"`
	Edition       string      `json:"edition"`
	Endpoint      string      `json:"endpoint"`
	Offsets       []int       `json:"offsets"`
	Pages         []int       `json:"pages"`
	ModePredicate string      `json:"mode_predicate"`
	Values        []dictValue `json:"values,omitempty"`
	Modules       []dictValue `json:"modules,omitempty"`
	Bits          []alarmBit  `json:"bits,omitempty"`
	Narratives    []alarmNote `json:"narratives,omitempty"`
	Unlisted      string      `json:"unlisted"`
}

type dictValue struct {
	Code    string `json:"code"`
	Meaning string `json:"meaning"`
}

type alarmBit struct {
	Offset   *int   `json:"offset,omitempty"`
	Word     string `json:"word,omitempty"`
	Bit      int    `json:"bit"`
	AlarmID  int    `json:"alarm_id"`
	Cause    *int   `json:"cause_id,omitempty"`
	Label    string `json:"label"`
	Severity string `json:"severity"`
	Module   string `json:"module"`
	Page     int    `json:"page"`
	// Condition and Quarantine are written for the gateway alarm tuples only.
	Condition  string   `json:"condition,omitempty"`
	Quarantine []string `json:"quarantine,omitempty"`
}

type quarantinesDoc struct {
	Schema      string       `json:"schema"`
	Quarantines []quarantine `json:"quarantines"`
}

type quarantine struct {
	ID       string        `json:"id"`
	Origin   string        `json:"origin"`
	Subject  string        `json:"subject"`
	Evidence string        `json:"evidence,omitempty"`
	Meaning  string        `json:"meaning"`
	Editions []string      `json:"editions"`
	Affected []affectedRow `json:"affected"`
}

type affectedRow struct {
	Set        string `json:"set"`
	Card       string `json:"card,omitempty"`
	Edition    string `json:"edition,omitempty"`
	Offset     *int   `json:"offset,omitempty"`
	Key        string `json:"key,omitempty"`
	Occurrence int    `json:"occurrence,omitempty"`
	Token      string `json:"token,omitempty"`
	Identifier *int   `json:"identifier,omitempty"`
	Page       int    `json:"page,omitempty"`
}
