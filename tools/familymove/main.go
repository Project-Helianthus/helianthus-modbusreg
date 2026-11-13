// Command familymove moves one vendor family out of the module root package
// into its own package directory, or checks a family before and after its move.
//
// The tool is offline. The go command lists export data for the root
// package's dependencies, and go/types resolves every reference, so a moved
// file's references to the root are qualified, references to the family are
// redirected and the family prefix is stripped by object identity, never by
// text. Usage:
//
//	familymove -family growatt -check   # report blockers, or verify a finished move
//	familymove -family growatt -apply   # move the family and print the rename table
package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// family is one vendor family's explicit move rule.
type family struct {
	name   string   // package name and directory of the family
	prefix string   // exported-name prefix stripped by the move
	files  []string // exact root files moved; empty selects by stems
	stems  []string // file-name prefixes selecting root files when files is empty
	keep   []string // root external tests that stay at the root
}

// growattFiles is the exact Growatt move set: production files and the
// in-package tests. The three external tests in keep share external fixtures
// with the root detector contract tests and stay at the root.
var growattFiles = []string{
	"growatt_aggregate_offline_provider.go",
	"growatt_aggregate_provider_port.go",
	"growatt_aggregate_retention.go",
	"growatt_bms_rs485.go",
	"growatt_bms_rs485_multigroup.go",
	"growatt_bms_rs485_rtu_observer.go",
	"growatt_bms_rs485_typed.go",
	"growatt_diagnostic_commit.go",
	"growatt_fc04_record_validation.go",
	"growatt_protocol_ii_fc04.go",
	"growatt_protocol_ii_fc04_rtu_observer.go",
	"growatt_protocol_ii_identity.go",
	"growatt_protocol_ii_rtu_observer.go",
	"growatt_aggregate_offline_provider_test.go",
	"growatt_aggregate_provider_port_test.go",
	"growatt_aggregate_retention_test.go",
	"growatt_bms_rs485_multigroup_test.go",
	"growatt_bms_rs485_rtu_observer_test.go",
	"growatt_bms_rs485_test.go",
	"growatt_bms_rs485_typed_test.go",
	"growatt_diagnostic_commit_test.go",
	"growatt_disposition_test.go",
	"growatt_exact_profile_internal_test.go",
	"growatt_fixture_flag_followup_test.go",
	"growatt_protocol_ii_fc04_rtu_observer_test.go",
	"growatt_protocol_ii_fc04_test.go",
	"growatt_protocol_ii_identity_test.go",
	"growatt_protocol_ii_rtu_observer_test.go",
}

// families lists every vendor family. Families after Growatt select by file
// stems until their own slice pins an exact list; -check reports what blocks
// their move.
var families = []family{
	{name: "growatt", prefix: "Growatt", files: growattFiles, keep: []string{
		"growatt_exact_profile_negative_test.go",
		"growatt_fc04_record_validation_test.go",
		"growatt_protocol_ii_fc04_public_api_test.go",
	}},
	{name: "tesla", prefix: "Tesla", stems: []string{"tesla_"}},
	{name: "huawei", prefix: "Huawei", stems: []string{"huawei_", "sun2000_", "luna_", "smartlogger_", "sdongle_", "emma_"}},
	{name: "sunspec", prefix: "SunSpec", stems: []string{"sunspec_"}},
	{name: "fronius", prefix: "Fronius", stems: []string{"fronius_"}},
	{name: "outback", prefix: "OutBack", stems: []string{"outback_"}},
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, families))
}

// run parses the command line and returns the process exit status: 0 on
// success, 1 when a check finds blockers or a move fails, 2 on usage errors.
func run(args []string, stdout, stderr io.Writer, table []family) int {
	flags := flag.NewFlagSet("familymove", flag.ContinueOnError)
	flags.SetOutput(stderr)
	name := flags.String("family", "", "vendor family to move or check")
	dir := flags.String("dir", ".", "module root directory")
	apply := flags.Bool("apply", false, "move the family into its package")
	check := flags.Bool("check", false, "report blockers, or verify a finished move")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	selected, ok := lookupFamily(table, *name)
	if !ok || *apply == *check || flags.NArg() != 0 {
		_, _ = fmt.Fprintln(stderr, "usage: familymove -family NAME (-check | -apply) [-dir DIR]")
		return 2
	}
	root, err := filepath.Abs(*dir)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	if *check {
		return checkFamily(root, selected, stdout, stderr)
	}
	return applyFamily(root, selected, stdout, stderr)
}

func lookupFamily(table []family, name string) (family, bool) {
	for _, candidate := range table {
		if candidate.name == name {
			return candidate, true
		}
	}
	return family{}, false
}

