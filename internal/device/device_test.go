package device

import (
	"bytes"
	"github.com/sstallion/go-hid"
	"testing"
	"time"
)

type fake struct {
	incoming [][]byte
	writes   [][]byte
	delay    time.Duration
	short    bool
}

func (f *fake) Write(p []byte) (int, error) {
	f.writes = append(f.writes, append([]byte(nil), p...))
	if f.short {
		return len(p) - 1, nil
	}
	return len(p), nil
}
func (f *fake) ReadWithTimeout(p []byte, t time.Duration) (int, error) {
	if f.delay > 0 {
		time.Sleep(f.delay)
	}
	if len(f.incoming) == 0 {
		return 0, hid.ErrTimeout
	}
	b := f.incoming[0]
	f.incoming = f.incoming[1:]
	return copy(p, b), nil
}
func (f *fake) Close() error { return nil }
func TestReadFraming(t *testing.T) {
	for _, numbered := range []bool{false, true} {
		t.Run(map[bool]string{false: "unnumbered", true: "numbered"}[numbered], func(t *testing.T) {
			f := &fake{}
			payload := []byte{4, 1, 0, 4, 0, 0, 0, 0}
			ack := []byte{0x81, 0x55, 0, 0, 0, 0, 0, 0}
			if numbered {
				payload = append([]byte{1}, payload...)
				ack = append([]byte{1}, ack...)
			}
			f.incoming = [][]byte{ack, payload}
			d := &Device{io: f, numbered: numbered, timeout: time.Second}
			got, err := d.readRecord(time.Now().Add(time.Second))
			if err != nil || !bytes.Equal(got, []byte{4, 1, 0, 4}) {
				t.Fatalf("%x %v", got, err)
			}
		})
	}
}
func TestRejectBadReports(t *testing.T) {
	for _, b := range [][]byte{{1, 4, 1, 0, 4}, {2, 4, 1, 0, 4, 0, 0, 0, 0}, {1, 1, 1, 0, 4, 0, 0, 0, 0}, {1, 12, 1, 0, 4, 0, 0, 0, 0}} {
		f := &fake{incoming: [][]byte{b}}
		d := &Device{io: f, numbered: true}
		if _, err := d.readRecord(time.Now().Add(time.Second)); err == nil {
			t.Fatalf("accepted %x", b)
		}
	}
}
func TestACKDeadline(t *testing.T) {
	f := &fake{delay: 2 * time.Millisecond}
	for range 10 {
		f.incoming = append(f.incoming, []byte{0x81, 0x55, 0, 0, 0, 0, 0, 0})
	}
	d := &Device{io: f}
	if _, err := d.readRecord(time.Now().Add(time.Millisecond)); err == nil {
		t.Fatal("expected timeout")
	}
	if len(f.incoming) < 9 {
		t.Fatal("deadline restarted after ACK")
	}
}
func TestWriteFramingAndShort(t *testing.T) {
	for _, numbered := range []bool{false, true} {
		f := &fake{}
		d := &Device{io: f, numbered: numbered}
		b := []byte{1, 0x82, 8, 1, 0, 0, 0, 0}
		if err := d.send(b); err != nil {
			t.Fatal(err)
		}
		want := b
		if numbered {
			want = append([]byte{1}, b...)
		}
		if !bytes.Equal(want, f.writes[0]) {
			t.Fatal(f.writes)
		}
		f.short = true
		if d.send(b) == nil {
			t.Fatal("short write accepted")
		}
	}
}

func TestSelectedSlotOnly(t *testing.T) {
	f := &fake{}
	d := &Device{Info: Info{Slots: 6}, io: f, numbered: true}
	if err := d.Write(4, []byte{4, 1, 0, 7}); err != nil {
		t.Fatal(err)
	}
	want := [][]byte{{1, 1, 0x81, 4, 4, 0, 0, 0, 0}, {1, 4, 1, 0, 7, 0, 0, 0, 0}}
	if len(f.writes) != len(want) {
		t.Fatal(f.writes)
	}
	for i := range want {
		if !bytes.Equal(f.writes[i], want[i]) {
			t.Fatalf("report %d: %x", i, f.writes[i])
		}
	}
	for _, slot := range []int{0, 7} {
		if err := d.Write(slot, []byte{4, 1, 0, 7}); err == nil {
			t.Fatal("invalid slot accepted")
		}
	}
	if len(f.writes) != 2 {
		t.Fatal("invalid slot caused writes")
	}
}
func TestDrain(t *testing.T) {
	f := &fake{incoming: [][]byte{{0x81, 0x55, 0, 0, 0, 0, 0, 0}, {4, 1, 0, 7, 0, 0, 0, 0}}}
	d := &Device{io: f}
	if err := d.drain(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if len(f.incoming) != 0 || len(f.writes) != 0 {
		t.Fatal("drain failed")
	}
}
func TestIdentity(t *testing.T) {
	base := hid.DeviceInfo{VendorID: 0x3553, ProductID: 0xb001, InterfaceNbr: 1, UsagePage: 1, Usage: 0}
	if _, ok := match(&base); !ok {
		t.Fatal("valid identity rejected")
	}
	for _, mutate := range []func(*hid.DeviceInfo){func(i *hid.DeviceInfo) { i.VendorID++ }, func(i *hid.DeviceInfo) { i.ProductID++ }, func(i *hid.DeviceInfo) { i.InterfaceNbr = 0 }, func(i *hid.DeviceInfo) { i.UsagePage++ }, func(i *hid.DeviceInfo) { i.Usage++ }} {
		i := base
		mutate(&i)
		if _, ok := match(&i); ok {
			t.Fatalf("accepted %+v", i)
		}
	}
}
