// Doctor for the pinned Playwright cache. This never installs or downloads.
const {accessSync, constants} = require('node:fs');
const {execFileSync} = require('node:child_process');
const base = '../guest-browser/node_modules/playwright-core';
const {chromium} = require(base);
// Private to this project's locked Playwright version. Avoid duplicating its
// platform/revision mapping (including platform-specific revision overrides).
const {registry, browserDirectoryToMarkerFilePath} = require(base + '/lib/coreBundle').registry;
const manifest = require(base + '/browsers.json');

(async () => {
  const result = [];
  for (const name of ['chromium', 'chromium-headless-shell', 'ffmpeg']) {
    const executable = registry.findExecutable(name);
    const path = executable.executablePath();
    accessSync(browserDirectoryToMarkerFilePath(executable.directory), constants.R_OK);
    accessSync(path, constants.X_OK);
    if (name === 'ffmpeg') {
      execFileSync(path, ['-version'], {timeout: 10000, stdio: 'pipe'});
    } else {
      const browser = await chromium.launch({executablePath: path, headless: true, timeout: 15000});
      try {
        const version = browser.version();
        if (version !== manifest.browsers.find(item => item.name === name).browserVersion)
          throw new Error(`${name}: cached browser version ${version} differs from locked manifest`);
        const page = await browser.newPage();
        await page.goto('data:text/html,<title>erii-setup</title>');
        if (await page.title() !== 'erii-setup') throw new Error(`${name}: browser smoke failed`);
      } finally {
        await browser.close();
      }
    }
    result.push({name, path});
  }
  console.log(JSON.stringify(result));
})().catch(error => {
  console.error(`Locked browser cache missing, incompatible or damaged: ${error.message}`);
  process.exitCode = 1;
});
