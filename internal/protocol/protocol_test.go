package protocol

import (
	"bytes"
	"testing"
)

func TestKey(t *testing.T) {
	for _, tt := range []struct {
		in   string
		want []byte
	}{
		{"ctrl+shift+a", []byte{8, 1, 3, 4, 0, 0, 0, 0}},
		{"cmd+f13", []byte{8, 1, 8, 104, 0, 0, 0, 0}},
		{"ralt", []byte{8, 1, 64, 0, 0, 0, 0, 0}},
	} {
		got, err := Key(tt.in)
		if err != nil || !bytes.Equal(got, tt.want) {
			t.Errorf("Key(%q)=%x,%v want %x", tt.in, got, err, tt.want)
		}
	}
	for _, s := range []string{"", "ctrl+", "a+b", "f25", "ctrl+ctrl+a", "a\n"} {
		if _, err := Key(s); err == nil {
			t.Errorf("accepted %q", s)
		}
	}
}

func TestReports(t *testing.T) {
	p := []byte{8, 1, 8, 104, 0, 0, 0, 0}
	got, err := WriteReports(2, 3, p)
	want := [][]byte{{1, 0x81, 8, 2, 0, 0, 0, 0}, p}
	if err != nil || len(got) != 2 {
		t.Fatalf("%x %v", got, err)
	}
	for i := range want {
		if !bytes.Equal(got[i], want[i]) {
			t.Errorf("report %d=%x", i, got[i])
		}
	}
	for _, n := range []int{0, 4} {
		if _, err := WriteReports(n, 3, p); err == nil {
			t.Errorf("accepted slot %d", n)
		}
	}
	for _, b := range [][]byte{nil, {8, 1}, {0, 0, 0, 0, 0, 0, 0, 0}, {9, 1, 0, 0, 0, 0, 0, 0}} {
		if _, err := WriteReports(1, 3, b); err == nil {
			t.Errorf("accepted data %x", b)
		}
	}
}

func TestRejectUnknownWritableRecords(t *testing.T) {
	for _, p := range [][]byte{{2, 255}, {3, 1, 0}, {8, 1, 0, 4, 1, 0, 0, 0}, {8, 0, 1, 0, 0, 0, 0, 0}} {
		if _, e := WriteReports(1, 3, p); e == nil {
			t.Errorf("accepted unsupported record %x", p)
		}
	}
}
func TestEquivalentKeyNormalization(t *testing.T) {
	if !Equivalent([]byte{4, 1, 8, 104}, []byte{8, 1, 8, 104, 0, 0, 0, 0}) {
		t.Fatal("normalization rejected")
	}
	if Equivalent([]byte{4, 1, 8, 104}, []byte{4, 1, 8, 105}) {
		t.Fatal("different keys equivalent")
	}
}
