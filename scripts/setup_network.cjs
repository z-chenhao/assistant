// Opt-in measurement, not a proxy: keep registry/TLS/integrity policy intact.
// Only hosts and request categories are recorded, never headers or URL queries.
const fs = require('node:fs');
const {createHash} = require('node:crypto');
for (const protocol of ['http', 'https']) {
  const mod = require('node:' + protocol), original = mod.request;
  mod.request = function (...args) {
    const req = original.apply(this, args);
    let bytes = 0, status = null;
    const kind = req.path.includes('/-/npm/v1/security/') ? 'audit' :
      /\.tgz(?:\?|$)/.test(req.path) ? 'npm-payload' :
      /playwright|microsoft/.test(req.host) ? 'browser' : 'other';
    const object = createHash('sha256').update(req.host + req.path.split('?')[0]).digest('hex');
    req.on('response', res => {
      status = res.statusCode;
      const emit = res.emit;
      // Observe bytes without putting the stream into flowing mode.
      res.emit = function (event, ...items) {
        if (event === 'data') bytes += Buffer.byteLength(items[0]);
        return emit.call(this, event, ...items);
      };
    });
    req.on('close', () => fs.appendFileSync(process.env.COLLIE_SETUP_NETWORK_LOG,
      JSON.stringify({pid: process.pid, protocol, host: req.host, kind, status,
        object_sha256: object, response_body_bytes: bytes}) + '\n'));
    return req;
  };
  // Native get() holds an internal request reference; route it through the
  // same observer without changing its request(...).end() semantics.
  mod.get = function (...args) { const req = mod.request(...args); req.end(); return req; };
}
