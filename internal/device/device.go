// Package device accesses only the verified configuration interfaces of supported Sanwa devices.
package device

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/daikw/sanwa-keys/internal/protocol"
	"github.com/sstallion/go-hid"
)

type Info struct {
	Model string `json:"model"`
	Path  string `json:"path"`
	Slots int    `json:"slots"`
}
type transport interface {
	Write([]byte) (int, error)
	ReadWithTimeout([]byte, time.Duration) (int, error)
	Close() error
}
type Device struct {
	Info     Info
	io       transport
	numbered bool
	timeout  time.Duration
}
type modelSpec struct {
	name       string
	pid        uint16
	slots      int
	descriptor string
}

var models = []modelSpec{
	{"400-MA214BK", 0xb001, 3, "05010900a1010901150025ff95087508810209019102c0"},
	{"400-SKB081", 0xc115, 6, "05010900a10185010901150025ff95087508810209019102c005010900a10185020901150025ff950f75088102c0"},
}

func match(i *hid.DeviceInfo) (modelSpec, bool) {
	if i == nil || i.VendorID != 0x3553 || i.InterfaceNbr != 1 || i.UsagePage != 1 || i.Usage != 0 {
		return modelSpec{}, false
	}
	for _, m := range models {
		if i.ProductID == m.pid {
			return m, true
		}
	}
	return modelSpec{}, false
}
func List() ([]Info, error) {
	out := []Info{}
	seen := map[string]bool{}
	err := hid.Enumerate(0x3553, 0, func(i *hid.DeviceInfo) error {
		m, ok := match(i)
		if ok && !seen[i.Path] {
			seen[i.Path] = true
			out = append(out, Info{m.name, i.Path, m.slots})
		}
		return nil
	})
	return out, err
}
func Open(model, path string, timeout time.Duration) (*Device, error) {
	if timeout < time.Millisecond || timeout/time.Millisecond > 2147483647 {
		return nil, fmt.Errorf("timeout must be between 1ms and 2147483647ms")
	}
	candidates, err := List()
	if err != nil {
		return nil, err
	}
	selected := []Info{}
	for _, i := range candidates {
		if (model == "" || model == i.Model) && (path == "" || path == i.Path) {
			selected = append(selected, i)
		}
	}
	if len(selected) != 1 {
		return nil, fmt.Errorf("expected one supported device, found %d; select --model and --device", len(selected))
	}
	prepareOpen()
	h, err := hid.OpenPath(selected[0].Path)
	if err != nil {
		return nil, err
	}
	valid := false
	defer func() {
		if !valid {
			h.Close()
		}
	}()
	current, err := h.GetDeviceInfo()
	if err != nil {
		return nil, err
	}
	m, ok := match(current)
	if !ok || m.name != selected[0].Model || current.Path != selected[0].Path {
		return nil, fmt.Errorf("opened device identity does not match selection")
	}
	descriptor := make([]byte, 4096) // HID_API_MAX_REPORT_DESCRIPTOR_SIZE from HIDAPI.
	n, err := h.GetReportDescriptor(descriptor)
	if err != nil {
		return nil, err
	}
	expected, _ := hex.DecodeString(m.descriptor)
	if n < 0 || n > len(descriptor) || !bytes.Equal(descriptor[:n], expected) {
		return nil, fmt.Errorf("unsupported HID report descriptor")
	}
	valid = true
	return &Device{Info: selected[0], io: h, numbered: m.pid == 0xc115, timeout: timeout}, nil
}
func (d *Device) Close() error { return d.io.Close() }
func (d *Device) send(p []byte) error {
	if len(p) != 8 {
		return fmt.Errorf("configuration output must have 8 payload bytes")
	}
	if d.numbered {
		p = append([]byte{1}, p...)
	}
	n, err := d.io.Write(p)
	if err != nil {
		return err
	}
	if n != len(p) {
		return fmt.Errorf("short HID write: %d of %d bytes", n, len(p))
	}
	return nil
}
func (d *Device) receive(wait time.Duration) ([]byte, error) {
	// A full HID-sized buffer detects unexpected report lengths instead of truncating them.
	b := make([]byte, 64)
	n, err := d.io.ReadWithTimeout(b, wait)
	if err != nil {
		return nil, err
	}
	want := 8
	if d.numbered {
		want = 9
	}
	if n != want {
		return nil, fmt.Errorf("unexpected HID input length: %d", n)
	}
	b = b[:n]
	if d.numbered {
		if b[0] != 1 {
			return nil, fmt.Errorf("unexpected HID input report ID: %d", b[0])
		}
		b = b[1:]
	}
	return b, nil
}
func (d *Device) drain(deadline time.Time) error {
	for {
		if !time.Now().Before(deadline) {
			return fmt.Errorf("timed out draining pending HID reports")
		}
		_, err := d.receive(0)
		if errors.Is(err, hid.ErrTimeout) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}
func (d *Device) readRecord(deadline time.Time) ([]byte, error) {
	var record []byte
	length := 0
	for {
		remaining := time.Until(deadline)
		if remaining < time.Millisecond {
			return nil, fmt.Errorf("configuration read timed out")
		}
		p, err := d.receive(remaining)
		if err != nil {
			return nil, err
		}
		if len(record) == 0 && p[0] == 0x81 && p[1] == 0x55 {
			continue
		}
		if len(record) == 0 {
			length = int(p[0])
			if length < 2 {
				return nil, fmt.Errorf("invalid configuration record length")
			}
		}
		take := min(8, length-len(record))
		record = append(record, p[:take]...)
		if len(record) == length {
			if err := protocol.Validate(record); err != nil {
				return nil, err
			}
			return record, nil
		}
	}
}
func (d *Device) ReadAll() ([][]byte, error) {
	result := make([][]byte, 0, d.Info.Slots)
	for slot := 1; slot <= d.Info.Slots; slot++ {
		deadline := time.Now().Add(d.timeout)
		if err := d.drain(deadline); err != nil {
			return nil, fmt.Errorf("slot %d: %w", slot, err)
		}
		q, err := protocol.Query(slot, d.Info.Slots)
		if err != nil {
			return nil, err
		}
		if err = d.send(q); err != nil {
			return nil, err
		}
		p, err := d.readRecord(deadline)
		if err != nil {
			return nil, fmt.Errorf("slot %d: %w", slot, err)
		}
		result = append(result, p)
	}
	return result, nil
}
func (d *Device) Write(slot int, record []byte) error {
	reports, err := protocol.WriteReports(slot, d.Info.Slots, record)
	if err != nil {
		return err
	}
	for _, p := range reports {
		if err = d.send(p); err != nil {
			return err
		} // PCsensor footswitch reference requires 30ms after each programming report.
		time.Sleep(30 * time.Millisecond)
	}
	return nil
}
