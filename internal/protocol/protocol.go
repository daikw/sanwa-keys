// Package protocol encodes PCsensor configuration payloads, without HID framing.
package protocol

import (
	"fmt"
	"strconv"
	"strings"
)

var modifiers = map[string]byte{"ctrl": 1, "lctrl": 1, "shift": 2, "lshift": 2, "alt": 4, "lalt": 4, "cmd": 8, "super": 8, "win": 8, "lmeta": 8, "rctrl": 16, "rshift": 32, "ralt": 64, "rcmd": 128, "rmeta": 128}
var keys = map[string]byte{"enter": 40, "escape": 41, "esc": 41, "backspace": 42, "tab": 43, "space": 44, "minus": 45, "equal": 46, "leftbracket": 47, "rightbracket": 48, "backslash": 49, "semicolon": 51, "quote": 52, "grave": 53, "comma": 54, "period": 55, "slash": 56, "capslock": 57, "printscreen": 70, "scrolllock": 71, "pause": 72, "insert": 73, "home": 74, "pageup": 75, "delete": 76, "end": 77, "pagedown": 78, "right": 79, "left": 80, "down": 81, "up": 82, "numlock": 83}

// Key accepts modifiers and one USB keyboard usage, independent of OS layout.
func Key(s string) ([]byte, error) {
	p := []byte{8, 1, 0, 0, 0, 0, 0, 0}
	for _, part := range strings.Split(strings.ToLower(s), "+") {
		if m, ok := modifiers[part]; ok {
			if p[2]&m != 0 {
				return nil, fmt.Errorf("duplicate modifier")
			}
			p[2] |= m
			continue
		}
		var k byte
		if len(part) == 1 && part[0] >= 'a' && part[0] <= 'z' {
			k = part[0] - 'a' + 4
		} else if len(part) == 1 && part[0] >= '1' && part[0] <= '9' {
			k = part[0] - '1' + 30
		} else if part == "0" {
			k = 39
		} else if strings.HasPrefix(part, "f") {
			n, e := strconv.Atoi(part[1:])
			if e == nil && n >= 1 && n <= 24 {
				if n <= 12 {
					k = byte(n + 57)
				} else {
					k = byte(n + 91)
				}
			}
		} else {
			k = keys[part]
		}
		if k == 0 || p[3] != 0 {
			return nil, fmt.Errorf("expected modifiers and one supported key")
		}
		p[3] = k
	}
	return p, nil
}

func Validate(p []byte) error {
	// Length is one byte; the first two bytes are record length and action type.
	if len(p) < 2 || len(p) > 255 || int(p[0]) != len(p) {
		return fmt.Errorf("invalid configuration record length")
	}
	return nil
}

// ValidateWritable accepts only action formats established by protocol sources.
// Unknown macros can be read for diagnosis, but cannot be safely replayed.
func ValidateWritable(p []byte) error {
	if e := Validate(p); e != nil {
		return e
	}
	switch p[1] {
	case 0:
		if len(p) == 8 && strings.Trim(string(p[2:]), "\x00") == "" {
			return nil
		}
	case 1, 0x81:
		if (len(p) == 4 || len(p) == 8) && strings.Trim(string(p[4:]), "\x00") == "" {
			return nil
		}
	case 2, 3:
		if len(p) == 8 && p[4] <= 7 {
			return nil
		}
	case 4:
		if len(p) >= 3 && len(p) <= 40 {
			return nil
		}
	}
	return fmt.Errorf("unsupported configuration action or shape; refusing replay")
}

func Query(slot, count int) ([]byte, error) {
	if slot < 1 || slot > count {
		return nil, fmt.Errorf("slot must be between 1 and %d", count)
	}
	return []byte{1, 0x82, 8, byte(slot), 0, 0, 0, 0}, nil
}

func WriteReports(slot, count int, p []byte) ([][]byte, error) {
	if _, e := Query(slot, count); e != nil {
		return nil, e
	}
	if e := ValidateWritable(p); e != nil {
		return nil, e
	}
	reports := [][]byte{{1, 0x81, byte(len(p)), byte(slot), 0, 0, 0, 0}}
	for offset := 0; offset < len(p); offset += 8 {
		b := make([]byte, 8)
		copy(b, p[offset:])
		reports = append(reports, b)
	}
	return reports, nil
}

// Equivalent allows firmware to omit trailing zeroes in fixed keyboard records.
func Equivalent(a, b []byte) bool {
	if Validate(a) != nil || Validate(b) != nil {
		return false
	}
	if a[1] != b[1] {
		return false
	}
	if a[1] != 1 && a[1] != 0x81 {
		return string(a) == string(b)
	}
	return strings.TrimRight(string(a[1:]), "\x00") == strings.TrimRight(string(b[1:]), "\x00")
}
