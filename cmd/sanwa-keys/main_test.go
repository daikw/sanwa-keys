package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type fakeDevice struct {
	records  [][]byte
	writes   int
	backup   string
	mismatch bool
}

func (f *fakeDevice) ReadAll() ([][]byte, error) {
	out := make([][]byte, len(f.records))
	for i, p := range f.records {
		out[i] = bytes.Clone(p)
	}
	return out, nil
}
func (f *fakeDevice) Close() error { return nil }
func (f *fakeDevice) Write(slot int, p []byte) error {
	if _, e := os.Stat(f.backup); e != nil {
		return errors.New("backup missing before write")
	}
	f.writes++
	if !f.mismatch {
		f.records[slot-1] = bytes.Clone(p)
	}
	return nil
}
func setup(t *testing.T) (*fakeDevice, backend) {
	t.Helper()
	f := &fakeDevice{records: [][]byte{{8, 1, 0, 4, 0, 0, 0, 0}, {8, 1, 0, 5, 0, 0, 0, 0}, {8, 1, 0, 6, 0, 0, 0, 0}}, backup: filepath.Join(t.TempDir(), "backup.json")}
	b := backend{open: func(string, string, time.Duration) (connection, error) { return f, nil }}
	return f, b
}
func TestSetBacksUpAndPreservesOtherSlots(t *testing.T) {
	f, b := setup(t)
	var out bytes.Buffer
	e := run([]string{"set", "--model", "400-MA214BK", "--slot", "2", "--key", "cmd+f13", "--backup", f.backup}, &out, b)
	if e != nil {
		t.Fatal(e)
	}
	if f.writes != 1 || f.records[0][3] != 4 || f.records[1][3] != 104 || f.records[2][3] != 6 {
		t.Fatalf("bad write: %x", f.records)
	}
	raw, e := os.ReadFile(f.backup)
	if e != nil {
		t.Fatal(e)
	}
	var s snapshot
	if e = json.Unmarshal(raw, &s); e != nil {
		t.Fatal(e)
	}
	if s.Records[1] != hex.EncodeToString([]byte{8, 1, 0, 5, 0, 0, 0, 0}) {
		t.Fatal(s)
	}
	info, _ := os.Stat(f.backup)
	if info.Mode().Perm() != 0600 {
		t.Fatal(info.Mode())
	}
}
func TestRefuseOverwriteBackup(t *testing.T) {
	f, b := setup(t)
	os.WriteFile(f.backup, []byte("keep"), 0600)
	e := run([]string{"set", "--model", "400-MA214BK", "--slot", "1", "--key", "f13", "--backup", f.backup}, &bytes.Buffer{}, b)
	if e == nil || f.writes != 0 {
		t.Fatal("overwrote backup or device")
	}
}
func TestMismatchIsFailure(t *testing.T) {
	f, b := setup(t)
	f.mismatch = true
	e := run([]string{"set", "--model", "400-MA214BK", "--slot", "1", "--key", "f13", "--backup", f.backup}, &bytes.Buffer{}, b)
	if e == nil {
		t.Fatal("mismatch reported success")
	}
}
func TestInvalidAndDryRunNeverOpen(t *testing.T) {
	b := backend{open: func(string, string, time.Duration) (connection, error) { t.Fatal("opened device"); return nil, nil }}
	for _, args := range [][]string{{"set", "--model", "400-MA214BK", "--slot", "4", "--key", "a", "--backup", "x"}, {"set", "--model", "wrong", "--slot", "1", "--key", "a", "--backup", "x"}, {"set", "--model", "400-MA214BK", "--slot", "1", "--key", "a"}, {"set", "--model", "400-MA214BK", "--slot", "1", "--key", "a", "--backup", "x", "junk"}} {
		if run(args, &bytes.Buffer{}, b) == nil {
			t.Fatalf("accepted %v", args)
		}
	}
	if e := run([]string{"set", "--model", "400-SKB081", "--slot", "6", "--key", "ctrl+a", "--dry-run"}, &bytes.Buffer{}, b); e != nil {
		t.Fatal(e)
	}
}

func TestRestoreAndNoop(t *testing.T) {
	f, b := setup(t)
	source := filepath.Join(t.TempDir(), "source.json")
	want := [][]byte{{8, 1, 8, 104, 0, 0, 0, 0}, {8, 1, 0, 5, 0, 0, 0, 0}, {8, 1, 1, 6, 0, 0, 0, 0}}
	if e := saveNew(source, makeSnapshot("400-MA214BK", want)); e != nil {
		t.Fatal(e)
	}
	args := []string{"restore", "--model", "400-MA214BK", "--from", source, "--backup", f.backup}
	if e := run(args, &bytes.Buffer{}, b); e != nil {
		t.Fatal(e)
	}
	if f.writes != 2 {
		t.Fatal(f.writes)
	}
	saved, e := loadSnapshot(f.backup, "400-MA214BK", 3)
	if e != nil || saved[0][3] != 4 {
		t.Fatalf("original not saved: %x %v", saved, e)
	}
	f.backup = filepath.Join(t.TempDir(), "noop.json")
	args[len(args)-1] = f.backup
	if e = run(args, &bytes.Buffer{}, b); e != nil {
		t.Fatal(e)
	}
	if f.writes != 2 {
		t.Fatal("no-op issued writes")
	}
}
func TestMalformedSnapshotsNeverOpen(t *testing.T) {
	b := backend{open: func(string, string, time.Duration) (connection, error) {
		t.Fatal("opened on invalid input")
		return nil, nil
	}}
	for _, raw := range []string{`{}`, `{"version":2,"model":"400-MA214BK","records":[]}`, `{"version":1,"model":"400-SKB081","records":[]}`, `{"version":1,"model":"400-MA214BK","records":["02ff","02ff","02ff"]}`, `{"version":1,"model":"400-MA214BK","records":["zz","zz","zz"]}`, `{"version":1,"model":"400-MA214BK","records":["04010004","04010004","04010004"]} {}`, string(bytes.Repeat([]byte(" "), 10000))} {
		p := filepath.Join(t.TempDir(), "invalid.json")
		os.WriteFile(p, []byte(raw), 0600)
		if e := run([]string{"restore", "--model", "400-MA214BK", "--from", p, "--backup", "unused"}, &bytes.Buffer{}, b); e == nil {
			t.Fatal("accepted malformed snapshot")
		}
	}
}

type failingDevice struct {
	*fakeDevice
	attempts int
}

func (f *failingDevice) Write(slot int, p []byte) error {
	f.attempts++
	if f.attempts == 2 {
		return errors.New("disconnected")
	}
	return f.fakeDevice.Write(slot, p)
}
func TestPartialWriteStopsAndKeepsBackup(t *testing.T) {
	f, _ := setup(t)
	d := &failingDevice{fakeDevice: f}
	b := backend{open: func(string, string, time.Duration) (connection, error) { return d, nil }}
	src := filepath.Join(t.TempDir(), "restore.json")
	saveNew(src, makeSnapshot("400-MA214BK", [][]byte{{4, 1, 0, 104}, {4, 1, 0, 105}, {4, 1, 0, 106}}))
	e := run([]string{"restore", "--model", "400-MA214BK", "--from", src, "--backup", f.backup}, &bytes.Buffer{}, b)
	if e == nil || d.attempts != 2 || f.records[2][3] != 6 {
		t.Fatalf("did not stop safely: %v %+v", e, d)
	}
	if _, e = loadSnapshot(f.backup, "400-MA214BK", 3); e != nil {
		t.Fatal(e)
	}
}
