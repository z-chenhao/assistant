#!/bin/bash
# Run only in the explicitly owned synthetic macOS Guest. No personal profile.
set -eu
collie_runtime=/Users/lume/collie25-runtime
test ! -L "$collie_runtime"
if [ ! -d "$collie_runtime" ]; then
  mkdir -m 700 "$collie_runtime"
  printf '%s' '{"issue_id":"01a14029-5486-7700-8253-9d891a1093c9"}' > "$collie_runtime/owned.json"
fi
/usr/bin/python3 -c 'import json; assert json.load(open("/Users/lume/collie25-runtime/owned.json"))=={"issue_id":"01a14029-5486-7700-8253-9d891a1093c9"}'
if [ ! -x "$collie_runtime/node/bin/node" ]; then
  /usr/bin/curl --fail --location --silent --show-error --max-time 90 https://nodejs.org/dist/v24.21.0/node-v24.21.0-darwin-arm64.tar.gz -o "$collie_runtime/node.tar.gz"
  test "$(/usr/bin/shasum -a 256 "$collie_runtime/node.tar.gz" | /usr/bin/awk '{print $1}')" = bed7eea5325e1108f32ce5228ddd6a5f0f08a499ee42aa7442aea583702f6057
  /usr/bin/tar -xzf "$collie_runtime/node.tar.gz" -C "$collie_runtime"
  mv "$collie_runtime/node-v24.21.0-darwin-arm64" "$collie_runtime/node"
fi
export PATH="$collie_runtime/node/bin:/usr/bin:/bin:/usr/sbin:/sbin"
test "$(node --version)" = v24.21.0
mkdir -p "$collie_runtime/browser"
cp /Users/lume/collie24-fixtures/collie25-package.json "$collie_runtime/browser/package.json"
cp /Users/lume/collie24-fixtures/collie25-package-lock.json "$collie_runtime/browser/package-lock.json"
npm ci --prefix "$collie_runtime/browser" --ignore-scripts --no-audit --no-fund
cp /Users/lume/collie24-fixtures/collie25-transfer.py "$collie_runtime/browser/node_modules/playwright-core/collie-transfer.py"
node "$collie_runtime/browser/node_modules/playwright-core/cli.js" install chromium
cp /Users/lume/collie24-fixtures/collie25-site.mjs "$collie_runtime/site.mjs"
/usr/bin/python3 - <<'PY'
import json,os,plistlib,subprocess
from pathlib import Path
root=Path('/Users/lume/collie25-runtime')
node=str(root/'node/bin/node')
chrome=subprocess.check_output([node,'-e','console.log(require("/Users/lume/collie25-runtime/browser/node_modules/playwright-core").chromium.executablePath())'],text=True).strip()
assert chrome.startswith('/Users/lume/Library/Caches/ms-playwright/')
folder=Path('/Users/lume/Library/LaunchAgents');folder.mkdir(exist_ok=True)
for label,arguments in {
 'com.collie.fixture.site':[node,str(root/'site.mjs')],
 'com.collie.fixture.browser':[chrome,'--headless=new','--remote-debugging-address=127.0.0.1','--remote-debugging-port=9222','--no-first-run','--user-data-dir='+str(root/'profile'),'about:blank']
}.items():
 target=folder/(label+'.plist')
 if target.is_symlink():raise ValueError('unsafe launch fixture')
 target.write_bytes(plistlib.dumps({'Label':label,'ProgramArguments':arguments,'RunAtLoad':True,'KeepAlive':True,'StandardOutPath':str(root/(label+'.log')),'StandardErrorPath':str(root/(label+'.log'))}))
print(json.dumps({'node':'24.21.0','playwright_core':'1.64.1','profile':'isolated synthetic Guest','start':'next Guest login'}))
PY
