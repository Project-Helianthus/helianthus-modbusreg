package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// zetaFamily is the synthetic family: two moved files and two external tests
// that stay at the root.
var zetaFamily = family{name: "zeta", prefix: "Zeta",
	files: []string{"zeta_profile.go", "zeta_profile_test.go"},
	keep:  []string{"zeta_public_test.go", "zeta_only_test.go"}}

// syntheticModule is the module before the move.
func syntheticModule() map[string]string {
	return map[string]string{
		"go.mod": "module example.com/synthreg\n\ngo 1.22\n",
		"contract.go": `// Package synthreg is a synthetic registry root.
package synthreg

// Words is a root contract.
type Words []uint16

// Decode is a root contract function.
func Decode(words Words) int { return len(words) }
`,
		"zeta_profile.go": `package synthreg

import "errors"

// ErrZetaInvalid reports an invalid profile.
var ErrZetaInvalid = errors.New("zeta invalid")

// ZetaProfile reads one Words slice.
type ZetaProfile struct {
	words Words
}

// NewZetaProfile returns a ZetaProfile; see also synthreg.ZetaLimit.
func NewZetaProfile(words Words) (*ZetaProfile, error) {
	if Decode(words) > ZetaLimit {
		return nil, ErrZetaInvalid
	}
	return &ZetaProfile{words: words}, nil
}

// ZetaLimit bounds a profile.
const ZetaLimit = 4

// DecodeZeta keeps its name: it does not begin with the prefix.
func DecodeZeta(profile *ZetaProfile) int { return Decode(profile.words) }
`,
		"zeta_profile_test.go": `package synthreg

import (
	"os"
	"testing"
)

func TestZetaProfileReadsFixture(t *testing.T) {
	if _, err := os.ReadFile("testdata/zeta.txt"); err != nil {
		t.Fatal(err)
	}
	profile, err := NewZetaProfile(Words{1})
	if err != nil || DecodeZeta(profile) != 1 {
		t.Fatal(err)
	}
}
`,
		"zeta_public_test.go": `package synthreg_test

import (
	"os/exec"
	"testing"

	reg "example.com/synthreg"
)

// TestZetaPublic reaches reg.ZetaProfile from an external test.
func TestZetaPublic(t *testing.T) {
	command := exec.Command("go", "doc", ".", "NewZetaProfile")
	if len(command.Args) != 4 {
		t.Fatal(command.Args)
	}
	if _, err := reg.NewZetaProfile(reg.Words{1, 2}); err != nil {
		t.Fatal(err)
	}
}
`,
		"zeta_only_test.go": `package synthreg_test

import (
	"testing"

	reg "example.com/synthreg"
)

func TestZetaOnlyLimit(t *testing.T) {
	if reg.ZetaLimit != 4 {
		t.Fatal(reg.ZetaLimit)
	}
}
`,
		"root_test.go": `package synthreg_test

import (
	"testing"

	reg "example.com/synthreg"
)

func sharedHelper() reg.Words { return reg.Words{1} }

func TestRoot(t *testing.T) {
	if reg.Decode(sharedHelper()) != 1 {
		t.Fatal("decode")
	}
}
`,
		"testdata/zeta.txt": "synthetic\n",
		"docs/zeta.md": "# Zeta\n\n`NewZetaProfile` returns a `ZetaProfile` bounded by `synthreg.ZetaLimit`;\n" +
			"`DecodeZeta` and `ErrZetaInvalid` keep their names, and so does `TestZetaProfileReadsFixture`.\n\n" +
			"Source: [profile](../zeta_profile.go) and [public test](../zeta_public_test.go).\n\n" +
			"Run `go test . -run '^TestZetaProfile'`; `go test . -run '^TestZeta'` also selects root tests.\n",
		"scripts/size-baseline.json":          sizeRows("zeta_profile.go", "(*ZetaProfile).Read"),
		"scripts/behavior-name-baseline.json": behaviorRows("zeta_profile.go", "ZetaProfile"),
		"scripts/go-lint-baseline.json":       "{\n  \"entries\": [\n    {\n      \"path\": \"contract.go\",\n      \"symbol\": \"ZetaProfile\"\n    }\n  ]\n}\n",
		"naming-inventory.json":               inventoryRows("zeta_profile.go"),
	}
}

