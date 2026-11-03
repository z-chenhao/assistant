#!/bin/bash
# Readiness for the explicitly owned synthetic Guest services, never product discovery.
set -eu
steward_runtime=/Users/lume/steward25-runtime
/usr/bin/python3 -c 'import json; assert json.load(open("/Users/lume/steward25-runtime/owned.json"))=={"issue_id":"01a14029-5486-7700-8253-9d891a1093c9"}'
steward_uid=$(/usr/bin/id -u)
for steward_label in com.steward.fixture.site com.steward.fixture.browser; do
  /bin/launchctl print "gui/$steward_uid/$steward_label" >/dev/null 2>&1 || /bin/launchctl bootstrap "gui/$steward_uid" "/Users/lume/Library/LaunchAgents/$steward_label.plist"
done
"$steward_runtime/node/bin/node" --input-type=module - <<'JS'
import fs from 'node:fs';
let ready;
for(let i=0;i<60;i++){
 try{
 const version=await (await fetch('http://127.0.0.1:9222/json/version',{signal:AbortSignal.timeout(1000)})).json();
 const site=await fetch('http://127.0.0.1:18025/',{signal:AbortSignal.timeout(1000)});
 if(version.webSocketDebuggerUrl && site.ok){ready=version.Browser;break;}
 }catch{}
 await new Promise(r=>setTimeout(r,500));
}
if(!ready){
 for(const label of ['site','browser']){
 const p=`/Users/lume/steward25-runtime/com.steward.fixture.${label}.log`;
 console.log(label,fs.existsSync(p)?fs.readFileSync(p,'utf8').slice(-4096):'no log');
 }
 process.exitCode=1;
}else console.log(JSON.stringify({browser:ready,site:'synthetic Guest loopback',ready:true}));
JS
