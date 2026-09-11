import {connect} from './hid.mjs';
import {encodeKey, describeRecord, snapshot, parseSnapshot, validateWritable, equivalent} from './protocol.mjs';
import {saveBackup} from './backup.mjs';

const $ = id => document.getElementById(id);
let client = null, current = [], imported = null, edits = new Map(), busy = false;
const canSave = typeof globalThis.showSaveFilePicker === 'function';
const supported = Boolean(navigator.hid) && globalThis.isSecureContext;

function status(text, kind = '') { $('status').textContent = text; $('status').dataset.kind = kind; }
function target() {
  const records = (imported ?? current).map(record => record.slice());
  for (const [index, text] of edits) if (text.trim()) records[index] = encodeKey(text.trim());
  return records;
}
function changed(records) { return records.some((record, i) => !equivalent(record, current[i])); }
function controls() {
  $('connect').disabled = busy || !supported;
  $('connect').hidden = Boolean(client);
  $('disconnect').hidden = !client;
  for (const id of ['disconnect','refresh','export','import','reset']) $(id).disabled = busy || !client;
  for (const input of document.querySelectorAll('#key-rows input')) input.disabled = busy;
  $('workspace').setAttribute('aria-busy', String(busy));
  let valid = false;
  try { valid = Boolean(client) && changed(target()); } catch { /* Field errors appear below the inputs. */ }
  $('apply').disabled = busy || !valid || !canSave;
}
function preview() {
  const list = $('change-list'); list.replaceChildren();
  try {
    const records = target();
    records.forEach((record, i) => {
      if (equivalent(record, current[i])) return;
      const item = document.createElement('li');
      item.textContent = `入力 ${i + 1}: ${describeRecord(current[i])} → ${describeRecord(record)}`;
      list.append(item);
    });
    $('no-changes').textContent = '変更はありません。';
    $('no-changes').hidden = list.childElementCount > 0;
  } catch {
    $('no-changes').hidden = false;
    $('no-changes').textContent = 'キー名を修正してください。';
  }
  controls();
}
function render() {
  $('workspace').hidden = !client;
  $('compatibility').hidden = Boolean(client);
  $('device-name').textContent = client ? `${client.model.name} · ${client.model.slots}入力` : '未接続';
  const rows = $('key-rows'); rows.replaceChildren();
  current.forEach((record, i) => {
    const row = document.createElement('div'); row.className = 'key-row';
    const slot = document.createElement('span'); slot.className = 'slot-label'; slot.textContent = `入力 ${i+1}`;
    const value = document.createElement('span'); value.className = 'current'; value.textContent = describeRecord(record);
    const field = document.createElement('div'); field.className = 'field';
    const label = document.createElement('label'); label.htmlFor = `key-${i}`; label.textContent = `入力 ${i+1} の新しい設定`;
    const input = document.createElement('input'); input.id = label.htmlFor; input.type = 'text'; input.autocomplete = 'off'; input.spellcheck = false;
    input.placeholder = imported ? describeRecord(imported[i]) : '変更しない';
    input.value = edits.get(i) ?? ''; input.setAttribute('aria-describedby', `error-${i} key-help`);
    const error = document.createElement('p'); error.id = `error-${i}`; error.className = 'field-error';
    input.addEventListener('input', () => {
      edits.set(i, input.value);
      try { if (input.value.trim()) encodeKey(input.value.trim()); input.removeAttribute('aria-invalid'); error.textContent = ''; }
      catch { input.setAttribute('aria-invalid', 'true'); error.textContent = 'キー名を確認してください。例: ctrl+shift+a'; }
      preview();
    });
    field.append(label,input,error); row.append(slot,value,field); rows.append(row);
  });
  $('apply-help').textContent = canSave ? '適用前の全設定をファイルに保存してから書き込みます。' : 'このブラウザでは保存APIが使えません。読み出しのみ利用できます。';
  preview();
}
async function forgetConnection() {
  const old = client; client = null; current = []; imported = null; edits.clear();
  try { await old?.close(); } catch { /* The connection is already unusable; show reconnect controls. */ }
  render();
}
async function withBusy(operation, closeOnError = true) {
  if (busy) return;
  busy = true; controls();
  try { await operation(); }
  catch (error) {
    if (error?.name === 'BackupCancelledError') status('保存を取り消しました。本体には書き込んでいません。');
    else { if (closeOnError) await forgetConnection(); status(`エラー: ${error.message ?? '操作に失敗しました。接続し直してください。'}`, 'error'); }
  } finally { busy = false; controls(); }
}
$('connect').addEventListener('click', () => withBusy(async () => {
  status('デバイスを選択し、現在の設定を読み出しています。');
  client = await connect(); current = await client.readAll(); imported = null; edits.clear(); render();
  status('現在の設定を読み出しました。変更する入力を選んでください。', 'success');
}));
$('disconnect').addEventListener('click', () => withBusy(async () => { await forgetConnection(); status('切断しました。'); }));
$('refresh').addEventListener('click', () => withBusy(async () => {
  status('現在の設定を読み出しています。'); current = await client.readAll(); imported = null; edits.clear(); render(); status('再読み込みしました。未適用の変更は取り消しました。','success');
}));
$('editor').addEventListener('submit', event => {
  event.preventDefault();
  if (!client || busy || !canSave || $('apply').disabled) return;
  let desired;
  try { desired = target(); current.forEach(validateWritable); desired.forEach(validateWritable); }
  catch(error) { status(`エラー: ${error.message}`, 'error'); return; }
  if (!changed(desired)) return;
  const selected = client, before = current.map(record => record.slice());
  withBusy(async () => {
    status('バックアップの保存先を選択してください。');
    await saveBackup(snapshot(selected.model, before));
    if (client !== selected) throw Error('接続が変わりました。再接続して設定を確認してください。');
    status('本体に書き込み、全入力の設定を確認しています。');
    try { current = await selected.apply(desired, before); }
    catch(error) { throw Error(`設定を完了できませんでした。一部変更されている可能性があります。バックアップを残し、再接続して確認してください。 ${error.message}`); }
    imported = null; edits.clear(); render();
    status('設定を適用し、全入力の読み戻し一致を確認しました。','success');
  });
});
$('export').addEventListener('click', () => withBusy(async () => {
  const value = snapshot(client.model, current);
  if (canSave) await saveBackup(value);
  else {
    const url = URL.createObjectURL(new Blob([JSON.stringify(value,null,2)+'\n'], {type:'application/json'}));
    const a=document.createElement('a'); a.href=url; a.download=`sanwa-keys-${client.model.name}.json`; document.body.append(a); a.click(); a.remove();
    // Revocation after a later user event gives the browser time to start its download.
    window.addEventListener('focus', () => URL.revokeObjectURL(url), {once:true});
  }
  status(canSave ? '読み出した設定を保存し、ファイルの内容を確認しました。' : 'バックアップのダウンロードを開始しました。保存結果を確認してください。','success');
}, false));
$('import').addEventListener('change', async () => {
  if (!client || busy) return;
  const file = $('import').files[0]; if (!file) return;
  await withBusy(async () => {
    // Bound before file.text() allocation; same maximum schema as the CLI.
    const maxSize=JSON.stringify({version:1,model:'400-MA214BK',records:Array(6).fill('ff'.repeat(255))},null,2).length+1;
    if (file.size > maxSize) throw Error('復元ファイルが大きすぎます。sanwa-keysのJSONを選択してください。');
    imported = parseSnapshot(await file.text(), client.model); edits.clear(); render();
    status('復元内容を読み込みました。変更内容を確認して適用してください。');
  }, false);
  $('import').value='';
});
$('reset').addEventListener('click', () => { imported=null; edits.clear(); render(); status('未適用の変更を取り消しました。'); });
navigator.hid?.addEventListener('disconnect', event => {
  if (client?.matchesDevice(event.device)) {
    forgetConnection().then(() => status('USB接続が切れました。接続し直して設定を確認してください。','error'));
  }
});
if (!supported) {
  $('compatibility').textContent = 'WebHIDを利用できません。デスクトップ版Chrome / EdgeでHTTPSのページを開いてください。';
  status('このブラウザでは接続できません。','error');
}
render();

const keyHelp = $('key-help-dialog');
$('show-key-help').addEventListener('click', () => keyHelp.showModal());
$('close-key-help').addEventListener('click', () => keyHelp.close());
keyHelp.addEventListener('click', event => {
  if (event.target !== keyHelp) return;
  const bounds = keyHelp.getBoundingClientRect();
  if (event.clientX < bounds.left || event.clientX > bounds.right || event.clientY < bounds.top || event.clientY > bounds.bottom) keyHelp.close();
});
