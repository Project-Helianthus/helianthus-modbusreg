package observe

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestTeslaObservePublicationSyncsParentDirectory(t *testing.T) {
	for _, replace := range []bool{false, true} {
		for _, failSync := range []bool{false, true} {
			t.Run(fmt.Sprintf("replace=%t/failSync=%t", replace, failSync), func(t *testing.T) {
				dir := t.TempDir()
				path := filepath.Join(dir, "evidence.json")
				if replace {
					if err := os.WriteFile(path, []byte("prior evidence"), 0o600); err != nil {
						t.Fatal(err)
					}
				}
				original := teslaObserveSyncDirectory
				t.Cleanup(func() { teslaObserveSyncDirectory = original })
				syncFailure := errors.New("synthetic directory sync failure")
				calls := 0
				data := []byte("complete evidence")
				teslaObserveSyncDirectory = func(parent string) error {
					calls++
					if parent != dir {
						t.Fatalf("sync directory %q, want %q", parent, dir)
					}
					got, err := os.ReadFile(path)
					if err != nil || !bytes.Equal(got, data) {
						t.Fatalf("sync before publication: data=%q error=%v", got, err)
					}
					if failSync {
						return syncFailure
					}
					return original(parent)
				}
				err := writeTeslaObserveFile(path, data, replace)
				if calls != 1 {
					t.Fatalf("directory sync calls = %d, want 1", calls)
				}
				if (failSync && !errors.Is(err, syncFailure)) || (!failSync && err != nil) {
					t.Fatalf("publication error = %v, failSync=%t", err, failSync)
				}
				got, readErr := os.ReadFile(path)
				if readErr != nil || !bytes.Equal(got, data) {
					t.Fatalf("published evidence lost after sync: data=%q error=%v", got, readErr)
				}
			})
		}
	}
}

func TestTeslaObserveDirectorySyncFailureFailsRun(t *testing.T) {
	original := teslaObserveSyncDirectory
	t.Cleanup(func() { teslaObserveSyncDirectory = original })
	teslaObserveSyncDirectory = func(string) error { return syscall.EIO }
	ports := &teslaObserveTestPorts{}
	if code := Run(teslaObserveArgs(t.TempDir()), ports.build()); code != teslaObserveExitRun {
		t.Fatalf("directory sync failure exit = %d: %s", code, ports.stderr.String())
	}
}
