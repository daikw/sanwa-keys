// sanwa-keys programs the onboard memory of supported PCsensor/Sanwa devices.
package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/daikw/sanwa-keys/internal/device"
	"github.com/daikw/sanwa-keys/internal/protocol"
)

var version = "dev"

type connection interface {
	ReadAll() ([][]byte, error)
	Write(int, []byte) error
	Close() error
}
type backend struct {
	open func(string, string, time.Duration) (connection, error)
	list func() ([]device.Info, error)
}
type snapshot struct {
	Version int      `json:"version"`
	Model   string   `json:"model"`
	Records []string `json:"records"`
}

var slots = map[string]int{"400-MA214BK": 3, "400-SKB081": 6}

func main() {
	b := backend{list: device.List, open: func(m, p string, t time.Duration) (connection, error) { return device.Open(m, p, t) }}
	if e := run(os.Args[1:], os.Stdout, b); e != nil {
		fmt.Fprintln(os.Stderr, "sanwa-keys:", e)
		os.Exit(1)
	}
}

const usage = `sanwa-keys: configure onboard keys on macOS and Linux

  list
  read    [--model MODEL] [--device PATH] [--out FILE]
  set     [--model MODEL] --slot N --key ctrl+shift+a --backup FILE
  set     --model MODEL --slot N --key cmd+f13 --dry-run
  restore [--model MODEL] --from FILE --backup FILE
  version

Models: 400-MA214BK (3 pedals), 400-SKB081 (6 keys).
Model is inferred when exactly one supported device matches --device, if given.
Files contain device key records, not application settings or LED settings.
All device commands accept --device PATH and --timeout DURATION.
Use a new backup filename for each mutation. Writes are verified by readback.
`

