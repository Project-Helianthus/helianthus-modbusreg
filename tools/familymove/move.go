package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/build"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// dataRoots are the root-relative data directories a moved file reaches
// through a string literal; the move prefixes such literals with "../".
var dataRoots = []string{"profiles", "testdata", "docs"}

// source is one parsed Go file of the root package directory.
type source struct {
	name     string
	file     *ast.File
	text     []byte
	moved    bool // selected by the family rule
	external bool // package <root>_test
	test     bool // name ends in _test.go
	edits    []edit
	imports  []string        // import paths the rewrite adds
	locals   map[string]bool // names declared anywhere in the file
	rootUses int             // selectors through the root import
	redirect int             // of which redirected to the family package
}

// edit replaces text[start:end] of one source.
type edit struct {
	start, end int
	text       string
}

// plan is the analysed move of one family.
type plan struct {
	root, module, rootName string
	fam                    family
	fset                   *token.FileSet
	sources                map[string]*source
	order                  []string
	pkg                    *types.Package
	info                   *types.Info
	renames                map[string]string // old package-level name -> new
	movedNames             map[string]bool   // exported package-level names of moved files
	tests                  map[string]bool   // test function -> declared in a moved file
	blockers               map[string]bool
}

// listing is the go list record of the root package.
type listing struct {
	ImportPath, Name                                   string
	GoFiles, TestGoFiles, XTestGoFiles, IgnoredGoFiles []string
}

// analyze loads and type-checks the root package with its tests and records
// every rename, edit and blocker of the family move without writing.
func analyze(root string, f family) (*plan, error) {
	listed, exports, err := goList(root)
	if err != nil {
		return nil, err
	}
	if listed.Name == f.name {
		return nil, fmt.Errorf("family %s has the root package name", f.name)
	}
	p := &plan{root: root, module: listed.ImportPath, rootName: listed.Name, fam: f,
		fset: token.NewFileSet(), sources: map[string]*source{}, renames: map[string]string{},
		movedNames: map[string]bool{}, tests: map[string]bool{}, blockers: map[string]bool{},
		info: &types.Info{Defs: map[*ast.Ident]types.Object{}, Uses: map[*ast.Ident]types.Object{}}}
	selected, err := f.selection(root)
	if err != nil {
		return nil, err
	}
	if err := p.parse(listed, selected); err != nil {
		return nil, err
	}
	for _, name := range listed.IgnoredGoFiles {
		p.block("build-constrained root file is not analysed: %s", name)
	}
	gc := importer.ForCompiler(p.fset, "gc", func(path string) (io.ReadCloser, error) {
		if file, ok := exports[path]; ok {
			return os.Open(file)
		}
		return nil, fmt.Errorf("no export data for %s", path)
	})
	internal := append(append([]string(nil), listed.GoFiles...), listed.TestGoFiles...)
	if p.pkg, err = p.check(p.module, internal, gc); err != nil {
		return nil, err
	}
	if _, err = p.check(p.module+"_test", listed.XTestGoFiles, &rootImporter{p, gc, map[string]*types.Package{}}); err != nil {
		return nil, err
	}
	p.computeRenames()
	for _, name := range p.order {
		p.scan(p.sources[name])
	}
	if err := p.scanModule(); err != nil {
		return nil, err
	}
	return p, nil
}

// rootImporter resolves the root package to the type-checked source package
// and type-checks other module packages, such as a family moved earlier, from
// source against it, so external tests see one identity for every root type.
type rootImporter struct {
	p      *plan
	next   types.Importer
	module map[string]*types.Package
}

func (r *rootImporter) Import(path string) (*types.Package, error) {
	if path == r.p.module {
		return r.p.pkg, nil
	}
	rel, ok := strings.CutPrefix(path, r.p.module+"/")
	if !ok {
		return r.next.Import(path)
	}
	if pkg, ok := r.module[path]; ok {
		return pkg, nil
	}
	listed, err := build.Default.ImportDir(filepath.Join(r.p.root, filepath.FromSlash(rel)), 0)
	if err != nil {
		return nil, err
	}
	files := make([]*ast.File, 0, len(listed.GoFiles))
	for _, name := range listed.GoFiles {
		file, err := parser.ParseFile(r.p.fset, filepath.Join(listed.Dir, name), nil, 0)
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}
	pkg, err := (&types.Config{Importer: r}).Check(path, r.p.fset, files, nil)
	if err != nil {
		return nil, err
	}
	r.module[path] = pkg
	return pkg, nil
}

