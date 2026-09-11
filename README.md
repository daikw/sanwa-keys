# sanwa-keys

Configure the onboard key assignments of Sanwa programmable keys and foot pedals from macOS or Linux. No Windows application or background remapping service is required.

| Model | USB ID | Inputs | Hardware tested |
| --- | --- | --- | --- |
| [400-MA214BK](https://direct.sanwa.co.jp/ItemPage/400-MA214BK) | `3553:b001` | 3 pedals | macOS, Apple Silicon |
| [400-SKB081](https://direct.sanwa.co.jp/ItemPage/400-SKB081) | `3553:c115` | 6 keys | Linux, ARM64 |

An independent project, not affiliated with Sanwa or PCsensor. Devices with different USB descriptors are rejected even if the product ID matches.

## Browser configurator

Open [sanwa-keys on GitHub Pages](https://daikw.github.io/sanwa-keys/) in desktop Chrome or Edge. Select the USB device, edit the desired slots, and save the backup before applying. The page rereads the saved file, verifies the device has not changed, writes only changed slots, and reads every slot back. CLI JSON snapshots can be imported for restoration.

The static page uses WebHID directly and sends no configuration data to a server. Writing also requires the File System Access save picker. Safari/Firefox do not support WebHID. On Linux the browser user needs permission to access the device's hidraw interface; do not run the browser as root. See [browser support and validation](docs/browser.md).

## Build and install

Requires Go 1.27.1 and a C compiler. macOS uses the Xcode Command Line Tools; Debian/Ubuntu also needs `libudev-dev`. HIDAPI is included in the pinned Go dependency, so no separate HIDAPI installation is needed.

```sh
git clone https://github.com/daikw/sanwa-keys.git
cd sanwa-keys
make check build
make install  # ~/.local/bin/sanwa-keys
```

The Go version is also pinned in `mise.toml`. Add `~/.local/bin` to your PATH if necessary. GitHub Actions builds native macOS ARM64 and Linux AMD64/ARM64 artifacts; downloadable artifacts include dependency licenses. Use native builds because the USB dependency requires cgo.

## Usage

```sh
sanwa-keys list
sanwa-keys read # model inferred when exactly one supported device is connected
sanwa-keys read --model 400-SKB081 --out original.json

# Preview encoded payloads without opening USB.
sanwa-keys set --model 400-MA214BK --slot 1 --key cmd+f13 --dry-run

# Back up all slots, change one, then verify every slot.
sanwa-keys set --model 400-MA214BK --slot 1 --key cmd+f13 --backup before-pedal.json
sanwa-keys set --model 400-SKB081 --slot 6 --key ctrl+shift+a --backup before-keyboard.json

# Preserve current state before restoring the previous assignments.
sanwa-keys restore --model 400-MA214BK --from before-pedal.json --backup before-restore.json
```

Use a new backup filename for every mutation: existing files are never overwritten. Snapshot files have owner-only permissions and are flushed to disk before any programming command. `set` and `restore` return success only after all slots match the expected configuration, including unchanged slots. Reapplying an equivalent configuration does not write device memory.

`--model` is optional when exactly one supported device matches. `--device PATH` can disambiguate two devices without a model name. Zero or multiple matches are rejected. An explicit model keeps `set --dry-run` usable without USB access.

`--device PATH` selects the configuration interface shown by `list` when multiple devices match. All device commands accept `--timeout 1s`; this bounds each configuration response, not an entire multi-slot command.

On Linux, access to `/dev/hidraw*` may require root. For a one-off operation, run the same command with `sudo` and an explicit executable path, for example `sudo ~/.local/bin/sanwa-keys read --model 400-SKB081`. No kernel drivers are detached. On macOS, the configuration interface is opened non-exclusively.

### Key names

Letters `a`–`z`, digits `0`–`9`, `f1`–`f24`, and named keys:

```text
enter escape esc backspace tab space minus equal leftbracket rightbracket
backslash semicolon quote grave comma period slash capslock printscreen
scrolllock pause insert home pageup delete end pagedown right left down up numlock
```

Combine one key with `ctrl`, `shift`, `alt`, `cmd`/`super`/`win`, `rctrl`, `rshift`, `ralt`, or `rcmd`. Modifier-only assignments are accepted. Keys are USB keyboard usages: character output depends on the host keyboard layout. `cmd`, `super`, and `win` name the same left GUI modifier.

Slot numbers are the device's firmware indices, starting at 1. Physical position labels have not been validated by pressing every control.

### Scope and recovery

- Creates single-key assignments with optional modifiers. Multi-step macros, lighting, layers, firmware updates, and application-specific actions are not implemented.
- Reads raw configuration records and restores known keyboard, mouse, combined keyboard/mouse, unconfigured, and short-string formats. Unknown action formats are rejected before programming; they can still be read for diagnosis.
- Snapshots are model-specific JSON. The loader bounds their size by the maximum encoded six-slot device record set and rejects unknown fields, extra JSON, unsupported versions, and malformed records.
- Do not run another configuration program concurrently. A disconnected device or failed write can leave a partial change. The command returns an error and retains the backup; reconnect and explicitly run `restore`. There is no automatic retry of programming commands.
- Hardware validation covers USB configuration writes and readback, not physical key events or persistence after removing power. See [validation](docs/validation.md).

## Development

```sh
make check  # go vet and race-enabled tests
make build
node --test web/*.test.mjs # browser protocol and backup logic (Node 26.8.1)
```

Protocol, malformed-input, backup ordering, partial-write, verification, and HID framing tests use simulated USB responses. No test automatically accesses hardware. On Linux systems whose virtual address layout is unsupported by Go's race runtime, `go test -race` may abort; `go test ./...` and `go vet ./...` can still run, but do not replace a race-enabled check on a supported host.

[Protocol notes and sources](docs/protocol.md) describe the two wire formats and compatibility limits. MIT licensed; bundled dependency notices are in [LICENSES](LICENSES/).
