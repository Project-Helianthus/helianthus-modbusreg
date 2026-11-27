package modbusreg

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// packageImportRules pins the module packages each package's production files
// may import, keyed by package directory relative to the module root. The
// root owns the contracts and imports no module package; a vendor package
// imports only the root and never another vendor package; a vendor overlay
// may also import the standard family it extends, and a standard family never
// imports a vendor package. The data package profiles imports no module
// package; a vendor package may import it for the committed data it embeds.
// A test-support package under internal/ imports at most the root and the
// standard family it supports, and only where its row allows it. The SunSpec
// standard family imports only the root; the Fronius and OutBack overlays
// may import the root and the SunSpec standard family. The detection and
// ledger packages import only the root, never each other or a vendor
// package, and no vendor package imports either of them.
var packageImportRules = map[string][]string{
	".":                              {},
	"driverdata/certification":       {},
	"detection":                      {"."},
	"fronius":                        {".", "sunspec"},
	"growatt":                        {"."},
	"huawei":                         {".", "profiles"},
	"internal/qualifiedfunctiontest": {},
	"internal/sunspecfixture":        {".", "sunspec"},
	"ledger":                         {"."},
	"outback":                        {".", "sunspec"},
	"profiles":                       {},
	"sunspec":                        {"."},
	"tesla":                          {"."},
}

// driverLeafDirectory is the driver leaf: its packages need no row and may
// import every package of the module, and they alone may import the SemReg
// contract packages in driverLeafAllowedImports. It is the Go view of
// driver_leaf_directory and driver_leaf_allowed_import_prefixes in
// policy/registry-boundary.json.
const driverLeafDirectory = "driver"

const semregModulePath = "github.com/Project-Helianthus/helianthus-semreg"

var driverLeafAllowedImports = []string{
	semregModulePath + "/semreg/v1/driver",
	semregModulePath + "/semreg/v1/roweval",
	semregModulePath + "/semreg/v1",
	semregModulePath + "/semreg/v1/catalog",
	semregModulePath + "/semreg/v1/operation",
}

func TestPackageImportDirections(t *testing.T) {
	if findings := packageImportFindings(t, ".", packageImportRules, driverLeafDirectory, driverLeafAllowedImports); len(findings) != 0 {
		t.Fatalf("package import directions: %v", findings)
	}
}

