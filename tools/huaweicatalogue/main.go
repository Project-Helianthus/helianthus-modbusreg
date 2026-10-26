// Command huaweicatalogue generates the Huawei catalogue under
// profiles/vendor/huawei/catalogue. The inverter mode reads the JSON twins and
// model tables of the consulted CC0 inverter reference baseline; the gateway
// mode reads the JSON edition twins of the consulted CC0 gateway reference
// baseline and writes the SmartLogger, S-Dongle and EMMA catalogue to its
// subdirectory. Both modes are deterministic, refuse counts that differ from
// the baseline accounting unless -expect states the reviewed numbers, and
// write locators beyond title, issue, date and page only to a sidecar
// directory outside the repository.
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

// checkSidecar refuses a sidecar directory inside the catalogue directory or
// inside any module checkout: the module root above the catalogue directory,
// above the working directory and above the sidecar itself.
func checkSidecar(out, sidecar string) error {
	guarded := []string{out}
	for _, start := range []string{out, ".", sidecar} {
		if root, err := moduleRoot(start); err == nil {
			guarded = append(guarded, root)
		}
	}
	for _, root := range guarded {
		absolute, err := filepath.Abs(root)
		if err != nil {
			return err
		}
		inside, err := insideDirectory(absolute, sidecar)
		if err != nil {
			return err
		}
		if inside {
			return fmt.Errorf("the sidecar directory must lie outside the repository and the catalogue directory")
		}
	}
	return nil
}

func run(arguments []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("huaweicatalogue", flag.ContinueOnError)
	mode := flags.String("mode", "inverter", "catalogue to generate: inverter (SUN2000 reference) or gateway (SmartLogger, S-Dongle and EMMA reference)")
	reference := flags.String("reference", "", "root of the consulted reference of the mode (required)")
	out := flags.String("out", "", "catalogue directory (default "+defaultInverterOut+" for inverter, "+defaultGatewayOut+" for gateway)")
	sidecar := flags.String("sidecar", "", "private sidecar directory outside the repository (optional)")
	expect := flags.String("expect", "", "reviewed counts that replace the baseline accounting: ro=N,core=N,controls=N,models=N,cards=N for inverter; editions=N,ro=N,controls=N,rows=N,alarms=N for gateway")
	check := flags.Bool("check", false, "compare the generated files with the catalogue directory and write nothing")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if *reference == "" {
		return fmt.Errorf("-reference is required")
	}
	switch *mode {
	case "inverter":
		expected, err := parseExpect(*expect)
		if err != nil {
			return err
		}
		generated, err := generate(options{Reference: *reference, Expect: expected})
		if err != nil {
			return err
		}
		directory := *out
		if directory == "" {
			directory = defaultInverterOut
		}
		return emit(stdout, directory, generated.Files, generated.Sidecar, *sidecar, *check, countLine(generated.Counts), gatewayDirectory)
	case "gateway":
		expected, err := parseGatewayExpect(*expect)
		if err != nil {
			return err
		}
		generated, err := generateGateway(gatewayOptions{Reference: *reference, Expect: expected})
		if err != nil {
			return err
		}
		directory := *out
		if directory == "" {
			directory = defaultGatewayOut
		}
		return emit(stdout, directory, generated.Files, generated.Sidecar, *sidecar, *check, gatewayCountLine(generated.Counts), "")
	}
	return fmt.Errorf("unknown -mode %q", *mode)
}

// emit compares the generated files with the catalogue directory (-check) or
// writes them, and writes the sidecar outside the repository. The nested
// directory, when named, belongs to the other mode and is left alone.
func emit(stdout io.Writer, out string, files, sidecarFiles map[string][]byte, sidecar string, check bool, line, nested string) error {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	if check {
		var differences []string
		for _, name := range names {
			current, err := os.ReadFile(filepath.Join(out, name))
			if err != nil || !bytes.Equal(current, files[name]) {
				differences = append(differences, name)
			}
		}
		entries, err := os.ReadDir(out)
		if err != nil {
			return err
		}
		present := 0
		for _, entry := range entries {
			if nested != "" && entry.IsDir() && entry.Name() == nested {
				continue
			}
			present++
		}
		if present != len(names) {
			differences = append(differences, fmt.Sprintf("%d files present, %d generated", present, len(names)))
		}
		if len(differences) > 0 {
			return fmt.Errorf("catalogue differs from the reference: %v", differences)
		}
		writeLine(stdout, "catalogue matches the reference: "+line)
		return nil
	}
	if sidecar != "" {
		if err := checkSidecar(out, sidecar); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	present, err := os.ReadDir(out)
	if err != nil {
		return err
	}
	for _, entry := range present {
		if nested != "" && entry.IsDir() && entry.Name() == nested {
			continue
		}
		if _, ok := files[entry.Name()]; !ok {
			return fmt.Errorf("the catalogue directory holds %s, which the generator does not produce", entry.Name())
		}
	}
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(out, name), files[name], 0o644); err != nil {
			return err
		}
	}
	if sidecar != "" {
		if err := os.MkdirAll(sidecar, 0o700); err != nil {
			return err
		}
		for name, data := range sidecarFiles {
			if err := os.WriteFile(filepath.Join(sidecar, name), data, 0o600); err != nil {
				return err
			}
		}
	}
	writeLine(stdout, "catalogue written: "+line)
	return nil
}
