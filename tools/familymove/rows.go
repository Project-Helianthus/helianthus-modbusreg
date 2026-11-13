package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"go/types"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// rowFiles name source paths and symbols row by row. Each is the output of
// json.dumps(indent=2, ensure_ascii=False) plus a newline; the tool refuses
// a file it cannot reproduce byte for byte.
var rowFiles = []string{
	"scripts/size-baseline.json",
	"scripts/go-lint-baseline.json",
	"scripts/behavior-name-baseline.json",
	"naming-inventory.json",
}

var identifierPattern = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*`)

// output is one file the move writes; from is set when it moves a file.
type output struct {
	path, from string
	data       []byte
	note       string
}

// write prepares every output first and only then changes the tree.
func (p *plan) write() ([]string, error) {
	outputs, err := p.goOutputs()
	if err != nil {
		return nil, err
	}
	for _, prepare := range []func() ([]output, error){p.rowOutputs, p.markdownOutputs} {
		more, err := prepare()
		if err != nil {
			return nil, err
		}
		outputs = append(outputs, more...)
	}
	if err := os.MkdirAll(filepath.Join(p.root, p.fam.name), 0o755); err != nil {
		return nil, err
	}
	summary := []string{fmt.Sprintf("renames: %d, collisions: 0", len(p.renames))}
	for _, out := range outputs {
		target := filepath.Join(p.root, filepath.FromSlash(out.path))
		info, err := os.Stat(filepath.Join(p.root, filepath.FromSlash(out.from)))
		if err != nil {
			return nil, err
		}
		if out.from != out.path {
			if err := os.Rename(filepath.Join(p.root, out.from), target); err != nil {
				return nil, err
			}
		}
		if err := os.WriteFile(target, out.data, info.Mode().Perm()); err != nil {
			return nil, err
		}
		summary = append(summary, out.path+": "+out.note)
	}
	return summary, nil
}

// goOutputs applies and formats the edits of every changed Go source.
func (p *plan) goOutputs() ([]output, error) {
	var outputs []output
	for _, name := range p.order {
		s := p.sources[name]
		if len(s.edits) == 0 && !s.moved {
			continue
		}
		edited, err := applyEdits(s.text, s.edits)
		if err != nil {
			return nil, fmt.Errorf("%s: %v", name, err)
		}
		formatted, err := format.Source(edited)
		if err != nil {
			return nil, fmt.Errorf("%s: %v", name, err)
		}
		out := output{path: name, from: name, data: formatted, note: "edited at the root"}
		if s.moved {
			out.path, out.note = p.fam.name+"/"+name, "moved from "+name
			if _, err := os.Stat(filepath.Join(p.root, p.fam.name, name)); err == nil {
				return nil, fmt.Errorf("%s already exists", out.path)
			}
		}
		outputs = append(outputs, out)
	}
	return outputs, nil
}

func applyEdits(text []byte, edits []edit) ([]byte, error) {
	sort.SliceStable(edits, func(i, j int) bool { return edits[i].start < edits[j].start })
	var out bytes.Buffer
	last := 0
	for _, e := range edits {
		if e.start < last {
			return nil, fmt.Errorf("overlapping edits at offset %d", e.start)
		}
		out.Write(text[last:e.start])
		out.WriteString(e.text)
		last = e.end
	}
	out.Write(text[last:])
	return out.Bytes(), nil
}

// movedPaths maps each moved root path to its path in the family package.
func (p *plan) movedPaths() map[string]string {
	paths := map[string]string{}
	for name, s := range p.sources {
		if s.moved {
			paths[name] = p.fam.name + "/" + name
		}
	}
	return paths
}

// rowOutputs relocates the rows of moved files in place: the path, and the
// symbol of a row whose path moved. Order and formatting are unchanged.
func (p *plan) rowOutputs() ([]output, error) {
	var outputs []output
	paths := p.movedPaths()
	for _, rel := range rowFiles {
		data, err := os.ReadFile(filepath.Join(p.root, filepath.FromSlash(rel)))
		if os.IsNotExist(err) {
			continue
		} else if err != nil {
			return nil, err
		}
		node, err := parseJSON(data)
		if err != nil {
			return nil, fmt.Errorf("%s: %v", rel, err)
		}
		if encodeJSON(node) != string(data) {
			return nil, fmt.Errorf("%s: not reproducible as indent=2 JSON; refusing to rewrite", rel)
		}
		if count := relocate(node, paths, p.renames); count > 0 {
			outputs = append(outputs, output{path: rel, from: rel, data: []byte(encodeJSON(node)),
				note: fmt.Sprintf("%d rows relocated", count)})
		}
	}
	return outputs, nil
}

// relocate rewrites moved paths and returns the number of rewritten values.
func relocate(n *jsonNode, paths, renames map[string]string) int {
	count, symbol := 0, -1
	movedRow := false
	for index, item := range n.items {
		if n.kind == 'o' && n.keys[index] == "symbol" {
			symbol = index
			continue
		}
		if target, ok := paths[item.text]; ok && item.kind == 's' {
			item.text = target
			count++
			movedRow = movedRow || (n.kind == 'o' && n.keys[index] == "path")
			continue
		}
		count += relocate(item, paths, renames)
	}
	if movedRow && symbol >= 0 && n.items[symbol].kind == 's' {
		n.items[symbol].text = identifierPattern.ReplaceAllStringFunc(n.items[symbol].text, func(name string) string {
			if target, ok := renames[name]; ok {
				return target
			}
			return name
		})
	}
	return count
}

// jsonNode is an order-preserving JSON value: 'o' object, 'a' array,
// 's' string, 'n' number and 'l' literal, with the raw number or literal text.
type jsonNode struct {
	kind  byte
	text  string
	keys  []string
	items []*jsonNode
}

func parseJSON(data []byte) (*jsonNode, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	node, err := parseValue(decoder)
	if err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, fmt.Errorf("trailing data after the JSON value")
	}
	return node, nil
}

func parseValue(decoder *json.Decoder) (*jsonNode, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	switch value := token.(type) {
	case json.Delim:
		node := &jsonNode{kind: 'a'}
		if value == '{' {
			node.kind = 'o'
		}
		for decoder.More() {
			if node.kind == 'o' {
				key, err := decoder.Token()
				if err != nil {
					return nil, err
				}
				node.keys = append(node.keys, key.(string))
			}
			item, err := parseValue(decoder)
			if err != nil {
				return nil, err
			}
			node.items = append(node.items, item)
		}
		_, err := decoder.Token()
		return node, err
	case string:
		return &jsonNode{kind: 's', text: value}, nil
	case json.Number:
		return &jsonNode{kind: 'n', text: string(value)}, nil
	case bool:
		return &jsonNode{kind: 'l', text: fmt.Sprint(value)}, nil
	}
	return &jsonNode{kind: 'l', text: "null"}, nil
}

// encodeJSON writes the node as json.dumps(indent=2, ensure_ascii=False)
// does, followed by a newline.
func encodeJSON(node *jsonNode) string {
	var out strings.Builder
	encodeValue(&out, node, 0)
	out.WriteByte('\n')
	return out.String()
}

func encodeValue(out *strings.Builder, node *jsonNode, depth int) {
	switch node.kind {
	case 's':
		quoteJSON(out, node.text)
		return
	case 'n', 'l':
		out.WriteString(node.text)
		return
	}
	open, close := "[", "]"
	if node.kind == 'o' {
		open, close = "{", "}"
	}
	out.WriteString(open)
	for index, item := range node.items {
		if index > 0 {
			out.WriteByte(',')
		}
		out.WriteString("\n" + strings.Repeat("  ", depth+1))
		if node.kind == 'o' {
			quoteJSON(out, node.keys[index])
			out.WriteString(": ")
		}
		encodeValue(out, item, depth+1)
	}
	if len(node.items) > 0 {
		out.WriteString("\n" + strings.Repeat("  ", depth))
	}
	out.WriteString(close)
}

func quoteJSON(out *strings.Builder, text string) {
	escapes := map[rune]string{'"': `\"`, '\\': `\\`, '\n': `\n`, '\r': `\r`, '\t': `\t`, '\b': `\b`, '\f': `\f`}
	out.WriteByte('"')
	for _, r := range text {
		if escape, ok := escapes[r]; ok {
			out.WriteString(escape)
		} else if r < 0x20 {
			fmt.Fprintf(out, `\u%04x`, r)
		} else {
			out.WriteRune(r)
		}
	}
	out.WriteByte('"')
}

