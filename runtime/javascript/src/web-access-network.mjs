// Actual-connect DNS pinning for public-page requests. The upstream preflight
// remains in place; this checks the answers used by the socket as well.
import { lookup } from 'node:dns/promises';
import { BlockList, isIP } from 'node:net';
import { Agent, fetch as undiciFetch } from 'undici';

function ranges(values) {
  const list = new BlockList();
  for (const value of values) {
    const [address, prefix] = value.split('/');
    const family = isIP(address) === 6 ? 'ipv6' : 'ipv4';
    if (prefix === undefined) list.addAddress(address, family);
    else list.addSubnet(address, Number(prefix), family);
  }
  return list;
}
const reserved = ranges(['0.0.0.0/8', '10.0.0.0/8', '100.64.0.0/10', '127.0.0.0/8', '169.254.0.0/16', '172.16.0.0/12', '192.0.0.0/24', '192.0.2.0/24', '192.88.99.0/24', '192.168.0.0/16', '198.18.0.0/15', '198.51.100.0/24', '203.0.113.0/24', '224.0.0.0/4', '240.0.0.0/4', '::/96', '64:ff9b::/96', '64:ff9b:1::/48', '100::/64', '2001::/23', '2001:db8::/32', '2002::/16', '3fff::/20', 'fc00::/7', 'fe80::/10', 'ff00::/8']);
export function publicAddress(address, allowedRanges = []) {
  if (!isIP(address)) return false;
  const family = isIP(address) === 6 ? 'ipv6' : 'ipv4';
  return ranges(allowedRanges).check(address, family) || !reserved.check(address, family);
}

export function pinnedFetch(validate, getProxy, options, fetchImpl = undiciFetch, resolve = lookup) {
  return async (target, init, policy = {}) => {
    // Explicit proxy DNS policy is owned by the configured proxy and upstream
    // ssrf.trustEnvProxy checks. Direct connections never bypass address checks.
    if (init?.__proxy || getProxy()) return globalThis.fetch(target, init);
    const url = new URL(target);
    const allowed = [...(options.ssrf?.allowRanges ?? []), ...(policy.allowRanges ?? []), ...(policy.allowLoopback ? ["127.0.0.0/8", "::1/128"] : [])];
    const addresses = isIP(url.hostname.replace(/^\[|\]$/g, ''))
      ? [{ address: url.hostname.replace(/^\[|\]$/g, ''), family: isIP(url.hostname.replace(/^\[|\]$/g, '')) }]
      : await resolve(url.hostname, { all: true });
    if (!addresses.length || addresses.some(item => !publicAddress(item.address, allowed))) throw new Error('WEB_ADDRESS_BLOCKED');
    await validate(url, { ...policy, allowRanges: allowed, lookup: async () => addresses });
    const agent = new Agent({ connect: { lookup: (_host, requested, callback) => {
      const matches = addresses.filter(item => !requested.family || requested.family === item.family);
      if (!matches.length) return callback(new Error('WEB_ADDRESS_BLOCKED'));
      if (requested.all) callback(null, matches);
      else callback(null, matches[0].address, matches[0].family);
    } } });
    return fetchImpl(target, { ...init, headers: credentialHeaders(init?.headers), dispatcher: agent });
  };
}

// Headers supplied in owner configuration use the same SecretStore-backed
// environment references as API keys. Never write resolved values to config.
export function credentialHeaders(input) {
  const headers = new Headers(input);
  for (const [name,value] of headers) {
    const match = /^\$([A-Z_][A-Z0-9_]*)$/.exec(value);
    if (!match) continue;
    const secret=process.env[match[1]];
    if (!secret) throw new Error('AUTH_REQUIRED');
    headers.set(name,secret);
  }
  return headers;
}

export function redactCredentials(value, secrets) {
  if (typeof value === 'string') {
    for (const secret of secrets) if (secret) value = value.replaceAll(secret, '[redacted]');
    return value;
  }
  if (Array.isArray(value)) return value.map(item => redactCredentials(item, secrets));
  if (value && typeof value === 'object') return Object.fromEntries(Object.entries(value).map(([key, item]) => [redactCredentials(key, secrets), redactCredentials(item, secrets)]));
  return value;
}
