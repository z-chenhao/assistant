// Chromium's pipe connection can be ready before its TCP debugger port file.
// Wait for that specific readiness signal; never retry a browser action.
import {readFile} from 'node:fs/promises';
import {join} from 'node:path';
import {setTimeout as delay} from 'node:timers/promises';

export async function waitForDevToolsPort(profile, timeoutMs=5000){
 const deadline=performance.now()+timeoutMs;
 for(;;){
  try{
   const line=(await readFile(join(profile,'DevToolsActivePort'),'utf8')).split('\n')[0];
   const port=Number(line);
   if(/^\d+$/.test(line)&&Number.isInteger(port)&&port>0&&port<=65535)return port;
  }catch(error){if(error.code!=='ENOENT')throw error}
  const remaining=deadline-performance.now();
  if(remaining<=0)throw new Error('Chromium debugger endpoint not ready');
  await delay(Math.min(25,remaining));
 }
}