// listRoot returns the go list record of the root package.
func listRoot(root string) (listing, error) {
	var listed listing
	out, err := goCommand(root, "list", "-json", ".")
	if err != nil {
		return listed, err
	}
	return listed, json.Unmarshal(out, &listed)
}

// goList returns the root package record and the export data file of every
// package the root package and its tests depend on.
func goList(root string) (listing, map[string]string, error) {
	listed, err := listRoot(root)
	if err != nil {
		return listed, nil, err
	}
	out, err := goCommand(root, "list", "-export", "-deps", "-test", "-f", "{{.ImportPath}}\t{{.Export}}", ".")
	if err != nil {
		return listed, nil, err
	}
	exports := map[string]string{}
	for _, line := range strings.Split(string(out), "\n") {
		path, file, ok := strings.Cut(line, "\t")
		if ok && file != "" && !strings.Contains(path, " ") {
			exports[path] = file
		}
	}
	return listed, exports, nil
}

func goCommand(dir string, args ...string) ([]byte, error) {
	command := exec.Command("go", args...)
	command.Dir = dir
	var stderr bytes.Buffer
	command.Stderr = &stderr
	out, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("go %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

func (p *plan) parse(listed listing, selected []string) error {
	moved := map[string]bool{}
	for _, name := range selected {
		moved[name] = true
	}
	groups := [][]string{listed.GoFiles, listed.TestGoFiles, listed.XTestGoFiles}
	for index, names := range groups {
		for _, name := range names {
			text, err := os.ReadFile(filepath.Join(p.root, name))
			if err != nil {
				return err
			}
			file, err := parser.ParseFile(p.fset, filepath.Join(p.root, name), text, parser.ParseComments)
			if err != nil {
				return err
			}
			p.sources[name] = &source{name: name, file: file, text: text, moved: moved[name],
				external: index == 2, test: index > 0, locals: map[string]bool{}}
			p.order = append(p.order, name)
			delete(moved, name)
		}
	}
	for name := range moved {
		return fmt.Errorf("selected file %s is not a file of the root package", name)
	}
	sort.Strings(p.order)
	return nil
}

func (p *plan) check(path string, names []string, imp types.Importer) (*types.Package, error) {
	files := make([]*ast.File, 0, len(names))
	for _, name := range names {
		files = append(files, p.sources[name].file)
	}
	var first error
	config := types.Config{Importer: imp, Error: func(err error) {
		if first == nil {
			first = err
		}
	}}
	pkg, _ := config.Check(path, p.fset, files, p.info)
	if first != nil {
		return nil, fmt.Errorf("type-check %s: %v", path, first)
	}
	return pkg, nil
}

func (p *plan) block(format string, args ...any) {
	p.blockers[fmt.Sprintf(format, args...)] = true
}

// blockerList returns the blockers sorted.
func (p *plan) blockerList() []string {
	list := make([]string, 0, len(p.blockers))
	for blocker := range p.blockers {
		list = append(list, blocker)
	}
	sort.Strings(list)
	return list
}

// sourceOf returns the root source declaring obj, or nil for other packages.
func (p *plan) sourceOf(obj types.Object) *source {
	if obj == nil || !obj.Pos().IsValid() {
		return nil
	}
	file := p.fset.Position(obj.Pos()).Filename
	if filepath.Dir(file) != p.root {
		return nil
	}
	return p.sources[filepath.Base(file)]
}

func (p *plan) offset(pos token.Pos) int {
	return p.fset.Position(pos).Offset
}

func (p *plan) packageLevel(obj types.Object) bool {
	return obj.Parent() != nil && obj.Parent() == obj.Pkg().Scope()
}

// strip removes the family prefix when an upper-case letter follows it.
func (p *plan) strip(name string) (string, bool) {
	rest, ok := strings.CutPrefix(name, p.fam.prefix)
	if !ok || rest == "" {
		return "", false
	}
	first, _ := utf8.DecodeRuneInString(rest)
	return rest, unicode.IsUpper(first)
}

// computeRenames strips the prefix from exported package-level names of the
// moved files, lets New<Type> constructors follow their types and blocks on
// any collision in the family package.
func (p *plan) computeRenames() {
	scope := p.pkg.Scope()
	for _, name := range scope.Names() {
		if s := p.sourceOf(scope.Lookup(name)); s != nil && s.moved && token.IsExported(name) {
			p.movedNames[name] = true
			if target, ok := p.strip(name); ok {
				p.renames[name] = target
			}
		}
	}
	for name := range p.movedNames {
		rest, ok := strings.CutPrefix(name, "New")
		_, function := scope.Lookup(name).(*types.Func)
		_, typeName := scope.Lookup(rest).(*types.TypeName)
		if target, renamed := p.renames[rest]; ok && function && typeName && renamed {
			p.renames[name] = "New" + target
		}
	}
	owner := map[string]string{p.rootName: "the root import"}
	for _, name := range scope.Names() {
		if s := p.sourceOf(scope.Lookup(name)); s == nil || !s.moved {
			continue
		}
		target := p.finalName(name)
		if previous, taken := owner[target]; taken {
			p.block("rename collision: %s and %s both become %s", previous, name, target)
		}
		owner[target] = name
	}
}

func (p *plan) finalName(name string) string {
	if target, ok := p.renames[name]; ok {
		return target
	}
	return name
}

// docTarget maps a go doc symbol argument naming a moved type, or a
// constructor of one, to its name in the family package.
func (p *plan) docTarget(symbol string) (string, bool) {
	if target, ok := p.renames[symbol]; ok {
		return target, true
	}
	rest, ok := strings.CutPrefix(symbol, "New")
	_, typeName := p.pkg.Scope().Lookup(rest).(*types.TypeName)
	if target, renamed := p.renames[rest]; ok && typeName && renamed {
		return "New" + target, true
	}
	return "", false
}

func (p *plan) packageClause(s *source) string {
	if s.external {
		return p.fam.name + "_test"
	}
	return p.fam.name
}

// scan records the edits and blockers of one source file.
func (p *plan) scan(s *source) {
	if s.moved {
		s.edits = append(s.edits, edit{p.offset(s.file.Name.Pos()), p.offset(s.file.Name.End()), p.packageClause(s)})
	}
	for _, decl := range s.file.Decls {
		if function, ok := decl.(*ast.FuncDecl); ok && s.test && function.Recv == nil && strings.HasPrefix(function.Name.Name, "Test") {
			p.tests[function.Name.Name] = s.moved
		}
	}
	ast.Inspect(s.file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.SelectorExpr:
			return !p.selector(s, node)
		case *ast.Ident:
			p.ident(s, node)
		case *ast.FuncDecl:
			p.receiver(s, node)
		case *ast.StructType:
			p.embedded(s, node)
		case *ast.CallExpr:
			p.goDoc(s, node)
		case *ast.BasicLit:
			p.dataPath(s, node)
		}
		return true
	})
	for _, group := range s.file.Comments {
		for _, comment := range group.List {
			if s.moved && strings.HasPrefix(comment.Text, "//go:embed") {
				p.block("%s: go:embed paths are not relocated", s.name)
			}
			bare, qualified := p.fam.name+".", p.fam.name+"."
			if s.moved {
				bare, qualified = "", ""
			}
			if text := p.renameText(comment.Text, bare, qualified, p.rootNames(s)); text != comment.Text {
				s.edits = append(s.edits, edit{p.offset(comment.Pos()), p.offset(comment.End()), text})
			}
		}
	}
	p.imports(s)
}

