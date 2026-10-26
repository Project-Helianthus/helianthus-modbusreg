// Command huaweicatalogue generates the Huawei SUN2000 inverter catalogue
// under profiles/vendor/huawei/catalogue from the JSON twins and model tables
// of the consulted CC0 inverter reference baseline. It is deterministic,
// refuses counts that differ from the baseline accounting unless -expect
// states the reviewed numbers, and writes locators beyond title, issue, date
// and page only to a sidecar directory outside the repository.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		writeLine(os.Stderr, "huaweicatalogue: "+err.Error())
		os.Exit(1)
	}
}

func writeLine(w io.Writer, text string) {
	_, _ = fmt.Fprintln(w, text)
}

func run(arguments []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("huaweicatalogue", flag.ContinueOnError)
	reference := flags.String("reference", "", "root of the consulted inverter reference (required)")
	out := flags.String("out", "profiles/vendor/huawei/catalogue", "catalogue directory")
	sidecar := flags.String("sidecar", "", "private sidecar directory outside the repository (optional)")
	expect := flags.String("expect", "", "reviewed counts that replace the baseline accounting, as ro=N,core=N,controls=N,models=N,cards=N")
	check := flags.Bool("check", false, "compare the generated files with the catalogue directory and write nothing")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if *reference == "" {
		return fmt.Errorf("-reference is required")
	}
	expected, err := parseExpect(*expect)
	if err != nil {
		return err
	}
	generated, err := generate(options{Reference: *reference, Expect: expected})
	if err != nil {
		return err
	}
	names := make([]string, 0, len(generated.Files))
	for name := range generated.Files {
		names = append(names, name)
	}
	sort.Strings(names)
	if *check {
		var differences []string
		for _, name := range names {
			current, err := os.ReadFile(filepath.Join(*out, name))
			if err != nil || !bytes.Equal(current, generated.Files[name]) {
				differences = append(differences, name)
			}
		}
		entries, err := os.ReadDir(*out)
		if err != nil {
			return err
		}
		if len(entries) != len(names) {
			differences = append(differences, fmt.Sprintf("%d files present, %d generated", len(entries), len(names)))
		}
		if len(differences) > 0 {
			return fmt.Errorf("catalogue differs from the reference: %v", differences)
		}
		writeLine(stdout, "catalogue matches the reference: "+countLine(generated.Counts))
		return nil
	}
	if *sidecar != "" {
		guarded := []string{*out}
		if root, err := moduleRoot(*out); err == nil {
			guarded = append(guarded, root)
		}
		for _, root := range guarded {
			absolute, err := filepath.Abs(root)
			if err != nil {
				return err
			}
			inside, err := insideDirectory(absolute, *sidecar)
			if err != nil {
				return err
			}
			if inside {
				return fmt.Errorf("the sidecar directory must lie outside the repository and the catalogue directory")
			}
		}
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		return err
	}
	present, err := os.ReadDir(*out)
	if err != nil {
		return err
	}
	for _, entry := range present {
		if _, ok := generated.Files[entry.Name()]; !ok {
			return fmt.Errorf("the catalogue directory holds %s, which the generator does not produce", entry.Name())
		}
	}
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(*out, name), generated.Files[name], 0o644); err != nil {
			return err
		}
	}
	if *sidecar != "" {
		if err := os.MkdirAll(*sidecar, 0o700); err != nil {
			return err
		}
		for name, data := range generated.Sidecar {
			if err := os.WriteFile(filepath.Join(*sidecar, name), data, 0o600); err != nil {
				return err
			}
		}
	}
	writeLine(stdout, "catalogue written: "+countLine(generated.Counts))
	return nil
}
