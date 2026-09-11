import test from 'node:test';
import assert from 'node:assert/strict';
import { saveBackup } from './backup.mjs';

test('backup is closed and read back before resolving', async () => {
  const events=[]; let content='';
  const picker=async()=>({createWritable:async()=>({write:async(s)=>{events.push('write');content=s;},close:async()=>events.push('close')}),getFile:async()=>({text:async()=>{events.push('read');return content;}})});
  await saveBackup({version:1,model:'400-MA214BK',records:['04010004']},picker);
  assert.deepEqual(events,['write','close','read']);
});
test('cancelled save or incorrect persisted file never counts as a backup', async()=>{
 await assert.rejects(saveBackup({},async()=>{throw new DOMException('cancelled','AbortError');}));
 await assert.rejects(saveBackup({},async()=>({createWritable:async()=>({write:async()=>{},close:async()=>{}}),getFile:async()=>({text:async()=>'{"wrong":true}'})})),/一致/);
});