// selection returns the root files the family rule selects, sorted.
func (f family) selection(root string) ([]string, error) {
	keep := map[string]bool{}
	for _, name := range f.keep {
		keep[name] = true
	}
	if len(f.files) > 0 {
		selected := append([]string(nil), f.files...)
		sort.Strings(selected)
		return selected, nil
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var selected []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || keep[name] {
			continue
		}
		for _, stem := range f.stems {
			if strings.HasPrefix(name, stem) {
				selected = append(selected, name)
				break
			}
		}
	}
	return selected, nil
}

// checkFamily verifies a finished move when the family directory holds Go
// files, and otherwise reports every reference that blocks the move.
func checkFamily(root string, f family, stdout, stderr io.Writer) int {
	moved, err := hasGoFiles(filepath.Join(root, f.name))
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	var findings []string
	if moved {
		findings, err = movedFindings(root, f)
	} else {
		var p *plan
		p, err = analyze(root, f)
		if p != nil {
			findings = p.blockerList()
		}
	}
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	for _, finding := range findings {
		_, _ = fmt.Fprintln(stdout, finding)
	}
	state := "not moved"
	if moved {
		state = "moved"
	}
	_, _ = fmt.Fprintf(stdout, "family %s: %s, %d findings\n", f.name, state, len(findings))
	if len(findings) > 0 {
		return 1
	}
	return 0
}

// applyFamily performs the move after an analysis without blockers and
// prints the summary and the CHANGELOG rename table.
func applyFamily(root string, f family, stdout, stderr io.Writer) int {
	p, err := analyze(root, f)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	if blockers := p.blockerList(); len(blockers) > 0 {
		for _, blocker := range blockers {
			_, _ = fmt.Fprintln(stderr, blocker)
		}
		_, _ = fmt.Fprintf(stderr, "family %s: %d blockers, nothing moved\n", f.name, len(blockers))
		return 1
	}
	summary, err := p.write()
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	for _, line := range summary {
		_, _ = fmt.Fprintln(stdout, line)
	}
	_, _ = fmt.Fprint(stdout, renameTable(p))
	return 0
}

func hasGoFiles(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".go") {
			return true, nil
		}
	}
	return false, nil
}

// movedFindings verifies a finished move: no selected file is left at the
// root, every listed file is in the family directory, and no root file
// declares a family name or reaches one through the root package.
func movedFindings(root string, f family) ([]string, error) {
	listed, err := listRoot(root)
	if err != nil {
		return nil, err
	}
	selected, err := f.selection(root)
	if err != nil {
		return nil, err
	}
	var findings []string
	for _, name := range selected {
		if _, err := os.Stat(filepath.Join(root, name)); err == nil {
			findings = append(findings, "left at the root: "+name)
		} else if _, err := os.Stat(filepath.Join(root, f.name, name)); err != nil {
			findings = append(findings, "missing from "+f.name+"/: "+name)
		}
	}
	old, err := familyNames(filepath.Join(root, f.name), f.prefix)
	if err != nil {
		return nil, err
	}
	paths, err := filepath.Glob(filepath.Join(root, "*.go"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	fset := token.NewFileSet()
	for _, path := range paths {
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return nil, err
		}
		name := filepath.Base(path)
		for _, declared := range declaredNames(file) {
			if old[declared] {
				findings = append(findings, fmt.Sprintf("%s declares family name %s", name, declared))
			}
		}
		for _, spec := range file.Imports {
			if path, _ := strconv.Unquote(spec.Path.Value); path == listed.ImportPath+"/"+f.name && file.Name.Name == listed.Name {
				findings = append(findings, fmt.Sprintf("root package file %s imports the family package", name))
			}
		}
		for _, selected := range rootSelectors(file, listed.ImportPath, listed.Name) {
			if old[selected] {
				findings = append(findings, fmt.Sprintf("%s refers to %s.%s", name, listed.Name, selected))
			}
		}
	}
	return findings, nil
}

// familyNames returns every name a family declaration may have had at the
// root: its current name, the prefixed name and the prefixed constructor.
func familyNames(dir, prefix string) (map[string]bool, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		return nil, err
	}
	names := map[string]bool{}
	fset := token.NewFileSet()
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return nil, err
		}
		for _, name := range declaredNames(file) {
			if token.IsExported(name) {
				names[name], names[prefix+name] = true, true
				if rest, ok := strings.CutPrefix(name, "New"); ok {
					names["New"+prefix+rest] = true
				}
			}
		}
	}
	return names, nil
}

// declaredNames lists the package-level names a file declares.
func declaredNames(file *ast.File) []string {
	var names []string
	for _, decl := range file.Decls {
		switch node := decl.(type) {
		case *ast.FuncDecl:
			if node.Recv == nil {
				names = append(names, node.Name.Name)
			}
		case *ast.GenDecl:
			for _, spec := range node.Specs {
				switch spec := spec.(type) {
				case *ast.TypeSpec:
					names = append(names, spec.Name.Name)
				case *ast.ValueSpec:
					for _, id := range spec.Names {
						names = append(names, id.Name)
					}
				}
			}
		}
	}
	return names
}

