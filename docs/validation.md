# Validation

Verified on 2026-09-11. This file distinguishes protocol tests, real USB configuration access, and physical key behavior.

## Automated checks

- macOS 26.5.1 / ARM64 / Go 1.27.1: `go test -race ./...` and `go vet ./...` passed, including independent review execution.
- Debian 13 / Raspberry Pi ARM64 / kernel 6.18.39+rpt-rpi-2712 / Go 1.27.1: `go test ./...`, `go vet ./...`, and native build passed.
- On that Raspberry Pi, `go test -race ./...` aborted before tests with `ThreadSanitizer: unsupported VMA range; Found 47 - Supported 48`. This is not recorded as a passing race check.
- Initial protocol/CLI/HID framing tests failed before implementation and passed afterwards. Unsupported replay-record tests reproduced acceptance before the validation fix. Restore and partial-write regression checks passed against the final implementation.
- Independent code review: no remaining CRITICAL/HIGH findings after input validation, bounded snapshot loading, and restore/failure tests were added.

## Real hardware

### 400-MA214BK on macOS

The CLI selected interface 1 of `3553:b001` and checked its descriptor. Original records:

```text
0801000400000000
0801000500000000
0801000600000000
```

`set --slot 1 --key f13 --backup ...` returned `verified: true`, `changed_slots: 1`. Slot 1 became `0801006800000000`; slots 2 and 3 were unchanged. `restore --from ... --backup ...` returned verified success, restoring all original bytes. Both commands read every slot after programming.

### 400-SKB081 on Linux

The CLI selected interface 1 of `3553:c115` and checked its descriptor. Original records:

```text
04010004
04010005
04010006
04010007
04010008
04010009
```

`set --slot 6 --key ctrl+f13 --backup ...` returned `verified: true`, `changed_slots: 1`. Slot 6 became `0801016800000000`; slots 1–5 were unchanged. `restore --from ... --backup ...` returned verified success, restoring all original bytes. Access used `sudo` because the host's hidraw interfaces were root-only.

## Limits

- Each model was tested on the host listed above; the opposite model/OS combinations were not physically tested.
- Physical presses, application shortcut handling, slot-to-position mapping, and persistence after unplugging/power loss were not tested.
- Real-device changes were temporary, and both original configurations were restored.
- Mouse and string replay formats are source-grounded; only keyboard records were programmed in these hardware tests.

The `.go` files and module metadata used for these checks are identified in [source-sha256.txt](source-sha256.txt). CI separately checks every published commit on the configured native runners.