// goDoc rewrites exec.Command("go", "doc", ".", symbol) when symbol names a
// moved type or a constructor of one.
func (p *plan) goDoc(s *source, call *ast.CallExpr) {
	fun, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || fun.Sel.Name != "Command" || len(call.Args) != 4 {
		return
	}
	if x, ok := fun.X.(*ast.Ident); !ok {
		return
	} else if name, ok := p.info.Uses[x].(*types.PkgName); !ok || name.Imported().Path() != "os/exec" {
		return
	}
	values := make([]string, len(call.Args))
	for index, arg := range call.Args {
		literal, ok := arg.(*ast.BasicLit)
		if !ok || literal.Kind != token.STRING {
			return
		}
		values[index], _ = strconv.Unquote(literal.Value)
	}
	target, ok := p.docTarget(values[3])
	if values[0] != "go" || values[1] != "doc" || values[2] != "." || !ok {
		return
	}
	directory := "./" + p.fam.name
	if s.moved {
		directory = "."
	}
	for index, text := range map[int]string{2: directory, 3: target} {
		s.edits = append(s.edits, edit{p.offset(call.Args[index].Pos()), p.offset(call.Args[index].End()), strconv.Quote(text)})
	}
}

// dataPath prefixes a moved file's root-relative data path with "../".
func (p *plan) dataPath(s *source, literal *ast.BasicLit) {
	if !s.moved || literal.Kind != token.STRING {
		return
	}
	value, err := strconv.Unquote(literal.Value)
	if err != nil {
		return
	}
	for _, directory := range dataRoots {
		if value == directory || strings.HasPrefix(value, directory+"/") {
			if !strings.HasPrefix(literal.Value[1:], directory) {
				p.block("%s: data path %s is not a plain literal", s.name, literal.Value)
				return
			}
			start := p.offset(literal.Pos()) + 1
			s.edits = append(s.edits, edit{start, start, "../"})
			return
		}
	}
}