// rootSelectors returns the names a file selects through its import of the
// module root package.
func rootSelectors(file *ast.File, module, rootName string) []string {
	local := ""
	for _, spec := range file.Imports {
		if path, _ := strconv.Unquote(spec.Path.Value); path == module {
			local = rootName
			if spec.Name != nil {
				local = spec.Name.Name
			}
		}
	}
	var names []string
	ast.Inspect(file, func(n ast.Node) bool {
		if selector, ok := n.(*ast.SelectorExpr); ok && local != "" {
			if x, ok := selector.X.(*ast.Ident); ok && x.Name == local {
				names = append(names, selector.Sel.Name)
			}
		}
		return true
	})
	return names
}

// scanModule blocks references to moved names from module packages outside
// the root directory; the move rewrites only the root package and its tests.
func (p *plan) scanModule() error {
	fset := token.NewFileSet()
	family := filepath.Join(p.root, p.fam.name)
	return filepath.WalkDir(p.root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := entry.Name()
		if entry.IsDir() {
			if path != p.root && (strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") || name == "testdata" || path == family) {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Dir(path) == p.root || !strings.HasSuffix(name, ".go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		for _, selected := range rootSelectors(file, p.module, p.rootName) {
			if p.movedNames[selected] {
				rel, _ := filepath.Rel(p.root, path)
				p.block("%s refers to %s.%s; packages outside the root are not rewritten", filepath.ToSlash(rel), p.rootName, selected)
			}
		}
		return nil
	})
}

// markdownOutputs rewrites README.md and docs/*.md: renamed identifiers,
// links to moved files and focused go test commands of moved tests.
func (p *plan) markdownOutputs() ([]output, error) {
	paths, err := filepath.Glob(filepath.Join(p.root, "docs", "*.md"))
	if err != nil {
		return nil, err
	}
	paths = append([]string{filepath.Join(p.root, "README.md")}, paths...)
	var moved []string
	for name := range p.movedPaths() {
		moved = append(moved, regexp.QuoteMeta(name))
	}
	sort.Sort(sort.Reverse(sort.StringSlice(moved)))
	links := regexp.MustCompile(`(\.\./|/blob/[^/\s()]+/|/tree/[^/\s()]+/)(` + strings.Join(moved, "|") + `)\b`)
	commands := regexp.MustCompile(`go test \. (-run '([^']*)')`)
	var outputs []output
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		} else if err != nil {
			return nil, err
		}
		text := p.renameText(string(data), "", p.fam.name+".", map[string]bool{p.rootName: true})
		if len(moved) > 0 {
			text = links.ReplaceAllString(text, "${1}"+p.fam.name+"/${2}")
		}
		text = commands.ReplaceAllStringFunc(text, func(command string) string {
			if p.onlyMovedTests(commands.FindStringSubmatch(command)[2]) {
				return "go test ./" + p.fam.name + command[len("go test ."):]
			}
			return command
		})
		if text != string(data) {
			rel, _ := filepath.Rel(p.root, path)
			outputs = append(outputs, output{path: filepath.ToSlash(rel), from: filepath.ToSlash(rel), data: []byte(text), note: "rewritten"})
		}
	}
	return outputs, nil
}

// onlyMovedTests reports whether pattern selects at least one test and only
// tests declared in moved files.
func (p *plan) onlyMovedTests(pattern string) bool {
	expression, err := regexp.Compile(pattern)
	if err != nil {
		return false
	}
	matched := false
	for name, moved := range p.tests {
		if expression.MatchString(name) {
			if !moved {
				return false
			}
			matched = true
		}
	}
	return matched
}

// renameText rewrites renamed identifiers in prose: a bare name gets bare as
// prefix, a name qualified by one of qualifiers gets qualified instead, and a
// name with any other qualifier is left alone.
func (p *plan) renameText(text, bare, qualified string, qualifiers map[string]bool) string {
	if len(p.renames) == 0 {
		return text
	}
	var names []string
	for name := range p.renames {
		names = append(names, regexp.QuoteMeta(name))
	}
	sort.Slice(names, func(i, j int) bool {
		return len(names[i]) > len(names[j]) || len(names[i]) == len(names[j]) && names[i] < names[j]
	})
	pattern := regexp.MustCompile(`\b(?:([A-Za-z_][A-Za-z0-9_]*)\.)?(` + strings.Join(names, "|") + `)\b`)
	return pattern.ReplaceAllStringFunc(text, func(match string) string {
		parts := pattern.FindStringSubmatch(match)
		switch {
		case parts[1] == "":
			return bare + p.renames[parts[2]]
		case qualifiers[parts[1]]:
			return qualified + p.renames[parts[2]]
		}
		return match
	})
}
