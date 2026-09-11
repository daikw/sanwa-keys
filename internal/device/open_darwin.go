package device

import "github.com/sstallion/go-hid"

func prepareOpen() { hid.SetOpenExclusive(false) }