// rootNames are the names a comment of s may use to qualify root names.
func (p *plan) rootNames(s *source) map[string]bool {
	names := map[string]bool{p.rootName: true}
	for _, spec := range s.file.Imports {
		if path, _ := strconv.Unquote(spec.Path.Value); path == p.module && spec.Name != nil {
			names[spec.Name.Name] = true
		}
	}
	return names
}

// imports adds the import paths the edits need. An external test whose every
// root selector now names the family swaps its root import for the family's.
func (p *plan) imports(s *source) {
	var rootSpec *ast.ImportSpec
	for _, spec := range s.file.Imports {
		if path, _ := strconv.Unquote(spec.Path.Value); path == p.module {
			rootSpec = spec
		}
	}
	swap := s.external && rootSpec != nil && s.rootUses > 0 && s.rootUses == s.redirect
	added := map[string]bool{}
	for _, path := range s.imports {
		if added[path] {
			continue
		}
		added[path] = true
		name := filepath.Base(path)
		if path == p.module {
			name = p.rootName
		}
		if s.locals[name] {
			p.block("%s declares %s, which the added import needs", s.name, name)
		}
		if swap && path != p.module {
			s.edits = append(s.edits, edit{p.offset(rootSpec.Pos()), p.offset(rootSpec.End()), importSpec(path, name)})
			swap = false
			continue
		}
		s.edits = append(s.edits, p.importEdit(s, path, name))
	}
	for _, target := range p.renames {
		if s.moved && s.locals[target] {
			p.block("%s declares %s; it would shadow a renamed declaration", s.name, target)
		}
	}
}

// importSpec names the import when the package name differs from the last
// path element.
func importSpec(path, name string) string {
	if name != filepath.Base(path) {
		return name + " " + strconv.Quote(path)
	}
	return strconv.Quote(path)
}

// importEdit inserts an import spec after the last import of the file, in a
// new group when the last import is a standard library package.
func (p *plan) importEdit(s *source, path, name string) edit {
	var last *ast.GenDecl
	for _, decl := range s.file.Decls {
		if general, ok := decl.(*ast.GenDecl); ok && general.Tok == token.IMPORT {
			last = general
		}
	}
	quoted := importSpec(path, name)
	if last == nil {
		at := p.offset(s.file.Name.End())
		return edit{at, at, "\n\nimport " + quoted}
	}
	if !last.Lparen.IsValid() {
		at := p.offset(last.End())
		return edit{at, at, "\n\nimport " + quoted}
	}
	spec := last.Specs[len(last.Specs)-1].(*ast.ImportSpec)
	at := p.offset(spec.End()) + bytes.IndexByte(s.text[p.offset(spec.End()):], '\n')
	previous, _ := strconv.Unquote(spec.Path.Value)
	separator := "\n\t"
	if !strings.Contains(strings.Split(previous, "/")[0], ".") {
		separator = "\n\n\t"
	}
	return edit{at, at, separator + quoted}
}

// renameTable is the CHANGELOG table of every rename, sorted by old name.
func renameTable(p *plan) string {
	var names []string
	for name := range p.renames {
		names = append(names, name)
	}
	sort.Strings(names)
	var out strings.Builder
	out.WriteString("| Previous name | Current name |\n| --- | --- |\n")
	for _, name := range names {
		fmt.Fprintf(&out, "| `%s.%s` | `%s.%s` |\n", p.rootName, name, p.fam.name, p.renames[name])
	}
	return out.String()
}