func run(args []string, out io.Writer, b backend) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" {
		_, e := io.WriteString(out, usage)
		return e
	}
	command := args[0]
	if command == "version" {
		if len(args) != 1 {
			return errors.New("version takes no arguments")
		}
		_, e := fmt.Fprintln(out, version)
		return e
	}
	if command == "list" {
		if len(args) != 1 {
			return errors.New("list takes no arguments")
		}
		infos, e := b.list()
		if e != nil {
			return e
		}
		return encode(out, infos)
	}
	if command != "read" && command != "set" && command != "restore" {
		return errors.New("unknown command; use --help")
	}
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	fs.SetOutput(out)
	model := fs.String("model", "", "model name (inferred when one device matches)")
	path := fs.String("device", "", "path from list")
	// The default is the 1s response window used by the hardware validation;
	// users can extend it for a slow USB connection.
	timeout := fs.Duration("timeout", time.Second, "response timeout")
	var slot int
	var key, backup, from, dest string
	var dry bool
	switch command {
	case "read":
		fs.StringVar(&dest, "out", "", "save records to a new file")
	case "set":
		fs.IntVar(&slot, "slot", 0, "1-based slot")
		fs.StringVar(&key, "key", "", "modifiers + one key")
		fs.StringVar(&backup, "backup", "", "new backup file")
		fs.BoolVar(&dry, "dry-run", false, "encode without opening USB")
	case "restore":
		fs.StringVar(&from, "from", "", "snapshot to restore")
		fs.StringVar(&backup, "backup", "", "new backup file")
	}
	if e := fs.Parse(args[1:]); e != nil {
		if errors.Is(e, flag.ErrHelp) {
			return nil
		}
		return e
	}
	if fs.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	*model = strings.ToUpper(*model)
	if *model == "" {
		infos, err := b.list()
		if err != nil {
			return err
		}
		var selected device.Info
		matches := 0
		for _, info := range infos {
			if *path == "" || *path == info.Path {
				selected = info
				matches++
			}
		}
		if matches != 1 {
			return fmt.Errorf("cannot infer model: expected one supported device, found %d; use list, then select --device or --model", matches)
		}
		*model, *path = selected.Model, selected.Path
	}
	count, ok := slots[*model]
	if !ok {
		return errors.New("specify --model 400-MA214BK or 400-SKB081")
	}
	if *timeout < time.Millisecond || *timeout/time.Millisecond > time.Duration(1<<31-1) {
		return errors.New("timeout must be positive and fit HIDAPI milliseconds")
	}
	var desired []byte
	var target [][]byte
	if command == "set" {
		var e error
		desired, e = protocol.Key(key)
		if e != nil {
			return e
		}
		reports, e := protocol.WriteReports(slot, count, desired)
		if e != nil {
			return e
		}
		if dry {
			return encode(out, struct {
				Model    string   `json:"model"`
				Slot     int      `json:"slot"`
				Payloads []string `json:"payloads"`
			}{*model, slot, hexRecords(reports)})
		}
	}
	if command == "restore" {
		if from == "" {
			return errors.New("--from is required")
		}
		var e error
		target, e = loadSnapshot(from, *model, count)
		if e != nil {
			return e
		}
	}
	if command != "read" && backup == "" {
		return errors.New("--backup is required before changing device memory")
	}
	d, e := b.open(*model, *path, *timeout)
	if e != nil {
		return e
	}
	defer d.Close()
	before, e := d.ReadAll()
	if e != nil {
		return e
	}
	if e = validateRecords(before, count); e != nil {
		return e
	}
	if command == "read" {
		s := makeSnapshot(*model, before)
		if dest != "" {
			return saveNew(dest, s)
		}
		return encode(out, s)
	}
	if command == "set" {
		target = make([][]byte, len(before))
		copy(target, before)
		target[slot-1] = desired
	}
	for _, p := range before {
		if e = protocol.ValidateWritable(p); e != nil {
			return fmt.Errorf("current configuration cannot be safely restored: %w", e)
		}
	}
	if e = saveNew(backup, makeSnapshot(*model, before)); e != nil {
		return e
	}
	changed := 0
	for i, p := range target {
		if protocol.Equivalent(before[i], p) {
			continue
		}
		if e = d.Write(i+1, p); e != nil {
			return fmt.Errorf("write failed at slot %d; backup retained; device may be partially changed: %w", i+1, e)
		}
		changed++
	}
	after, e := d.ReadAll()
	if e != nil {
		return fmt.Errorf("verification failed; backup retained; device may be changed: %w", e)
	}
	if e = validateRecords(after, count); e != nil {
		return e
	}
	for i, p := range target {
		if !protocol.Equivalent(after[i], p) {
			return fmt.Errorf("verification mismatch at slot %d; backup retained; device may be changed", i+1)
		}
	}
	return encode(out, struct {
		Verified bool     `json:"verified"`
		Changed  int      `json:"changed_slots"`
		Snapshot snapshot `json:"snapshot"`
	}{true, changed, makeSnapshot(*model, after)})
}
func validateRecords(records [][]byte, count int) error {
	if len(records) != count {
		return errors.New("snapshot slot count does not match model")
	}
	for _, p := range records {
		if e := protocol.Validate(p); e != nil {
			return e
		}
	}
	return nil
}
func hexRecords(records [][]byte) []string {
	s := make([]string, len(records))
	for i, p := range records {
		s[i] = hex.EncodeToString(p)
	}
	return s
}
func makeSnapshot(model string, records [][]byte) snapshot {
	return snapshot{1, model, hexRecords(records)}
}
func encode(w io.Writer, v any) error {
	e := json.NewEncoder(w)
	e.SetIndent("", "  ")
	return e.Encode(v)
}
func saveNew(path string, s snapshot) error {
	f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return errors.New("cannot create backup/output file; choose a new writable filename")
	}
	e = encode(f, s)
	if e == nil {
		e = f.Sync()
	}
	closeErr := f.Close()
	if e != nil {
		return errors.New("could not finish saving snapshot; no device write started")
	}
	if closeErr != nil {
		return errors.New("could not close snapshot; no device write started")
	}
	return nil
}
func loadSnapshot(path, model string, count int) ([][]byte, error) {
	f, e := os.Open(path)
	if e != nil {
		return nil, errors.New("cannot open snapshot")
	}
	defer f.Close()
	var s snapshot
	maxRecords := make([]string, 6)
	for i := range maxRecords {
		maxRecords[i] = strings.Repeat("ff", 255)
	}
	envelope, _ := json.MarshalIndent(snapshot{1, "400-MA214BK", maxRecords}, "", "  ")
	raw, readErr := io.ReadAll(io.LimitReader(f, int64(len(envelope)+2)))
	if readErr != nil || len(raw) > len(envelope)+1 {
		return nil, errors.New("snapshot exceeds maximum encoded device records")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if dec.Decode(&s) != nil {
		return nil, errors.New("invalid snapshot JSON")
	}
	var extra any
	if dec.Decode(&extra) != io.EOF {
		return nil, errors.New("unexpected data after snapshot")
	}
	if s.Version != 1 || s.Model != model || len(s.Records) != count {
		return nil, errors.New("snapshot version, model or slot count mismatch")
	}
	result := make([][]byte, count)
	for i, str := range s.Records {
		if len(str) > 510 {
			return nil, errors.New("snapshot record too long")
		}
		p, e := hex.DecodeString(str)
		if e != nil || protocol.ValidateWritable(p) != nil {
			return nil, errors.New("invalid snapshot record")
		}
		result[i] = p
	}
	return result, nil
}