// sizeRows varies both identity fields of a row: a moved path with a renamed
// symbol, and an unmoved path whose symbol names a moved declaration.
func sizeRows(path, symbol string) string {
	return "{\n  \"version\": 1,\n  \"violations\": [\n    {\n      \"path\": \"" + path + "\",\n      \"symbol\": \"" + symbol +
		"\",\n      \"metric\": \"function_lines\",\n      \"value\": 200\n    },\n    {\n      \"path\": \"contract.go\",\n" +
		"      \"symbol\": \"(*ZetaProfile).Read\",\n      \"metric\": \"function_lines\",\n      \"value\": 200\n    }\n  ]\n}\n"
}

// behaviorRows holds a renamed symbol, a filename symbol and an unrenamed
// symbol under the moved path, and an unmoved path with an old name.
func behaviorRows(path, symbol string) string {
	row := func(path, symbol, category string) string {
		return "    {\n      \"path\": \"" + path + "\",\n      \"symbol\": \"" + symbol + "\",\n      \"category\": \"" + category + "\"\n    }"
	}
	return "{\n  \"version\": 1,\n  \"baseline\": [\n" + strings.Join([]string{
		row(path, symbol, "identifier"), row(path, "zeta_profile.go", "filename"),
		row(path, "DecodeZeta", "identifier"), row("contract.go", "ZetaProfile", "identifier")}, ",\n") + "\n  ]\n}\n"
}

// inventoryRows keeps non-ASCII text, HTML characters, escapes, empty
// containers and every literal kind, as json.dumps writes them.
func inventoryRows(path string) string {
	return "{\n  \"purpose\": \"Synthetic: ă, <&> and \\\"quotes\\\"\\n.\",\n  \"source_files\": [\n    \"contract.go\",\n    \"" + path +
		"\",\n    \"zeta_public_test.go\"\n  ],\n  \"empty_list\": [],\n  \"empty_object\": {},\n" +
		"  \"literals\": [\n    true,\n    false,\n    null,\n    1.5\n  ]\n}\n"
}

