export const models = Object.freeze([
  Object.freeze({name: '400-MA214BK', pid: 0xb001, slots: 3, reportId: 0}),
  Object.freeze({name: '400-SKB081', pid: 0xc115, slots: 6, reportId: 1}),
]);
const modifiers = {ctrl:1,lctrl:1,shift:2,lshift:2,alt:4,lalt:4,cmd:8,super:8,win:8,lmeta:8,rctrl:16,rshift:32,ralt:64,rcmd:128,rmeta:128};
const keys = {enter:40,escape:41,esc:41,backspace:42,tab:43,space:44,minus:45,equal:46,leftbracket:47,rightbracket:48,backslash:49,semicolon:51,quote:52,grave:53,comma:54,period:55,slash:56,capslock:57,printscreen:70,scrolllock:71,pause:72,insert:73,home:74,pageup:75,delete:76,end:77,pagedown:78,right:79,left:80,down:81,up:82,numlock:83};
const owns = (object, key) => Object.hasOwn(object, key);
export function encodeKey(text) {
  if (typeof text !== 'string') throw Error('Enter a keyboard shortcut.');
  const record = Uint8Array.of(8,1,0,0,0,0,0,0);
  for (const part of text.toLowerCase().split('+')) {
    if (owns(modifiers,part)) {
      if (record[2] & modifiers[part]) throw Error('Duplicate modifier.');
      record[2] |= modifiers[part];
      continue;
    }
    let key = 0;
    if (/^[a-z]$/.test(part)) key = part.charCodeAt(0)-93;
    else if (/^[1-9]$/.test(part)) key = Number(part)+29;
    else if (part==='0') key=39;
    else if (/^f([1-9]|1[0-9]|2[0-4])$/.test(part)) {const n=Number(part.slice(1));key=n+(n<=12?57:91);}
    else if (owns(keys,part)) key=keys[part];
    if (!key || record[3]) throw Error('Use modifiers and one supported key, such as ctrl+shift+a.');
    record[3]=key;
  }
  return record;
}
export function validateRecord(record) {
  if (!(record instanceof Uint8Array) || record.length<2 || record.length>255 || record[0]!==record.length) throw Error('Invalid configuration record length.');
  return record;
}
export function validateWritable(record) {
  validateRecord(record);
  const size=record.length, type=record[1];
  if (type===0 && size===8 && record.slice(2).every(n=>n===0)) return record;
  if ((type===1 || type===0x81) && (size===4 || size===8) && record.slice(4).every(n=>n===0)) return record;
  if ((type===2 || type===3) && size===8 && record[4]<=7) return record;
  if (type===4 && size>=3 && size<=40) return record;
  throw Error('Unsupported configuration action or shape; this record cannot be safely restored.');
}
const hex = record => Array.from(record,n=>n.toString(16).padStart(2,'0')).join('');
export function equivalent(a,b) {
  try {validateRecord(a);validateRecord(b);} catch {return false;}
  if (a[1]!==b[1]) return false;
  if (a[1]!==1 && a[1]!==0x81) return hex(a)===hex(b);
  let endA=a.length, endB=b.length;
  while (endA>1 && a[endA-1]===0) endA--;
  while (endB>1 && b[endB-1]===0) endB--;
  return hex(a.slice(1,endA))===hex(b.slice(1,endB));
}
function knownModel(model) {
  const name=typeof model==='string'?model:model?.name;
  const found=models.find(item=>item.name===name);
  if (!found) throw Error('Unsupported model.');
  return found;
}
export function snapshot(model,records) {
  model=knownModel(model);
  if (!Array.isArray(records) || records.length!==model.slots) throw Error('Backup has the wrong number of slots.');
  return {version:1,model:model.name,records:records.map(record=>hex(validateRecord(record)))};
}
export function parseSnapshot(text,expectedModel) {
  // CLI ceiling: six maximum 255-byte records, formatted envelope, final newline.
  const maxSize=JSON.stringify({version:1,model:'400-MA214BK',records:Array(6).fill('ff'.repeat(255))},null,2).length+1;
  if (typeof text!=='string' || text.length>maxSize || new TextEncoder().encode(text).length>maxSize) throw Error('Backup is too large.');
  let value;
  try {value=JSON.parse(text);} catch {throw Error('Backup must contain valid JSON.');}
  const model=knownModel(expectedModel);
  if (!value || typeof value!=='object' || Array.isArray(value) || Object.keys(value).some(key=>!['version','model','records'].includes(key)) || value.version!==1 || value.model!==model.name || !Array.isArray(value.records) || value.records.length!==model.slots) throw Error('Backup version, model, or slot count does not match.');
  return value.records.map(record=>{
    if (typeof record!=='string' || !/^(?:[0-9a-fA-F]{2}){2,255}$/.test(record)) throw Error('Invalid hexadecimal configuration record.');
    return validateWritable(Uint8Array.from(record.match(/../g),pair=>parseInt(pair,16)));
  });
}
export function describeRecord(record) {
  validateRecord(record);
  try {validateWritable(record);} catch {return `hex:${hex(record)}`;}
  if (record[1]===0) return 'disabled';
  if (record[1]!==1 && record[1]!==0x81) return `hex:${hex(record)}`;
  let key;
  const usage=record[3];
  if (usage>=4 && usage<=29) key=String.fromCharCode(usage+93);
  else if (usage>=30 && usage<=38) key=String(usage-29);
  else if (usage===39) key='0';
  else if (usage>=58 && usage<=69) key=`f${usage-57}`;
  else if (usage>=104 && usage<=115) key=`f${usage-91}`;
  else key=Object.keys(keys).find(name=>keys[name]===usage);
  if (!key && usage!==0) return `hex:${hex(record)}`;
  const parts=['ctrl','shift','alt','cmd','rctrl','rshift','ralt','rcmd'].filter((_,i)=>record[2]&(1<<i));
  if (key) parts.push(key);
  return (record[1]===0x81?'release: ':'')+(parts.join('+') || 'no key');
}