// selector redirects root-qualified references of external tests to moved
// declarations; it reports whether the selector was rewritten.
func (p *plan) selector(s *source, node *ast.SelectorExpr) bool {
	x, ok := node.X.(*ast.Ident)
	if !ok {
		return false
	}
	if name, ok := p.info.Uses[x].(*types.PkgName); !ok || name.Imported().Path() != p.module {
		return false
	}
	s.rootUses++
	obj := p.info.Uses[node.Sel]
	declared := p.sourceOf(obj)
	switch {
	case declared == nil:
		return false
	case declared.test && declared.moved != s.moved:
		p.block("%s uses %s declared in test file %s", s.name, obj.Name(), declared.name)
	case declared.moved:
		s.redirect++
		s.imports = append(s.imports, p.module+"/"+p.fam.name)
		s.edits = append(s.edits, edit{p.offset(node.Pos()), p.offset(node.End()), p.fam.name + "." + p.finalName(obj.Name())})
		return true
	}
	return false
}

// ident renames, qualifies or blocks one identifier.
func (p *plan) ident(s *source, id *ast.Ident) {
	obj, defined := p.info.Defs[id], true
	if obj == nil {
		obj, defined = p.info.Uses[id], false
	}
	if obj == nil {
		return
	}
	if defined && obj.Parent() != nil {
		s.locals[obj.Name()] = true
	}
	declared := p.sourceOf(obj)
	if _, isImport := obj.(*types.PkgName); isImport || declared == nil {
		return
	}
	switch {
	case declared.external:
		if p.packageLevel(obj) && declared.moved != s.moved {
			p.block("cross-package test helper: %s uses %s declared in %s", s.name, obj.Name(), declared.name)
		}
	case s.external:
	case declared.moved == s.moved:
		if target, ok := p.renames[obj.Name()]; ok && p.packageLevel(obj) {
			s.edits = append(s.edits, edit{p.offset(id.Pos()), p.offset(id.End()), target})
		}
	case !s.moved:
		p.block("root file %s refers to family declaration %s in %s", s.name, obj.Name(), declared.name)
	case !obj.Exported():
		p.block("unexported crossing: %s uses root %s declared in %s", s.name, obj.Name(), declared.name)
	case declared.test && p.packageLevel(obj):
		p.block("%s uses root test declaration %s in %s", s.name, obj.Name(), declared.name)
	case p.packageLevel(obj):
		s.imports = append(s.imports, p.module)
		s.edits = append(s.edits, edit{p.offset(id.Pos()), p.offset(id.End()), p.rootName + "." + obj.Name()})
	}
}