// TestPackageImportDirectionsRejectEachViolation changes one input of a
// synthetic module at a time: a vendor importing a vendor, the root importing
// a vendor, a standard family importing a vendor or its overlay, a vendor or
// an overlay importing outside its row, a test-support package importing the
// root, a data package importing the root or a vendor, a top-level or nested
// package without a row and a row without a package. The driver leaf imports
// the root, a vendor and the exact SemReg contract packages without a finding,
// while a package outside the leaf importing a leaf package or any SemReg
// package beyond the exact paths is a finding. The unchanged module
// keeps an overlay importing its standard family, a vendor importing the data
// package, and test files, testdata and tools importing a vendor, which the
// production import rules do not cover.
func TestPackageImportDirectionsRejectEachViolation(t *testing.T) {
	rules := map[string][]string{
		".": {}, "alpha": {".", "data"}, "beta": {"."}, "standard": {"."},
		"overlay": {".", "standard"}, "internal/support": {}, "data": {},
	}
	base := map[string]string{
		"go.mod":                "module example.com/synthetic\n",
		"root.go":               "package synthetic\n",
		"alpha/a.go":            "package alpha\n\nimport (\n\t_ \"example.com/synthetic\"\n\t_ \"example.com/synthetic/data\"\n)\n",
		"beta/b.go":             "package beta\n\nimport _ \"example.com/synthetic\"\n",
		"beta/b_test.go":        "package beta\n\nimport _ \"example.com/synthetic/alpha\"\n",
		"standard/s.go":         "package standard\n\nimport _ \"example.com/synthetic\"\n",
		"overlay/o.go":          "package overlay\n\nimport (\n\t_ \"example.com/synthetic\"\n\t_ \"example.com/synthetic/standard\"\n)\n",
		"internal/support/s.go": "package support\n",
		"data/d.go":             "package data\n",
		"driver/x/x.go":         "package x\n",
		"testdata/t.go":         "package t\n\nimport _ \"example.com/synthetic/alpha\"\n",
		"tools/x/x.go":          "package main\n\nimport _ \"example.com/synthetic/alpha\"\n",
		"alpha/data/x.txt":      "not a package\n",
	}
	withRow := func(dir string, allowed ...string) map[string][]string {
		table := map[string][]string{}
		for key, value := range rules {
			table[key] = value
		}
		table[dir] = allowed
		return table
	}
	for name, test := range map[string]struct {
		file, text string
		rules      map[string][]string
		want       []string
	}{
		"unchanged":                                    {want: nil},
		"vendor imports vendor":                        {file: "beta/c.go", text: "package beta\n\nimport _ \"example.com/synthetic/alpha\"\n", want: []string{"beta imports alpha"}},
		"root imports vendor":                          {file: "root_extra.go", text: "package synthetic\n\nimport _ \"example.com/synthetic/beta\"\n", want: []string{". imports beta"}},
		"standard imports vendor":                      {file: "standard/v.go", text: "package standard\n\nimport _ \"example.com/synthetic/alpha\"\n", want: []string{"standard imports alpha"}},
		"standard imports overlay":                     {file: "standard/o.go", text: "package standard\n\nimport _ \"example.com/synthetic/overlay\"\n", want: []string{"standard imports overlay"}},
		"vendor imports standard":                      {file: "alpha/s.go", text: "package alpha\n\nimport _ \"example.com/synthetic/standard\"\n", want: []string{"alpha imports standard"}},
		"overlay imports vendor":                       {file: "overlay/v.go", text: "package overlay\n\nimport _ \"example.com/synthetic/alpha\"\n", want: []string{"overlay imports alpha"}},
		"test support imports root":                    {file: "internal/support/r.go", text: "package support\n\nimport _ \"example.com/synthetic\"\n", want: []string{"internal/support imports ."}},
		"data imports root":                            {file: "data/r.go", text: "package data\n\nimport _ \"example.com/synthetic\"\n", want: []string{"data imports ."}},
		"data imports vendor":                          {file: "data/v.go", text: "package data\n\nimport _ \"example.com/synthetic/alpha\"\n", want: []string{"data imports alpha"}},
		"vendor without data row":                      {rules: withRow("alpha", "."), want: []string{"alpha imports data"}},
		"package without row":                          {file: "gamma/g.go", text: "package gamma\n", want: []string{"gamma has no import row"}},
		"row without package":                          {rules: withRow("delta", "."), want: []string{"delta has no package"}},
		"nested package without row":                   {file: "alpha/inner/i.go", text: "package inner\n\nimport _ \"example.com/synthetic/alpha\"\n", want: []string{"alpha/inner has no import row"}},
		"internal package without row":                 {file: "internal/other/o.go", text: "package other\n", want: []string{"internal/other has no import row"}},
		"overlay row allows only its family":           {rules: withRow("overlay", "."), want: []string{"overlay imports standard"}},
		"driver leaf imports root vendor and standard": {file: "driver/x/x.go", text: "package x\n\nimport (\n\t_ \"example.com/synthetic\"\n\t_ \"example.com/synthetic/beta\"\n\t_ \"example.com/synthetic/standard\"\n)\n", want: nil},
		"driver leaf imports each contract path": {file: "driver/x/x.go", text: "package x\n\nimport (\n" +
			"\t_ \"" + semregModulePath + "/semreg/v1/driver\"\n" +
			"\t_ \"" + semregModulePath + "/semreg/v1/roweval\"\n" +
			"\t_ \"" + semregModulePath + "/semreg/v1\"\n" +
			"\t_ \"" + semregModulePath + "/semreg/v1/catalog\"\n" +
			"\t_ \"" + semregModulePath + "/semreg/v1/operation\"\n" +
			")\n", want: nil},
		"driver leaf imports non contract semreg paths": {file: "driver/x/x.go", text: "package x\n\nimport (\n" +
			"\t_ \"" + semregModulePath + "/semreg/v1/driverextra\"\n" +
			"\t_ \"" + semregModulePath + "/semreg/v1/rowevalextra\"\n" +
			"\t_ \"" + semregModulePath + "/semreg/v1/runtime\"\n" +
			"\t_ \"" + semregModulePath + "/semreg/v1/driver/extra\"\n" +
			")\n", want: []string{
			"driver/x imports " + semregModulePath + "/semreg/v1/driver/extra",
			"driver/x imports " + semregModulePath + "/semreg/v1/driverextra",
			"driver/x imports " + semregModulePath + "/semreg/v1/rowevalextra",
			"driver/x imports " + semregModulePath + "/semreg/v1/runtime",
		}},
		"alpha imports driver leaf": {file: "alpha/l.go", text: "package alpha\n\nimport _ \"example.com/synthetic/driver/x\"\n", want: []string{"alpha imports driver/x"}},
		"beta imports each contract path": {file: "beta/c.go", text: "package beta\n\nimport (\n" +
			"\t_ \"" + semregModulePath + "/semreg/v1\"\n" +
			"\t_ \"" + semregModulePath + "/semreg/v1/catalog\"\n" +
			"\t_ \"" + semregModulePath + "/semreg/v1/driver\"\n" +
			"\t_ \"" + semregModulePath + "/semreg/v1/operation\"\n" +
			"\t_ \"" + semregModulePath + "/semreg/v1/roweval\"\n" +
			")\n", want: []string{
			"beta imports " + semregModulePath + "/semreg/v1",
			"beta imports " + semregModulePath + "/semreg/v1/catalog",
			"beta imports " + semregModulePath + "/semreg/v1/driver",
			"beta imports " + semregModulePath + "/semreg/v1/operation",
			"beta imports " + semregModulePath + "/semreg/v1/roweval",
		}},
		"driver subpackage without row": {file: "driver/x/y/y.go", text: "package y\n\nimport _ \"example.com/synthetic/alpha\"\n", want: nil},
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			files := map[string]string{}
			for path, text := range base {
				files[path] = text
			}
			if test.file != "" {
				files[test.file] = test.text
			}
			for path, text := range files {
				target := filepath.Join(root, filepath.FromSlash(path))
				if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(target, []byte(text), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			table := rules
			if test.rules != nil {
				table = test.rules
			}
			if got := packageImportFindings(t, root, table, driverLeafDirectory, driverLeafAllowedImports); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("findings=%q want=%q", got, test.want)
			}
		})
	}
}

