import {models, validateRecord, validateWritable, equivalent} from './protocol.mjs';

// CLI default read deadline and the PCsensor footswitch programming interval.
const timeoutMS = 1000;
const writeIntervalMS = 30;
const ack = bytes => bytes[0]===0x81 && bytes[1]===0x55;
function validateCollections(device, model) {
  const inputs=[], outputs=[];
  if (!Array.isArray(device.collections) || device.collections.length===0) throw Error('Missing HID collections.');
  for (const collection of device.collections) {
    if (collection.usagePage!==1 || collection.usage!==0 || !Array.isArray(collection.children) || collection.children.length || !Array.isArray(collection.featureReports) || collection.featureReports.length || !Array.isArray(collection.inputReports) || !Array.isArray(collection.outputReports)) throw Error('Unsupported HID collection.');
    if (collection.inputReports.length+collection.outputReports.length===0) throw Error('Empty HID collection.');
    inputs.push(...collection.inputReports);
    outputs.push(...collection.outputReports);
  }
  const validReport=(report,id,bytes)=>report?.reportId===id && Array.isArray(report.items) && report.items.length===1 && report.items[0].reportSize===8 && report.items[0].reportCount===bytes;
  if (outputs.length!==1 || !validReport(outputs[0],model.reportId,8)) throw Error('Unsupported HID output report.');
  if (inputs.filter(report=>validReport(report,model.reportId,8)).length!==1 || inputs.some(report=>!validReport(report,model.reportId,8) && !(model.reportId===1 && validReport(report,2,15))) || inputs.length>(model.reportId===1?2:1)) throw Error('Unsupported HID input report.');
  // WebHID exposes parsed collections, not raw descriptors or interface numbers.
}
export async function connect(hid=globalThis.navigator?.hid) {
  if (!hid || typeof hid.requestDevice!=='function') throw Error('WebHID is unavailable. Use a supported browser over HTTPS or localhost.');
  const selected=await hid.requestDevice({filters:models.map(model=>({vendorId:0x3553,productId:model.pid,usagePage:1,usage:0}))});
  if (!Array.isArray(selected) || selected.length!==1) throw Error('Select exactly one supported device.');
  const device=selected[0];
  const model=models.find(item=>device.vendorId===0x3553 && device.productId===item.pid);
  if (!model) throw Error('Unsupported device identity.');
  validateCollections(device,model);
  if (device.opened) throw Error('The device is already open. Close its existing connection first.');
  const client=new Client(device,hid,model);
  await client.open();
  return client;
}
class Client {
  #device; #hid; #invalid=false; #busy=false; #pending=null; #failure; #rejectFailure; #closing;
  #onInput; #onDisconnect;
  constructor(device,hid,model) {
    this.#device=device;this.#hid=hid;this.model=model;
    this.#failure=new Promise((_,reject)=>{this.#rejectFailure=reject;});
    this.#failure.catch(()=>{});
    this.#onInput=event=>this.#input(event);
    this.#onDisconnect=event=>{if(event.device===device)this.#invalidate(Error('Device disconnected. Reconnect before continuing.'));};
    device.addEventListener('inputreport',this.#onInput);
    hid.addEventListener('disconnect',this.#onDisconnect);
  }
  #invalidate(error) {
    if(this.#invalid)return;
    this.#invalid=true;
    this.#rejectFailure(error);
    this.#device.removeEventListener('inputreport',this.#onInput);
    this.#hid.removeEventListener('disconnect',this.#onDisconnect);
    this.#closing=Promise.resolve().then(()=>this.#device.close());
    this.#closing.catch(()=>{});
  }
  async #bounded(operation,deadline=performance.now()+timeoutMS) {
    if(this.#invalid)throw Error('Connection is closed. Reconnect before continuing.');
    let timer;
    try {
      const result=await Promise.race([operation,this.#failure,new Promise((_,reject)=>{
        timer=setTimeout(()=>reject(Error('HID operation timed out. Reconnect before continuing.')),Math.max(0,deadline-performance.now()));
      })]);
      if(this.#invalid)throw Error('Connection is closed.');
      return result;
    } catch(error) {this.#invalidate(error);throw error;} finally {clearTimeout(timer);}
  }
  async open() {
    const opening=Promise.resolve().then(()=>this.#device.open());
    // A timed-out open cannot be cancelled by WebHID; close if it later succeeds.
    opening.then(()=>{if(this.#invalid)this.#device.close().catch(()=>{});},()=>{});
    await this.#bounded(opening);
  }
  matchesDevice(device) {return device===this.#device;}
  async close() {
    this.#invalidate(Error('Connection closed.'));
    let timer;
    try {await Promise.race([this.#closing,new Promise((_,reject)=>{timer=setTimeout(()=>reject(Error('Device close timed out.')),timeoutMS);})]);}
    finally {clearTimeout(timer);}
  }
  async #run(operation) {
    if(this.#invalid)throw Error('Connection is closed. Reconnect before continuing.');
    if(this.#busy)throw Error('Another device operation is in progress.');
    this.#busy=true;
    try {return await operation();} finally {this.#busy=false;}
  }
  #input(event) {
    if(this.#invalid)return;
    if(event.device!==this.#device || event.reportId!==this.model.reportId || !(event.data instanceof DataView) || event.data.byteLength!==8) {
      this.#invalidate(Error('Unexpected HID input report ID or length.'));return;
    }
    const bytes=new Uint8Array(event.data.buffer,event.data.byteOffset,event.data.byteLength).slice();
    const pending=this.#pending;
    if(!pending)return;
    if(pending.bytes.length===0 && ack(bytes))return;
    if(pending.bytes.length===0) {
      pending.length=bytes[0];
      if(pending.length<2) {this.#invalidate(Error('Invalid configuration record length.'));return;}
    }
    pending.bytes.push(...bytes.slice(0,Math.min(8,pending.length-pending.bytes.length)));
    if(pending.bytes.length===pending.length) {
      const record=Uint8Array.from(pending.bytes);
      try {validateRecord(record);} catch(error){this.#invalidate(error);return;}
      this.#pending=null;pending.resolve(record);
    }
  }
  async #send(payload,deadline) {
    if(this.#invalid)throw Error('Connection is closed.');
    if(!(payload instanceof Uint8Array) || payload.length!==8)throw Error('Invalid HID output payload.');
    await this.#bounded(Promise.resolve().then(()=>{if(this.#invalid)throw Error('Connection is closed.');return this.#device.sendReport(this.model.reportId,payload);}),deadline);
  }
  async #readAll() {
    const records=[];
    for(let slot=1;slot<=this.model.slots;slot++) {
      const deadline=performance.now()+timeoutMS;
      const response=new Promise(resolve=>{this.#pending={resolve,bytes:[],length:0};});
      try {
        const results=await this.#bounded(Promise.all([this.#send(Uint8Array.of(1,0x82,8,slot,0,0,0,0),deadline),response]),deadline);
        records.push(results[1]);
      } finally {this.#pending=null;}
    }
    return records;
  }
  readAll() {return this.#run(()=>this.#readAll());}
  apply(targetRecords,beforeRecords) {
    return this.#run(async()=>{
      const copy=records=>{
        if(!Array.isArray(records) || records.length!==this.model.slots)throw Error('Configuration has the wrong slot count.');
        return records.map(record=>validateWritable(record).slice());
      };
      // Copy caller-owned arrays before the first await so edits cannot race I/O.
      const target=copy(targetRecords), before=copy(beforeRecords);
      const current=await this.#readAll();
      if(!current.every((record,i)=>equivalent(record,before[i])))throw Error('Device settings changed since the backup. Read and save a fresh backup.');
      const changed=target.map((record,i)=>!equivalent(record,before[i]));
      if(!changed.some(Boolean))return current;
      for(let i=0;i<target.length;i++) {
        if(!changed[i])continue;
        const record=target[i];
        const reports=[Uint8Array.of(1,0x81,record.length,i+1,0,0,0,0)];
        for(let offset=0;offset<record.length;offset+=8) {const payload=new Uint8Array(8);payload.set(record.slice(offset,offset+8));reports.push(payload);}
        for(const report of reports) {
          await this.#send(report);
          await this.#bounded(new Promise(resolve=>setTimeout(resolve,writeIntervalMS)));
        }
      }
      const after=await this.#readAll();
      if(!after.every((record,i)=>equivalent(record,target[i]))) {
        const error=Error('Readback does not match the requested settings. Keep the backup and reconnect before recovery.');
        this.#invalidate(error);throw error;
      }
      return after;
    });
  }
}
