// Run with Playwright MCP browser_run_code_unsafe(filename) against the served web/ page.
async (page) => {
  await page.addInitScript(() => {
    const report = {reportId: 0, items: [{reportSize:8,reportCount:8}]};
    class FakeDevice extends EventTarget {
      vendorId=0x3553; productId=0xb001; opened=false;
      collections=[{usagePage:1,usage:0,children:[],featureReports:[],inputReports:[report],outputReports:[report]}];
      records=[4,5,6].map(key=>Uint8Array.of(8,1,0,key,0,0,0,0));
      writes=[]; selected=0; failWrite=false;
      async open(){this.opened=true;}
      async close(){this.opened=false;}
      async sendReport(id,payload){
        if(id!==0 || payload.length!==8)throw Error('wrong framing');
        if(payload[0]===1 && payload[1]===0x82){
          const bytes=this.records[payload[3]-1].slice();
          queueMicrotask(()=>this.dispatchEvent(Object.assign(new Event('inputreport'),{device:this,reportId:0,data:new DataView(bytes.buffer)})));
        } else {
          if(!window.fixture.saved)throw Error('programmed without saved backup');
          this.writes.push([...payload]);
          if(this.failWrite)throw new DOMException('USB operation aborted','AbortError');
          if(payload[0]===1 && payload[1]===0x81)this.selected=payload[3]-1;
          else this.records[this.selected]=payload.slice(0,payload[0]);
        }
      }
    }
    const hid = new EventTarget(), device = new FakeDevice();
    hid.requestDevice=async()=>[device];
    Object.defineProperty(navigator,'hid',{configurable:true,value:hid});
    window.fixture={device,hid,saved:'',cancel:false};
    window.showSaveFilePicker=async()=>{
      if(window.fixture.cancel)throw new DOMException('cancel','AbortError');
      let pending='';
      return {createWritable:async()=>({write:async text=>{pending=text;},close:async()=>{window.fixture.saved=pending;}}),getFile:async()=>({text:async()=>window.fixture.saved})};
    };
  });
  await page.reload();
  await page.getByRole('button',{name:'デバイスを接続',exact:true}).click();
  await page.getByRole('status').filter({hasText:'現在の設定を読み出しました'}).waitFor();
  const first=page.getByLabel('入力 1 の新しい設定',{exact:true});
  await first.fill('cmd+;');
  const help=page.getByRole('button',{name:'使えるキー名',exact:true});
  if(await help.count()!==1)throw Error('Missing in-page key help button');
  await help.click();
  const dialog=page.getByRole('dialog',{name:'使えるキー名',exact:true});
  if(!await dialog.isVisible() || !(await dialog.innerText()).includes('cmd+semicolon'))throw Error('Key help is missing symbol example');
  await page.getByRole('button',{name:'閉じる',exact:true}).click();
  if(await dialog.isVisible() || await first.inputValue()!=='cmd+;')throw Error('Help changed pending input');
  await help.click(); await page.keyboard.press('Escape');
  if(await dialog.isVisible() || !await help.evaluate(el=>document.activeElement===el))throw Error('Dialog Escape/focus restoration failed');
  await first.fill('a+b');
  if(await page.getByRole('button',{name:'バックアップを保存して適用',exact:true}).isEnabled())throw Error('invalid key accepted');
  await first.fill('f14');
  await page.evaluate(()=>{window.fixture.cancel=true;});
  await page.getByRole('button',{name:'バックアップを保存して適用',exact:true}).click();
  await page.getByRole('status').filter({hasText:'保存を取り消しました'}).waitFor();
  if(await page.evaluate(()=>window.fixture.device.writes.length))throw Error('cancelled save wrote device');
  await page.evaluate(()=>{window.fixture.cancel=false;});
  await page.getByRole('button',{name:'バックアップを保存して適用',exact:true}).click();
  await page.getByRole('status').filter({hasText:'設定を適用し、全入力'}).waitFor();
  const evidence=await page.evaluate(()=>({saved:JSON.parse(window.fixture.saved),records:window.fixture.device.records.map(x=>[...x]),writes:window.fixture.device.writes.length}));
  if(evidence.records[0][3]!==105 || evidence.records[1][3]!==5 || evidence.writes!==2 || evidence.saved.records[0]!=='0801000400000000')throw Error('incorrect apply or backup');
  // Restore through the actual file input event; fixture does not bypass app parsing.
  await page.locator('#import').evaluate(input=>{
    const files=new DataTransfer();files.items.add(new File([window.fixture.saved],'original.json',{type:'application/json'}));input.files=files.files;input.dispatchEvent(new Event('change',{bubbles:true}));
  });
  await page.getByRole('status').filter({hasText:'復元内容を読み込みました'}).waitFor();
  await page.getByRole('button',{name:'バックアップを保存して適用',exact:true}).click();
  await page.getByRole('status').filter({hasText:'設定を適用し、全入力'}).waitFor();
  if(await page.evaluate(()=>window.fixture.device.records[0][3])!==4)throw Error('restore failed');
  const dimensions=[];
  for(const width of [320,375,414,768,1280]){
    await page.setViewportSize({width,height:900});
    const metrics=await page.evaluate(()=>({width:innerWidth,scroll:document.documentElement.scrollWidth,buttons:[...document.querySelectorAll('button')].filter(x=>x.offsetParent).map(x=>({text:x.textContent,width:x.getBoundingClientRect().width,height:x.getBoundingClientRect().height}))}));
    if(metrics.scroll>width)throw Error(`overflow at ${width}`);
    dimensions.push(metrics);
  }
  await page.evaluate(()=>window.fixture.hid.dispatchEvent(Object.assign(new Event('disconnect'),{device:{vendorId:0x3553,productId:0xb001}})));
  if(!(await page.getByRole('button',{name:'切断',exact:true}).isVisible()))throw Error('same-model other device disconnected selected device');
  await page.getByLabel('入力 1 の新しい設定',{exact:true}).fill('f14');
  await page.evaluate(()=>{window.fixture.device.failWrite=true;});
  await page.getByRole('button',{name:'バックアップを保存して適用',exact:true}).click();
  await page.getByRole('status').filter({hasText:'エラー:'}).waitFor();
  const failure=await page.getByRole('status').innerText();
  if(failure.includes('書き込んでいません'))throw Error('USB AbortError misreported as cancelled save');
  return {applyAndRestore:'passed',cancelledSave:'no writes',deviceAbort:failure,dimensions};
}
