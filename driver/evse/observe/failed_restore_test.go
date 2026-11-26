package observe

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// The worksheet publication fails and the filesystem also refuses the restore of
// the prior evidence: the command ends with status 1, the joined errors reach
// stderr, and the backup stays as the recovery copy with the prior bytes.
func TestTeslaObserveFailedRestoreKeepsRecoveryCopy(t *testing.T) {
	dir := t.TempDir()
	out, worksheet := filepath.Join(dir, "evidence.json"), filepath.Join(dir, "worksheet.md")
	if code := Run(teslaObserveArgs(dir), (&teslaObserveTestPorts{}).build()); code != teslaObserveExitOK {
		t.Fatalf("seed exit %d", code)
	}
	prior, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	priorWorksheet, err := os.ReadFile(worksheet)
	if err != nil {
		t.Fatal(err)
	}
	publish := teslaObservePublishFile
	t.Cleanup(func() { teslaObservePublishFile = publish })
	publishCause := &os.LinkError{Op: "link", Old: "staged-worksheet", New: "worksheet.md", Err: syscall.EIO}
	teslaObservePublishFile = func(staged, path string, replace bool) error {
		if filepath.Base(path) != "worksheet.md" {
			return publish(staged, path, replace)
		}
		if err := os.Remove(out); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Join(out, "occupied"), 0o700); err != nil {
			return err
		}
		return publishCause
	}
	ports := &teslaObserveTestPorts{}
	if code := Run(teslaObserveArgs(dir, "-append"), ports.build()); code != teslaObserveExitRun {
		t.Fatalf("exit %d: %s", code, ports.stderr.String())
	}
	stderr := ports.stderr.String()
	lines := strings.Split(strings.TrimSuffix(stderr, "\n"), "\n")
	if len(lines) != 2 || !strings.HasPrefix(lines[0], "tesla-observe: writing evidence failed: "+publishCause.Error()) ||
		!strings.HasPrefix(lines[1], "rename ") || !strings.Contains(lines[1], " evidence.json") && !strings.Contains(lines[1], out) {
		t.Fatalf("stderr %q", stderr)
	}
	if strings.Contains(stderr, teslaObserveTestPath) {
		t.Fatalf("stderr carries the serial path: %q", stderr)
	}
	recovery, err := filepath.Glob(filepath.Join(dir, ".tesla-observe-*"))
	if err != nil || len(recovery) != 1 {
		t.Fatalf("recovery copies %v: %v", recovery, err)
	}
	data, err := os.ReadFile(recovery[0])
	if err != nil || !bytes.Equal(data, prior) {
		t.Fatalf("recovery copy bytes differ: %v", err)
	}
	info, err := os.Stat(recovery[0])
	if err != nil || info.Mode() != 0o600 {
		t.Fatalf("recovery copy mode %v: %v", info.Mode(), err)
	}
	if got, err := os.ReadFile(worksheet); err != nil || !bytes.Equal(got, priorWorksheet) {
		t.Fatalf("worksheet changed: %v", err)
	}
}
