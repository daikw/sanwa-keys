// Writing is enabled only after the browser has closed and reread this file.
export async function saveBackup(value, picker = options => globalThis.showSaveFilePicker(options)) {
  try {
  const text = JSON.stringify(value, null, 2) + '\n';
  const handle = await picker({
    suggestedName: `sanwa-keys-${value.model ?? 'backup'}-${new Date().toISOString().replace(/[:.]/g, '-')}.json`,
    types: [{description: 'sanwa-keys backup', accept: {'application/json': ['.json']}}],
  });
  const writer = await handle.createWritable();
  try { await writer.write(text); await writer.close(); }
  catch (error) { await writer.abort?.().catch(() => {}); throw error; }
  if (await (await handle.getFile()).text() !== text) throw Error('保存したバックアップが一致しません。書き込みを中止しました。');
  } catch (error) {
    if(error?.name === 'AbortError') throw Object.assign(new Error('バックアップの保存を取り消しました。'), {name: 'BackupCancelledError'});
    throw error;
  }
}