// receiver blocks methods that a moved file declares on a root type.
func (p *plan) receiver(s *source, node *ast.FuncDecl) {
	method, ok := p.info.Defs[node.Name].(*types.Func)
	if !ok || node.Recv == nil || !s.moved {
		return
	}
	base := method.Type().(*types.Signature).Recv().Type()
	if pointer, ok := base.(*types.Pointer); ok {
		base = pointer.Elem()
	}
	if named, ok := base.(*types.Named); ok {
		if declared := p.sourceOf(named.Obj()); declared != nil && !declared.moved {
			p.block("%s declares method %s on root type %s", s.name, node.Name.Name, named.Obj().Name())
		}
	}
}

// embedded blocks embedded fields whose field name a rename would change.
func (p *plan) embedded(s *source, node *ast.StructType) {
	for _, field := range node.Fields.List {
		if len(field.Names) > 0 {
			continue
		}
		expr := field.Type
		if star, ok := expr.(*ast.StarExpr); ok {
			expr = star.X
		}
		if index, ok := expr.(*ast.IndexExpr); ok {
			expr = index.X
		}
		if selector, ok := expr.(*ast.SelectorExpr); ok {
			expr = selector.Sel
		}
		if id, ok := expr.(*ast.Ident); ok {
			if _, renamed := p.renames[id.Name]; renamed && p.sourceOf(p.info.Uses[id]) != nil {
				p.block("%s embeds %s; the rename would change its field name", s.name, id.Name)
			}
		}
	}
}
