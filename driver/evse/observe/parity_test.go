package observe

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"syscall"
	"testing"
	"time"
)

type fixedCase struct {
	Name  string
	Argv  []string
	Clock time.Time
	Wire  []byte
	Seed  bool
}

type fixedFile struct {
	Name string
	Data []byte
	Mode fs.FileMode
}

type fixedResult struct {
	Status                  int
	Stderr                  []byte
	Opened                  []string
	Reads, Closes, Contexts int
	Files                   []fixedFile
}

type fixedRecording struct {
	Case     fixedCase
	Expected fixedResult
}

type fixedSource struct {
	result *fixedResult
	wire   []byte
}

func (s *fixedSource) ReceiveByte(_ context.Context) (byte, error) {
	s.result.Reads++
	if len(s.wire) == 0 {
		return 0, syscall.EIO
	}
	b := s.wire[0]
	s.wire = s.wire[1:]
	return b, nil
}

func (s *fixedSource) Close() error { s.result.Closes++; return nil }

func recordFixedCase(t *testing.T, c fixedCase) fixedResult {
	t.Helper()
	home, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(home); err != nil {
			t.Fatal(err)
		}
	}()
	var result fixedResult
	var stderr bytes.Buffer
	ports := Ports{
		OpenSerial: func(path string) (ByteSource, error) {
			result.Opened = append(result.Opened, path)
			return &fixedSource{&result, c.Wire}, nil
		},
		Now:    func() time.Time { return c.Clock },
		Stderr: &stderr,
		Context: func() (context.Context, context.CancelFunc) {
			result.Contexts++
			return context.WithCancel(context.Background())
		},
	}
	if c.Seed {
		seed := append([]string(nil), c.Argv[2:len(c.Argv)-1]...)
		if status := Run(seed, ports); status != 0 {
			t.Fatalf("seed status %d: %s", status, stderr.Bytes())
		}
		stderr.Reset()
		result = fixedResult{}
	}
	write, publish, syncDir := teslaObserveWriteData, teslaObservePublishFile, teslaObserveSyncDirectory
	defer func() {
		teslaObserveWriteData, teslaObservePublishFile, teslaObserveSyncDirectory = write, publish, syncDir
	}()
	switch c.Name {
	case "WorksheetStage":
		teslaObserveWriteData = func(file *os.File, data []byte) error {
			if bytes.HasPrefix(data, []byte("#")) {
				return syscall.EIO
			}
			return write(file, data)
		}
	case "EvidencePublish", "WorksheetPublishNew", "WorksheetPublishAppend":
		target := "worksheet.md"
		if c.Name == "EvidencePublish" {
			target = "evidence.json"
		}
		teslaObservePublishFile = func(staged, path string, replace bool) error {
			if filepath.Base(path) == target {
				return syscall.EIO
			}
			return publish(staged, path, replace)
		}
	case "DirectorySync":
		teslaObserveSyncDirectory = func(string) error { return syscall.EIO }
	}
	result.Status, result.Stderr = Run(c.Argv[2:], ports), stderr.Bytes()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		result.Files = append(result.Files, fixedFile{entry.Name(), data, info.Mode()})
	}
	return result
}

func TestTeslaObserveFixedClockDonorParity(t *testing.T) {
	var recordings []fixedRecording
	indexPath := os.Getenv("TESLA_OBSERVE_DONOR_INDEX")
	if indexPath == "" {
		readFixedJSON(t, "testdata/fixed-clock.json", &recordings)
		if len(recordings) != 9 {
			t.Fatalf("fixed replay has %d cases, want 9", len(recordings))
		}
	} else {
		var index struct {
			Revision string
			Cases    []struct {
				Name, Path string
				Digest     string `json:"SHA256"`
			}
		}
		readFixedJSON(t, indexPath, &index)
		if len(index.Revision) != 40 || len(index.Cases) != 10 {
			t.Fatal("incomplete indexed replay")
		}
		for _, entry := range index.Cases {
			var recording fixedRecording
			sum := readFixedJSON(t, filepath.Join(filepath.Dir(indexPath), entry.Path), &recording)
			if recording.Case.Name != entry.Name || sum != entry.Digest {
				t.Fatalf("fixed replay operand mismatch: %s", entry.Name)
			}
			recordings = append(recordings, recording)
		}
	}
	for _, recording := range recordings {
		t.Run(recording.Case.Name, func(t *testing.T) {
			actual := recordFixedCase(t, recording.Case)
			if !reflect.DeepEqual(actual, recording.Expected) {
				t.Errorf("runner result: status %d, stderr %q, opens %v, reads %d, closes %d, contexts %d", actual.Status, actual.Stderr, actual.Opened, actual.Reads, actual.Closes, actual.Contexts)
				for i, file := range actual.Files {
					if i >= len(recording.Expected.Files) || !reflect.DeepEqual(file, recording.Expected.Files[i]) {
						t.Errorf("output differs: %s mode %v digest %x", file.Name, file.Mode, sha256.Sum256(file.Data))
					}
				}
				if len(actual.Files) != len(recording.Expected.Files) {
					t.Errorf("file count: got %d want %d", len(actual.Files), len(recording.Expected.Files))
				}
			}
		})
	}
}

func readFixedJSON(t *testing.T, path string, dst any) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err == nil {
		err = json.Unmarshal(data, dst)
	}
	if err != nil {
		t.Fatal(fmt.Errorf("fixed replay: %w", err))
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