// packageImportFindings parses every Go package directory of the module at
// root, skipping hidden, underscore, testdata and top-level tools directories,
// and reports production imports that break the rules. A package at or below
// leaf needs no row and may import every module package, and the SemReg
// contract packages listed in leafImports only from there; every other
// SemReg import is a finding, and no package outside leaf may import a
// package at or below leaf.
func packageImportFindings(t *testing.T, root string, rules map[string][]string, leaf string, leafImports []string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	module := strings.TrimSpace(strings.TrimPrefix(strings.SplitN(string(data), "\n", 2)[0], "module "))
	imports := map[string]map[string]bool{}
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		name := entry.Name()
		if entry.IsDir() {
			if rel != "." && (strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") || name == "testdata" || rel == "tools") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(name, ".go") {
			return nil
		}
		dir := filepath.ToSlash(filepath.Dir(rel))
		if imports[dir] == nil {
			imports[dir] = map[string]bool{}
		}
		if strings.HasSuffix(name, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, spec := range file.Imports {
			imported, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return err
			}
			switch {
			case imported == module:
				imports[dir]["."] = true
			case strings.HasPrefix(imported, module+"/"):
				imports[dir][strings.TrimPrefix(imported, module+"/")] = true
			case imported == semregModulePath || strings.HasPrefix(imported, semregModulePath+"/"):
				imports[dir][imported] = true
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var findings []string
	for dir, used := range imports {
		inLeaf := directoryWithin(dir, leaf)
		allowed, ok := rules[dir]
		if !inLeaf && !ok {
			findings = append(findings, dir+" has no import row")
			continue
		}
		for imported := range used {
			switch {
			case imported == semregModulePath || strings.HasPrefix(imported, semregModulePath+"/"):
				if !inLeaf || !slices.Contains(leafImports, imported) {
					findings = append(findings, dir+" imports "+imported)
				}
			case directoryWithin(imported, leaf):
				if !inLeaf {
					findings = append(findings, dir+" imports "+imported)
				}
			case inLeaf:
				// The driver leaf may import every package of the module.
			case !slices.Contains(allowed, imported):
				findings = append(findings, dir+" imports "+imported)
			}
		}
	}
	for dir := range rules {
		if imports[dir] == nil {
			findings = append(findings, dir+" has no package")
		}
	}
	sort.Strings(findings)
	return findings
}

func directoryWithin(dir, leaf string) bool {
	return dir == leaf || strings.HasPrefix(dir, leaf+"/")
}
