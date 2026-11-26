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
	recovery, err := filepath.Glob(filepath.Join(dir, ".tesla-observe-*"))
	if err != nil || len(recovery) != 1 {
		t.Fatalf("recovery copies %v: %v", recovery, err)
	}
	// The restore the command attempted is the rename of the recovery copy onto
	// the evidence path, now an occupied directory; repeating it fails with the
	// same cause and leaves everything as it is. No further cause is joined: the
	// directory sync succeeds.
	// The command resolves its directories, so the probe resolves them too.
	resolvedDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	restoreErr := os.Rename(filepath.Join(resolvedDir, filepath.Base(recovery[0])), filepath.Join(resolvedDir, filepath.Base(out)))
	if restoreErr == nil {
		t.Fatal("the restore was expected to fail")
	}
	if want := "tesla-observe: writing evidence failed: " + publishCause.Error() + "\n" + restoreErr.Error() + "\n"; stderr != want {
		t.Fatalf("stderr %q, want %q", stderr, want)
	}
	if strings.Contains(stderr, teslaObserveTestPath) {
		t.Fatalf("stderr carries the serial path: %q", stderr)
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
