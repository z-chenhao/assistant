#!/bin/bash
# Run only in the explicitly owned synthetic macOS Guest. No personal profile.
set -eu
erii_runtime=/Users/lume/erii25-runtime
test ! -L "$erii_runtime"
if [ ! -d "$erii_runtime" ]; then
  mkdir -m 700 "$erii_runtime"
  printf '%s' '{"issue_id":"01a14029-5486-7700-8253-9d891a1093c9"}' > "$erii_runtime/owned.json"
fi
/usr/bin/python3 -c 'import json; assert json.load(open("/Users/lume/erii25-runtime/owned.json"))=={"issue_id":"01a14029-5486-7700-8253-9d891a1093c9"}'
if [ ! -x "$erii_runtime/node/bin/node" ]; then
  /usr/bin/curl --fail --location --silent --show-error --max-time 90 https://nodejs.org/dist/v24.21.0/node-v24.21.0-darwin-arm64.tar.gz -o "$erii_runtime/node.tar.gz"
  test "$(/usr/bin/shasum -a 256 "$erii_runtime/node.tar.gz" | /usr/bin/awk '{print $1}')" = bed7eea5325e1108f32ce5228ddd6a5f0f08a499ee42aa7442aea583702f6057
  /usr/bin/tar -xzf "$erii_runtime/node.tar.gz" -C "$erii_runtime"
  mv "$erii_runtime/node-v24.21.0-darwin-arm64" "$erii_runtime/node"
fi
export PATH="$erii_runtime/node/bin:/usr/bin:/bin:/usr/sbin:/sbin"
test "$(node --version)" = v24.21.0
mkdir -p "$erii_runtime/browser"
cp /Users/lume/erii24-fixtures/erii25-package.json "$erii_runtime/browser/package.json"
cp /Users/lume/erii24-fixtures/erii25-package-lock.json "$erii_runtime/browser/package-lock.json"
npm ci --prefix "$erii_runtime/browser" --ignore-scripts --no-audit --no-fund
cp /Users/lume/erii24-fixtures/erii25-transfer.py "$erii_runtime/browser/node_modules/playwright-core/erii-transfer.py"
node "$erii_runtime/browser/node_modules/playwright-core/cli.js" install chromium
cp /Users/lume/erii24-fixtures/erii25-site.mjs "$erii_runtime/site.mjs"
/usr/bin/python3 - <<'PY'
import json,os,plistlib,subprocess
from pathlib import Path
root=Path('/Users/lume/erii25-runtime')
node=str(root/'node/bin/node')
chrome=subprocess.check_output([node,'-e','console.log(require("/Users/lume/erii25-runtime/browser/node_modules/playwright-core").chromium.executablePath())'],text=True).strip()
assert chrome.startswith('/Users/lume/Library/Caches/ms-playwright/')
folder=Path('/Users/lume/Library/LaunchAgents');folder.mkdir(exist_ok=True)
for label,arguments in {
 'com.erii.fixture.site':[node,str(root/'site.mjs')],
 'com.erii.fixture.browser':[chrome,'--headless=new','--remote-debugging-address=127.0.0.1','--remote-debugging-port=9222','--no-first-run','--user-data-dir='+str(root/'profile'),'about:blank']
}.items():
 target=folder/(label+'.plist')
 if target.is_symlink():raise ValueError('unsafe launch fixture')
 target.write_bytes(plistlib.dumps({'Label':label,'ProgramArguments':arguments,'RunAtLoad':True,'KeepAlive':True,'StandardOutPath':str(root/(label+'.log')),'StandardErrorPath':str(root/(label+'.log'))}))
print(json.dumps({'node':'24.21.0','playwright_core':'1.64.1','profile':'isolated synthetic Guest','start':'next Guest login'}))
PY
