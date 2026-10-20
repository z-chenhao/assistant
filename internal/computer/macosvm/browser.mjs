// Executed only inside the Guest by the Host adapter. No model-supplied source,
// endpoint, module, evaluator or filesystem operation enters this process.
import fs from 'node:fs';
import {pathToFileURL} from 'node:url';
import { spawnSync } from 'node:child_process';
const request = JSON.parse(process.argv[1]);
const { method, args: a, root } = request;
let browser;
let mutating = false;
const fail = code => { throw Object.assign(new Error(code), { code }); };
const selector = s => {
  if (typeof s !== 'string' || !s || s.length > 2048 || s.includes('>>')) fail('INVALID_ARGUMENT');
  return `css=${s}`;
};
const url = value => {
  if (value === 'about:blank') return value;
  let u; try { u = new URL(value); } catch { fail('INVALID_ARGUMENT'); }
  if (!['http:', 'https:'].includes(u.protocol) || u.username || u.password) fail('INVALID_ARGUMENT');
  return value;
};
const timeout = value => {
  if (value === undefined) return 3000;
  if (!Number.isInteger(value) || value < 1 || value > 10000) fail('INVALID_ARGUMENT');
  return value;
};
async function identity(page) {
  const session = await page.context().newCDPSession(page);
  try { return (await session.send('Target.getTargetInfo')).targetInfo.targetId; }
  finally { await session.detach(); }
}
async function navigation(page) {
  const session = await page.context().newCDPSession(page);
  try { return JSON.stringify([(await session.send('Page.getFrameTree')).frameTree.frame.loaderId,page.url()]); }
  finally { await session.detach(); }
}
// Transfer helpers are installed from this repository alongside the pinned
// Guest Playwright module. Python owns the same openat/O_NOFOLLOW path checks.
function transfer(operation, path, data) {
  const helper = fs.readFileSync(new URL('./erii-transfer.py', pathToFileURL(request.module)), 'utf8');
  const p = spawnSync('/usr/bin/python3', ['-c', helper], { input:JSON.stringify({ root, path, operation, overwrite: false, data: data?.toString('base64') ?? '' }), encoding: 'utf8', maxBuffer: 12 << 20, timeout: 10000 });
  let result; try { result = JSON.parse(p.stdout); } catch { fail('PERMISSION_DENIED'); }
  if (p.status !== 0 || result.error) fail('PERMISSION_DENIED');
  return Buffer.from(result.data, 'base64');
}
async function main() {
  if (!Number.isInteger(request.port) || request.port < 1024 || request.port > 65535) fail('INVALID_ARGUMENT');
  const manifest = JSON.parse(fs.readFileSync(new URL('./package.json',pathToFileURL(request.module)), 'utf8'));
  if(manifest.version !== '1.64.1' || manifest.name !== 'playwright-core') fail('PROVIDER_UNAVAILABLE');
  const { chromium } = await import(request.module);
  browser = await chromium.connectOverCDP(`http://127.0.0.1:${request.port}`, { timeout: 5000 });
  const context = browser.contexts()[0];
  if (!context) fail('PROVIDER_UNAVAILABLE');
  const pages = context.pages();
  if (method === 'inventory') {
    const tabs = [];
    for (const page of pages) tabs.push({ id: await identity(page), kind: 'tab', name: await page.title(), url:page.url(), browser: 'chromium' });
    return { browsers: [{ id: 'chromium', kind: 'browser', name: 'Chromium' }], tabs };
  }
  if (method === 'createBrowserTab') {
    if (a.browserId !== 'chromium') fail('INVALID_ARGUMENT');
    const destination = url(a.url || 'about:blank');
    mutating = true;
    const page = await context.newPage();
    await page.goto(destination, { timeout: 10000 });
    return { id: await identity(page), kind: 'tab', name: await page.title(), url:page.url(), browser: 'chromium' };
  }
  let page;
  for (const candidate of pages) if (await identity(candidate) === a.tabId) page = candidate;
  if (!page) fail('STALE_SURFACE_HANDLE');
  page.setDefaultTimeout(timeout(a.timeoutMs));
  const nav = await navigation(page);
  if (a.expectedNavigation && a.expectedNavigation !== nav) fail('STALE_SURFACE_HANDLE');
  if (method === 'observe') {
    if(a.ax===false){const viewport=await page.evaluate(()=>({x:scrollX,y:scrollY,width:innerWidth,height:innerHeight,scale:devicePixelRatio}));const png=await page.screenshot({type:'png',scale:'css',timeout:10000});return {navigation:nav,window_bounds:viewport,geometry:JSON.stringify(viewport),screenshot_width:viewport.width,screenshot_height:viewport.height,screenshot_png_b64:png.toString('base64')};}
    // Projection is fixed Host code, bounded, and excludes form secrets.
    const tree = await page.evaluate(() => {
      const elements = [...document.querySelectorAll('button,input:not([type=password]):not([type=hidden]),textarea,select,a,[role=button]')].filter(el=>el.getClientRects().length>0).slice(0, 500);
      function css(el) {
        const parts = [];
        while (el && el.nodeType === 1) {
          const siblings = el.parentElement ? [...el.parentElement.children].filter(s => s.tagName === el.tagName) : [el];
          parts.unshift(el.tagName.toLowerCase() + `:nth-of-type(${siblings.indexOf(el) + 1})`);
          el = el.parentElement;
        }
        return parts.join(' > ');
      }
      return elements.map((el, index) => ({ index, element_token: css(el), actions: [], role:el.tagName, value:typeof el.value==='string'?el.value.slice(0,4096):undefined, label: (el.getAttribute('aria-label') || el.innerText || el.getAttribute('placeholder') || el.tagName).slice(0, 256) }));
    });
    const dom=await page.context().newCDPSession(page);
    try {
      const {root:document}=await dom.send('DOM.getDocument');
      for(const element of tree){
        const {nodeId}=await dom.send('DOM.querySelector',{nodeId:document.nodeId,selector:element.element_token});
        if(!nodeId) fail('STALE_ELEMENT_REFERENCE');
        const {node}=await dom.send('DOM.describeNode',{nodeId});
        element.element_token=JSON.stringify({selector:element.element_token,backendNodeId:node.backendNodeId});
      }
    } finally {await dom.detach();}
    const state = (await page.locator('body').innerText()).slice(0,16384)+'\n'+tree.map(e => `[${e.index}] ${e.role} ${JSON.stringify(e.label)}${e.value===undefined?'':' = '+JSON.stringify(e.value)}`).join('\n');
    const viewport = await page.evaluate(() => ({ x: scrollX, y: scrollY, width: innerWidth, height: innerHeight, scale: devicePixelRatio }));
    const png = a.screenshot ? await page.screenshot({ type: 'png', scale:'css', timeout: 10000 }) : undefined;
    return { tree_markdown: state, elements: tree, navigation: nav, window_bounds: viewport, geometry: JSON.stringify(viewport), screenshot_width: viewport.width, screenshot_height: viewport.height, screenshot_png_b64: png?.toString('base64') };
  }
  if (a.geometry) {
    const current = await page.evaluate(() => JSON.stringify({ x: scrollX, y: scrollY, width: innerWidth, height: innerHeight, scale: devicePixelRatio }));
    if (current !== a.geometry) fail('STALE_SURFACE_HANDLE');
  }
  let elementSelector;
  if(a.element_token){
    let token;try{token=JSON.parse(a.element_token);}catch{fail('STALE_ELEMENT_REFERENCE');}
    const dom=await page.context().newCDPSession(page);
    try{
      const {root:document}=await dom.send('DOM.getDocument');
      const {nodeId}=await dom.send('DOM.querySelector',{nodeId:document.nodeId,selector:token.selector});
      if(!nodeId)fail('STALE_ELEMENT_REFERENCE');
      const {node}=await dom.send('DOM.describeNode',{nodeId});
      if(node.backendNodeId!==token.backendNodeId)fail('STALE_ELEMENT_REFERENCE');
      elementSelector=token.selector;
    }finally{await dom.detach();}
  }
  const locator = () => page.locator(selector(a.selector ?? elementSelector));
  if (method === 'playwright.count') return { value: await locator().count() };
  if (method === 'playwright.textContent') return { value: (await locator().textContent())?.slice(0, 16384) };
  if (method === 'playwright.waitForSelector') { await locator().waitFor({ state: 'visible' }); return {}; }
  if (method === 'playwright.waitForURL') { await page.waitForURL(url(a.url), { timeout: timeout(a.timeoutMs) }); return {}; }
  // Validate before dispatch. A lost transport after this point is ambiguous.
  if (method === 'goto') url(a.url);
  mutating = true;
  switch (method) {
    case 'goto': await page.goto(a.url, { timeout: 10000 }); break;
    case 'back': await page.goBack({ timeout: 10000 }); break;
    case 'forward': await page.goForward({ timeout: 10000 }); break;
    case 'reload': await page.reload({ timeout: 10000 }); break;
    case 'close': await page.close(); break;
    case 'playwright.click': await locator().click(); break;
    case 'playwright.fill': await locator().fill(a.value); break;
    case 'click': {
      const options={button:a.options?.button ?? 'left',clickCount:a.options?.count ?? 1};
      if (a.element_token) await locator().click(options); else await page.mouse.click(a.x, a.y,options);
      break;
    }
    case 'drag': await page.mouse.move(a.x, a.y); await page.mouse.down(); await page.mouse.move(a.to_x, a.to_y); await page.mouse.up(); break;
    case 'scroll': if (a.element_token) await locator().hover(); else await page.mouse.move(a.x, a.y); await page.mouse.wheel(['left','right'].includes(a.direction) ? (a.direction === 'left' ? -1 : 1) * (a.amount ?? 100) : 0, ['up','down'].includes(a.direction) ? (a.direction === 'up' ? -1 : 1) * (a.amount ?? 100) : 0); break;
    case 'setValue': await locator().fill(a.value); break;
    case 'selectText': await locator().evaluate((el, value) => {
      const text = el.value ?? el.textContent;
      const start = text.indexOf(value);
      if (!value || start < 0 || text.indexOf(value, start + value.length) !== -1 || typeof el.setSelectionRange !== 'function') throw new Error('INVALID_ARGUMENT');
      el.focus(); el.setSelectionRange(start, start + value.length);
    }, a.value); break;
    case 'typeText': case 'paste': if (a.element_token) await locator().pressSequentially(a.text); else await page.keyboard.insertText(a.text); break;
    case 'pressKey': if (a.element_token) await locator().press(a.text); else await page.keyboard.press(a.text); break;
    case 'playwright.download': {
      const download = page.waitForEvent('download', { timeout: 10000 });
      await locator().click();
      const stream = await (await download).createReadStream();
      if (!stream) fail('PROVIDER_UNAVAILABLE');
      const chunks = []; let bytes = 0;
      for await (const chunk of stream) { bytes += chunk.length; if (bytes > 8 << 20) fail('PERMISSION_DENIED'); chunks.push(chunk); }
      transfer('write', a.path, Buffer.concat(chunks));
      break;
    }
    case 'playwright.upload': {
      // A buffer supplies bytes to FileChooser without exposing any path to
      // Playwright, the page or its chosen filename.
      const data = transfer('read', a.path);
      const chooser = page.waitForEvent('filechooser', { timeout: 10000 });
      await locator().click();
      await (await chooser).setFiles({ name: a.path.split('/').at(-1), mimeType: 'application/octet-stream', buffer: data });
      break;
    }
    default: fail('INVALID_ARGUMENT');
  }
  return { effect: 'dispatched' };
}
try { process.stdout.write(JSON.stringify(await main())); }
catch (e) { process.stdout.write(JSON.stringify({ error: ['INVALID_ARGUMENT','PERMISSION_DENIED','STALE_SURFACE_HANDLE','STALE_ELEMENT_REFERENCE'].includes(e.code) ? e.code : mutating ? 'UNKNOWN_SIDE_EFFECT' : 'PROVIDER_UNAVAILABLE' })); }
finally { if (browser) await browser.close(); }
