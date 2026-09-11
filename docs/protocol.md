# Configuration protocol

Payloads below are eight bytes before transport framing. Only interface 1, usage page 1, usage 0 is opened. The CLI validates the complete report descriptor against the tested device before sending commands.

| Operation | Payload |
| --- | --- |
| Read slot | `01 82 08 SLOT 00 00 00 00` |
| Write slot header | `01 81 LENGTH SLOT 00 00 00 00` |
| Normal key data | `08 01 MODIFIER HID_USAGE 00 00 00 00` |

A configuration record begins with its byte length and action type. Record chunks are eight bytes, with zero padding on the final chunk. ACK payloads beginning `81 55` are skipped only before the first configuration chunk. Readback validates report lengths and report IDs, then collects the declared record length within one response deadline.

Writes target only the selected slot. The legacy `01 80 ...` flash/start command is not sent: these two devices do not require it. Programming reports are spaced by the 30 ms interval used by the PCsensor footswitch reference implementation. No firmware commands are exposed.

## 400-MA214BK

`3553:b001`, three slots. Descriptor:

```text
05010900a1010901150025ff95087508810209019102c0
```

Despite the unnumbered descriptor, the established footswitch implementation passes the eight-byte command/data payload directly to HIDAPI `Write`. This unusual framing, including a programming data packet starting `08`, was verified on macOS with this model. Do not infer it from the general HIDAPI convention of a leading zero for unnumbered reports. Responses contain eight payload bytes with no report ID.

## 400-SKB081

`3553:c115`, six slots. Descriptor:

```text
05010900a10185010901150025ff95087508810209019102c0
05010900a10185020901150025ff950f75088102c0
```

HIDAPI writes contain Report ID `01` followed by the eight-byte payload. Configuration responses also have Report ID `01` followed by eight bytes. The second declared input report is not used for configuration. The firmware may return a four-byte key record (`04 01 MODIFIER HID_USAGE`) even when given an eight-byte record. Verification permits omitted trailing zeroes for keyboard records only.

## Sources

Protocol facts were cross-checked against the following sources; their applications and extracted code are not distributed here:

- [rgerganov/footswitch, commit 454e00b](https://github.com/rgerganov/footswitch/blob/454e00b517cf6eaaa4dc5c6c15403732bc2365aa/src/footswitch.c): legacy `3553:b001` support, query/write payloads, record types, 38-key string limit, and programming pacing (MIT).
- [ScottAlanStevens/elf-key, commit 6bedd98](https://github.com/ScottAlanStevens/elf-key/tree/6bedd9804ce228a311387e84c1c4c80ba60fe506): related MK321U numbered-report protocol.
- [Kynde/elfctl protocol notes, commit 48bb13c](https://github.com/Kynde/elfctl/blob/48bb13c451c0a170704b087f5dd2d51fa93fd5cc/docs/PROTOCOL.md): related MK424BT protocol observations. This is a different product; compatibility was not assumed.
- [PCsensor downloads](https://pcsensor.com/download/), ElfKey 3.0.0 macOS ARM64: static inspection of generic keyboard serialization, ACK handling, and the older-device-only flash condition. The application was not executed. Download SHA-256: `eded07c296f76ef91377712bea19e662e5296d0f8c75060c9da0731133282561`.
- Live USB descriptors and configuration read/write/restore on the exact two Sanwa models, recorded in [validation.md](validation.md).

USB access uses [sstallion/go-hid v0.15.0](https://github.com/sstallion/go-hid/tree/v0.15.0), which includes HIDAPI. Dependencies are version-pinned and checked by Go's module checksum database. Platform APIs supply device discovery and access control.
