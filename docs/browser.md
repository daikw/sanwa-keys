# Browser configurator

Open <https://daikw.github.io/sanwa-keys/> in desktop Chrome or Edge. The page uses WebHID to talk to a supported device connected to **the computer running that browser**. It cannot reach USB devices attached to another computer over SSH or Tailscale.

1. Select **デバイスを接続** and choose the keyboard or pedal in the browser's permission prompt. The model and current assignments are read automatically.
2. Enter a new key or shortcut only for slots you want to change. Review the visible differences.
3. Select **バックアップを保存して適用** and save the JSON file. Cancelling the picker does not write to the device.
4. The page closes the saved file, reads its contents back, checks the device still matches that snapshot, then programs changed slots and verifies every slot.
5. To restore, choose the saved JSON under **復元ファイルを選択**, review the differences, then apply. The pre-restore state is also backed up.

The CLI and browser use the same snapshot format. The save picker is required for programming; browsers without it are read-only. This project has no configuration server, analytics, remote scripts, or fonts. A Content Security Policy prohibits network connections from the page. GitHub Pages receives ordinary requests for the static site assets; device records are not transmitted to it.

## Compatibility boundaries

- WebHID is a desktop Chromium feature. Safari and Firefox are unsupported. HTTPS (or localhost for development) is required. See [Chrome's WebHID documentation](https://developer.chrome.com/docs/capabilities/hid).
- On Linux the browser user must have access to the selected `/dev/hidraw*` node. Do not run the browser as root. Have the administrator grant narrowly scoped access for the supported VID/PID and configuration interface; the CLI's `sudo` example does not apply to browsers.
- WebHID exposes parsed HID collections, not raw report-descriptor bytes or interface numbers. The browser checks VID/PID, usage page/usage, report IDs, item counts, and payload sizes. This is a different identity gate from the CLI's byte-exact descriptor check.
- Keyboard input collections are protected by the browser. Only the separate configuration collection is requested. The application does not monitor typed keys.
- Both protocol families are implemented. Only the MA214BK/macOS browser combination has real hardware write-and-restore evidence. SKB081 browser support is tested with simulated WebHID; its real Linux configuration evidence currently belongs to the CLI.
- Physical presses, host shortcut behavior, and persistence after power loss remain untested.

## Validation — 2026-09-11

**Real hardware:** macOS 26.5.1, Chrome for Testing 149.0.7827.55, 400-MA214BK. Used native browser device selection and native File System Access save/open dialogs. The page read `a / b / c`, saved the original snapshot, programmed slot 1 to `f14`, verified all slots, loaded the saved snapshot, saved the test state, and restored `a / b / c` with successful all-slot readback. The connection was explicitly closed afterwards. The saved JSON files were retained locally, outside this public repository.

**Automated:** `node --test web/*.test.mjs` covers 18 checks, including both report formats, key encoding, restore schema bounds, saved-file readback, cancellation, stale-state rejection, peer preservation, malformed reports, endless ACK timeout, failed writes, and exact device identity on disconnect. Protocol/HID/backup tests were run RED before implementation and GREEN afterwards.

**Browser UI with simulated USB and file saving:** [tests/browser-check.js](../tests/browser-check.js) runs through connect, invalid input, save cancellation with zero programming commands, apply, backup import/restore, same-model other-device disconnect, and USB AbortError handling. It uses Playwright's page API and can be run through Playwright MCP `browser_run_code_unsafe` with the test's filename while serving `web/`. No fake hardware mode is included in the published page. Layout checks passed at 320, 375, 414, 768, and 1280 CSS pixels with no horizontal overflow and all visible buttons above 44 pixels high.

## Local preview and deployment

```sh
python3 -m http.server --bind 127.0.0.1 --directory web 8000
node --test web/*.test.mjs
```

Open `http://127.0.0.1:8000/`. No Node dependency installation or bundler is needed. `tests/browser-check.js` requires an existing Playwright environment; it does not download or install one.

The Pages workflow tests the WebHID modules and uploads an explicit allowlist of production assets. Test fixtures and configuration backups are excluded. Set repository **Settings → Pages → Source** to **GitHub Actions**. After integration, changes to `web/` on `main` deploy automatically; a maintainer can also dispatch the workflow manually. Publishing the reviewed feature branch for initial verification is separate from merging its PR.
