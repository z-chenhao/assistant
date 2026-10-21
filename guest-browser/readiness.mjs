import {readFile} from 'node:fs/promises';
import {join} from 'node:path';
import {setTimeout as delay} from 'node:timers/promises';

// Playwright's pipe is ready before Chromium necessarily publishes its CDP port.
export async function waitForDevToolsPort(profile, timeout=5000) {
 const deadline=performance.now()+timeout;
 do {
  try {
   const text=await readFile(join(profile,'DevToolsActivePort'),'utf8');
   const value=text.split('\n')[0];
   if (text.includes('\n') && /^\d+$/.test(value) && Number(value)>0 && Number(value)<=65535) return Number(value);
  } catch (error) {
   if (error.code!=='ENOENT') throw error;
  }
  const remaining=deadline-performance.now();
  if (remaining<=0) throw new Error('doctor: Chromium CDP port not ready');
  await delay(Math.min(25,remaining));
 } while (true);
}