func writeModule(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for path, text := range files {
		target := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func runTool(root string, args ...string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	code := run(append([]string{"-family", "zeta", "-dir", root}, args...), &stdout, &stderr, []family{zetaFamily})
	return code, stdout.String(), stderr.String()
}

func readFile(t *testing.T, root, path string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// TestApplyMovesFamilyAndRewritesEveryReference compares each rewritten file
// with its expected text, then builds, vets and tests the moved module.
func TestApplyMovesFamilyAndRewritesEveryReference(t *testing.T) {
	root := writeModule(t, syntheticModule())
	code, stdout, stderr := runTool(root, "-apply")
	if code != 0 {
		t.Fatalf("apply exit=%d stderr=%s", code, stderr)
	}
	for _, line := range []string{
		"renames: 3, collisions: 0",
		"zeta/zeta_profile.go: moved from zeta_profile.go",
		"zeta_only_test.go: edited at the root",
		"scripts/size-baseline.json: 1 rows relocated",
		"scripts/behavior-name-baseline.json: 3 rows relocated",
		"naming-inventory.json: 1 rows relocated",
		"| `synthreg.NewZetaProfile` | `zeta.NewProfile` |",
		"| `synthreg.ZetaLimit` | `zeta.Limit` |",
		"| `synthreg.ZetaProfile` | `zeta.Profile` |",
	} {
		if !strings.Contains(stdout, line+"\n") {
			t.Errorf("summary lacks %q:\n%s", line, stdout)
		}
	}
	for path, want := range expectedAfterMove() {
		if got := readFile(t, root, path); got != want {
			t.Errorf("%s:\n%s\nwant:\n%s", path, got, want)
		}
	}
	before := syntheticModule()
	for _, path := range []string{"scripts/go-lint-baseline.json", "root_test.go", "contract.go"} {
		if readFile(t, root, path) != before[path] {
			t.Errorf("%s changed", path)
		}
	}
	for _, path := range zetaFamily.files {
		if _, err := os.Stat(filepath.Join(root, path)); !os.IsNotExist(err) {
			t.Errorf("%s left at the root: %v", path, err)
		}
	}
	for _, args := range [][]string{{"vet", "./..."}, {"test", "-count=1", "./..."}} {
		command := exec.Command("go", args...)
		command.Dir = root
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("go %v: %v\n%s", args, err, output)
		}
	}
	if code, stdout, _ := runTool(root, "-check"); code != 0 || !strings.Contains(stdout, "family zeta: moved, 0 findings") {
		t.Fatalf("moved check exit=%d:\n%s", code, stdout)
	}
}

// TestCheckAfterMoveReportsEachLeftover changes one thing at a time after a
// finished move: a selected file back at the root, a root declaration of an
// old name and a root selector of an old name.
func TestCheckAfterMoveReportsEachLeftover(t *testing.T) {
	for name, test := range map[string]struct{ path, text, want string }{
		"file left at the root": {"zeta_profile.go", "package synthreg\n", "left at the root: zeta_profile.go"},
		"root declares old name": {"contract_extra.go", "package synthreg\n\n// ZetaLimit is left over.\nconst ZetaLimit = 4\n",
			"contract_extra.go declares family name ZetaLimit"},
		"root selects old name": {"root_extra_test.go", "package synthreg_test\n\nimport reg \"example.com/synthreg\"\n\nvar _ = reg.NewZetaProfile\n",
			"root_extra_test.go refers to synthreg.NewZetaProfile"},
	} {
		t.Run(name, func(t *testing.T) {
			root := writeModule(t, syntheticModule())
			if code, _, stderr := runTool(root, "-apply"); code != 0 {
				t.Fatal(stderr)
			}
			if err := os.WriteFile(filepath.Join(root, test.path), []byte(test.text), 0o644); err != nil {
				t.Fatal(err)
			}
			code, stdout, stderr := runTool(root, "-check")
			if code != 1 || !strings.Contains(stdout, test.want+"\n") {
				t.Fatalf("exit=%d stdout=%s stderr=%s", code, stdout, stderr)
			}
		})
	}
}

// TestCheckReportsEachBlockerAndApplyRefuses applies one mutation per case to
// the unmoved module. Each blocker must be reported by -check, and -apply
// must refuse it without changing a file.
func TestCheckReportsEachBlockerAndApplyRefuses(t *testing.T) {
	for name, test := range map[string]struct {
		files map[string]string
		moved []string
		want  string
	}{
		"unexported crossing": {files: map[string]string{
			"contract_helper.go": "package synthreg\n\nfunc helper() int { return 1 }\n",
			"zeta_extra.go":      "package synthreg\n\nfunc zetaHelper() int { return helper() }\n"},
			moved: []string{"zeta_extra.go"}, want: "unexported crossing: zeta_extra.go uses root helper declared in contract_helper.go"},
		"root refers to family": {files: map[string]string{"contract_use.go": "package synthreg\n\nfunc useZeta() *ZetaProfile { return nil }\n"},
			want: "root file contract_use.go refers to family declaration ZetaProfile in zeta_profile.go"},
		"root test declaration": {files: map[string]string{
			"contract_internal_test.go": "package synthreg\n\n// RootFixture is a root test fixture.\nfunc RootFixture() Words { return Words{1} }\n",
			"zeta_extra_test.go":        "package synthreg\n\nvar _ = RootFixture()\n"},
			moved: []string{"zeta_extra_test.go"}, want: "zeta_extra_test.go uses root test declaration RootFixture in contract_internal_test.go"},
		"cross-package test helper": {files: map[string]string{"zeta_external_test.go": "package synthreg_test\n\nvar _ = sharedHelper()\n"},
			moved: []string{"zeta_external_test.go"}, want: "cross-package test helper: zeta_external_test.go uses sharedHelper declared in root_test.go"},
		"rename collision": {files: map[string]string{"zeta_extra.go": "package synthreg\n\n// Profile collides.\ntype Profile struct{}\n"},
			moved: []string{"zeta_extra.go"}, want: "rename collision: Profile and ZetaProfile both become Profile"},
		"method on root type": {files: map[string]string{"zeta_extra.go": "package synthreg\n\n// Zeta is a method on a root type.\nfunc (Words) Zeta() int { return 0 }\n"},
			moved: []string{"zeta_extra.go"}, want: "zeta_extra.go declares method Zeta on root type Words"},
		"embedded renamed type": {files: map[string]string{"zeta_extra.go": "package synthreg\n\ntype zetaWrapper struct{ ZetaProfile }\n\nvar _ zetaWrapper\n"},
			moved: []string{"zeta_extra.go"}, want: "zeta_extra.go embeds ZetaProfile; the rename would change its field name"},
		"shadowed rename": {files: map[string]string{"zeta_extra.go": "package synthreg\n\nfunc zetaShadow() int {\n\tProfile := 1\n\treturn Profile + ZetaLimit\n}\n"},
			moved: []string{"zeta_extra.go"}, want: "zeta_extra.go declares Profile; it would shadow a renamed declaration"},
		"import name taken": {files: map[string]string{"zeta_extra.go": "package synthreg\n\nfunc zetaTaken() int {\n\tsynthreg := Decode(nil)\n\treturn synthreg\n}\n"},
			moved: []string{"zeta_extra.go"}, want: "zeta_extra.go declares synthreg, which the added import needs"},
		"module package outside the root": {files: map[string]string{"tools/use/main.go": "package main\n\nimport reg \"example.com/synthreg\"\n\nvar _ = reg.NewZetaProfile\n\nfunc main() {}\n"},
			want: "tools/use/main.go refers to synthreg.NewZetaProfile; packages outside the root are not rewritten"},
		"embedded data path": {files: map[string]string{"zeta_extra.go": "package synthreg\n\nimport _ \"embed\"\n\n//go:embed testdata/zeta.txt\nvar zetaData string\n"},
			moved: []string{"zeta_extra.go"}, want: "zeta_extra.go: go:embed paths are not relocated"},
	} {
		t.Run(name, func(t *testing.T) {
			files := syntheticModule()
			for path, text := range test.files {
				files[path] = text
			}
			root := writeModule(t, files)
			saved := zetaFamily
			zetaFamily.files = append(append([]string(nil), saved.files...), test.moved...)
			defer func() { zetaFamily = saved }()
			code, stdout, stderr := runTool(root, "-check")
			if code != 1 || !strings.Contains(stdout, test.want+"\n") {
				t.Fatalf("check exit=%d stdout=%s stderr=%s", code, stdout, stderr)
			}
			if code, _, stderr := runTool(root, "-apply"); code != 1 || !strings.Contains(stderr, test.want+"\n") {
				t.Fatalf("apply exit=%d stderr=%s", code, stderr)
			}
			for path, text := range files {
				if readFile(t, root, path) != text {
					t.Fatalf("refused apply changed %s", path)
				}
			}
		})
	}
}

// TestApplyRefusesUnreproducibleRows keeps the tree unchanged when a row
// file is not in the indent=2 form the tool reproduces.
func TestApplyRefusesUnreproducibleRows(t *testing.T) {
	for name, text := range map[string]string{
		"compact":          "{\"version\":1,\"violations\":[]}\n",
		"escaped unicode":  "{\n  \"purpose\": \"\\u0103\"\n}\n",
		"no final newline": "{\n  \"version\": 1\n}",
	} {
		t.Run(name, func(t *testing.T) {
			files := syntheticModule()
			files["scripts/size-baseline.json"] = text
			root := writeModule(t, files)
			code, _, stderr := runTool(root, "-apply")
			if code != 1 || !strings.Contains(stderr, "scripts/size-baseline.json: not reproducible") {
				t.Fatalf("exit=%d stderr=%s", code, stderr)
			}
			for path, text := range files {
				if readFile(t, root, path) != text {
					t.Fatalf("refused apply changed %s", path)
				}
			}
		})
	}
}

func TestUsageErrors(t *testing.T) {
	for name, args := range map[string][]string{
		"no mode":        {"-family", "zeta"},
		"both modes":     {"-family", "zeta", "-check", "-apply"},
		"unknown family": {"-family", "other", "-check"},
		"extra argument": {"-family", "zeta", "-check", "extra"},
		"unknown flag":   {"-family", "zeta", "-check", "-force"},
	} {
		t.Run(name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := run(args, &stdout, &stderr, []family{zetaFamily}); code != 2 {
				t.Fatalf("exit=%d", code)
			}
		})
	}
}

// expectedAfterMove is the exact text of every file the move rewrites.
func expectedAfterMove() map[string]string {
	return map[string]string{
		"zeta/zeta_profile.go": `package zeta

import "errors"

import "example.com/synthreg"

// ErrZetaInvalid reports an invalid profile.
var ErrZetaInvalid = errors.New("zeta invalid")

// Profile reads one Words slice.
type Profile struct {
	words synthreg.Words
}

// NewProfile returns a Profile; see also Limit.
func NewProfile(words synthreg.Words) (*Profile, error) {
	if synthreg.Decode(words) > Limit {
		return nil, ErrZetaInvalid
	}
	return &Profile{words: words}, nil
}

// Limit bounds a profile.
const Limit = 4

// DecodeZeta keeps its name: it does not begin with the prefix.
func DecodeZeta(profile *Profile) int { return synthreg.Decode(profile.words) }
`,
		"zeta/zeta_profile_test.go": `package zeta

import (
	"os"
	"testing"

	"example.com/synthreg"
)

func TestZetaProfileReadsFixture(t *testing.T) {
	if _, err := os.ReadFile("../testdata/zeta.txt"); err != nil {
		t.Fatal(err)
	}
	profile, err := NewProfile(synthreg.Words{1})
	if err != nil || DecodeZeta(profile) != 1 {
		t.Fatal(err)
	}
}
`,
		"zeta_public_test.go": `package synthreg_test

import (
	"os/exec"
	"testing"

	reg "example.com/synthreg"
	"example.com/synthreg/zeta"
)

// TestZetaPublic reaches zeta.Profile from an external test.
func TestZetaPublic(t *testing.T) {
	command := exec.Command("go", "doc", "./zeta", "NewProfile")
	if len(command.Args) != 4 {
		t.Fatal(command.Args)
	}
	if _, err := zeta.NewProfile(reg.Words{1, 2}); err != nil {
		t.Fatal(err)
	}
}
`,
		"zeta_only_test.go": `package synthreg_test

import (
	"testing"

	"example.com/synthreg/zeta"
)

func TestZetaOnlyLimit(t *testing.T) {
	if zeta.Limit != 4 {
		t.Fatal(zeta.Limit)
	}
}
`,
		"docs/zeta.md": "# Zeta\n\n`NewProfile` returns a `Profile` bounded by `zeta.Limit`;\n" +
			"`DecodeZeta` and `ErrZetaInvalid` keep their names, and so does `TestZetaProfileReadsFixture`.\n\n" +
			"Source: [profile](../zeta/zeta_profile.go) and [public test](../zeta_public_test.go).\n\n" +
			"Run `go test ./zeta -run '^TestZetaProfile'`; `go test . -run '^TestZeta'` also selects root tests.\n",
		"scripts/size-baseline.json":          sizeRows("zeta/zeta_profile.go", "(*Profile).Read"),
		"scripts/behavior-name-baseline.json": behaviorRows("zeta/zeta_profile.go", "Profile"),
		"naming-inventory.json":               inventoryRows("zeta/zeta_profile.go"),
	}
}

// TestCheckAnotherFamilyAfterOneMoved analyses a second family while root
// external tests import the first, already moved, family package.
func TestCheckAnotherFamilyAfterOneMoved(t *testing.T) {
	files := syntheticModule()
	files["omega_profile.go"] = "package synthreg\n\n// OmegaLimit bounds an omega profile.\nvar OmegaLimit = Words(nil)\n"
	root := writeModule(t, files)
	if code, _, stderr := runTool(root, "-apply"); code != 0 {
		t.Fatal(stderr)
	}
	omega := family{name: "omega", prefix: "Omega", files: []string{"omega_profile.go"}}
	var stdout, stderr bytes.Buffer
	code := run([]string{"-family", "omega", "-check", "-dir", root}, &stdout, &stderr, []family{zetaFamily, omega})
	if code != 0 || stdout.String() != "family omega: not moved, 0 findings\n" {
		t.Fatalf("exit=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
}
