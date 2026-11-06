// Steward host for the pinned pi-web-access core. No Pi session, auth files,
// ambient provider credentials or browser profile is inherited from the host.
import { createInterface } from 'node:readline';
import { mkdir, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import { pathToFileURL } from 'node:url';
import { redactCredentials } from './web-access-network.mjs';

const secrets = Object.entries(process.env).filter(([name]) => /_(KEY|TOKEN|SECRET|PASSWORD)$/.test(name)).map(([,value]) => value);
const output = value => process.stdout.write(JSON.stringify(redactCredentials(value, secrets)) + '\n');
console.log = console.info = console.debug = (...args) => console.error(...args);
const lines = createInterface({ input: process.stdin, crlfDelay: Infinity });
let invoke;
let nextCall = 0;
const callbacks = new Map();
const controller = new AbortController();
function modelCall(request, signal = controller.signal) {
  const id = String(++nextCall);
  return new Promise((resolve, reject) => {
    const abort = () => {
      callbacks.delete(id);
      output({ type: 'model_cancel', id });
      reject(signal.reason ?? new Error('GENERATION_CANCELLED'));
    };
    if (signal.aborted) { reject(signal.reason ?? new Error('GENERATION_CANCELLED')); return; }
    signal.addEventListener('abort', abort, { once: true });
    const cleanup = () => signal.removeEventListener('abort', abort);
    callbacks.set(id, { resolve: value => { cleanup(); resolve(value); }, reject: error => { cleanup(); reject(error); } });
    output({ type: 'model_request', id, request });
  });
}
lines.on('line', line => {
  let message;
  try { message = JSON.parse(line); } catch { controller.abort(); return; }
  if (message.type === 'invoke' && !invoke) {
    invoke = message;
    void execute(message).catch(error => {
      output({ type: 'error', code: 'WEB_ACCESS_FAILED', message: String(error?.message ?? error) });
      process.exitCode = 1;
      lines.close();
    });
  } else if (message.type === 'model_result') {
    const callback = callbacks.get(message.id);
    if (!callback) return;
    callbacks.delete(message.id);
    if (message.error) callback.reject(new Error(message.error));
    else callback.resolve(message.result);
  } else if (message.type === 'cancel') controller.abort();
});
lines.on('close', () => controller.abort());
process.once('SIGTERM', () => { controller.abort(); process.exit(1); });

function nativeResponse(result) {
  const search = { id: 'steward-search', type: 'web_search_call', status: 'completed', action: { type: 'search', sources: (result.web_search?.sources ?? []).map(source => ({ type: 'url', ...source })) } };
  const answer = { id: 'steward-answer', type: 'message', role: 'assistant', content: [{ type: 'output_text', text: result.text ?? '', annotations: [] }] };
  const events = [
    { type: 'response.output_text.delta', delta: result.text ?? '' },
    { type: 'response.output_item.done', item: search },
    { type: 'response.output_item.done', item: answer },
    { type: 'response.completed', response: { id: 'steward-search', status: 'completed', output: [search, answer] } },
  ];
  return new Response(events.map(event => 'data: ' + JSON.stringify(event) + '\n\n').join(''), { headers: { 'content-type': 'text/event-stream' } });
}

async function execute(message) {
  const root = process.env.PI_CODING_AGENT_DIR;
  if (!root || process.env.HOME !== root) throw new Error('Private Web Access root is required');
  await mkdir(root, { recursive: true, mode: 0o700 });
  await writeFile(join(root, 'web-search.json'), JSON.stringify(message.options ?? {}), { mode: 0o600 });
  const upstream = pathToFileURL(join(import.meta.dirname, 'vendor/web-access/')).href;
  const [{ createWebToolCore, DEFAULT_TOOL_NAMES, getMaxInlineContentChars, resolveFetchModeConfig }, storage] = await Promise.all([
    import(upstream + 'web-tool-core.ts'), import(upstream + 'storage.ts'),
  ]);
  const options = message.options ?? {};
  options.githubClone = { ...options.githubClone, clonePath: join(root, 'github-repos') };
  options.pdf = { ...options.pdf, outputDir: join(root, 'pdf') };
  await writeFile(join(root, 'web-search.json'), JSON.stringify(options), { mode: 0o600 });
  globalThis.__stewardResizeImage = async (data, mimeType) => {
    const result = await modelCall({ kind: 'resize_image', image: { data: Buffer.from(data).toString('base64'), mimeType } });
    return result.image;
  };
  const [{ pinnedFetch, credentialHeaders }, { validateRemoteUrl }, { getActiveProxy }] = await Promise.all([
    import('./web-access-network.mjs'), import(upstream + 'ssrf-protection.ts'), import(upstream + 'utils.ts'),
  ]);
  const nativeFetch=globalThis.fetch;
  globalThis.fetch=(input,init)=>nativeFetch(input,init?.headers ? {...init,headers:credentialHeaders(init.headers)} : init);
  globalThis.__stewardPublicFetch = pinnedFetch(validateRemoteUrl, getActiveProxy, options);
  const settings = {
    toolNames: Object.fromEntries(Object.entries(DEFAULT_TOOL_NAMES).map(([key, name]) => [key, 'builtin__' + name])),
    enabledTools: { webSearch: true, fetchContent: true, getSearchContent: true, sourceCheck: true },
    maxInlineContentChars: getMaxInlineContentChars(options),
    fetchModes: resolveFetchModeConfig(options),
  };
  for (const data of message.restore ?? []) storage.storeResult(data.id, data);
  const core = createWebToolCore({
    settings,
    storeFetchedContent(id, data) { storage.storeResult(id, data); },
    async answerFromPage(request, context, signal) {
      const { answerFromPage } = await import(upstream + 'page-query.ts');
      const scoped = { ...context, modelRegistry: { ...context.modelRegistry, complete: complete('page_answer') } };
      const answer = await answerFromPage(request, scoped, signal);
      return { text: answer.text };
    },
  });
  let extensionContext;
  if (message.model) {
    const model = { ...message.model, input: ["text"], reasoning: false, cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 } };
    const placeholder = 'steward-host-managed';
    extensionContext = {
      model,
      scopedModels: [],
      sessionManager: { getSessionId: () => message.runId },
      modelRegistry: {
        getAll: () => [model],
        getAvailable: () => [model],
        find: (provider, id) => provider === model.provider && id === model.id ? model : undefined,
        complete: (...args) => complete('summary')(...args),
        isUsingOAuth: () => model.oauth === true,
        getApiKeyAndHeaders: async () => ({ ok: true, apiKey: placeholder, baseUrl: model.baseUrl, headers: {} }),
      },
    };
    // Narrow vendored hook: account requests go through Steward's fenced,
    // budgeted Provider Bridge. The Node process never receives that token.
    globalThis.__stewardModelFetch = async (url, init) => {
      if (new Headers(init.headers).get('authorization') !== 'Bearer ' + placeholder) return null;
      const body = JSON.parse(init.body);
      if (!String(url).endsWith('/responses')) return new Response('Unsupported current-account endpoint', { status: 501 });
      try {
        return nativeResponse(await modelCall({ kind: 'hosted_search', body }, init.signal));
      } catch (error) {
        const code = String(error.message);
        const status = { MODEL_INCOMPATIBLE: 400, MODEL_NOT_FOUND: 404, PROVIDER_UNAVAILABLE: 503, RATE_LIMITED: 429, AUTH_REQUIRED: 401 }[code];
        if (status) return new Response(JSON.stringify({ error: { message: code } }), { status, headers: { 'content-type': 'application/json' } });
        if (init.signal?.aborted && !controller.signal.aborted) throw error;
        controller.abort(error);
        throw error;
      }
    };
  }
  const args = message.args ?? {};
  if (args.proxy && args.proxy !== options.proxy) throw new Error('WEB_PROXY_REQUIRES_FROZEN_CONFIGURATION');
  if (message.name === 'fetch_content') {
    const urls = args.urls?.length ? args.urls : args.url ? [args.url] : [];
    if (urls.some(url => !/^https?:\/\//i.test(url) && !message.localPaths?.includes(url))) throw new Error('WEB_LOCAL_INPUT_REQUIRES_ARTIFACT');
  }
  const call = { extensionContext, awaitContent: true };
  const workflow = args.workflow ?? options.workflow ?? 'none';
  if (message.name === 'web_search' && workflow !== 'none') {
    call.summarize = async results => {
      const { generateSummaryDraft, buildDeterministicSummary } = await import(upstream + 'summary-review.ts');
      const draft = extensionContext
        ? await generateSummaryDraft(results, extensionContext, controller.signal, options.summaryModel, undefined, complete('summary'), options.summaryGenerationDeadlineMs, options.summaryInstructions)
        : buildDeterministicSummary(results);
      if (workflow === 'summary-review') {
        const returned = core.buildSearchReturn({ queryList: results.map(result => result.query), results, urls: results.flatMap(result => result.results.map(source => source.url)), includeContent: args.includeContent ?? false });
        returned.content.unshift({ type: 'text', text: 'Summary draft — user review is required; this is not an approved summary.\n\n' + draft.summary });
        returned.details = { ...returned.details, reviewRequired: true, summaryMeta: draft.meta };
        return returned;
      }
      return { approvedSummary: draft.summary, summaryMeta: draft.meta };
    };
  }
  let result;
  switch (message.name) {
    case 'web_search': result = await core.webSearch(args, controller.signal, call); break;
    case 'fetch_content': result = await core.fetchContent(args, controller.signal, call); break;
    case 'get_search_content': result = await core.getSearchContent(args, controller.signal); break;
    case 'source_check': result = await core.sourceCheck(args, controller.signal, call); break;
    default: throw new Error('Unknown Web Access operation');
  }
  const detail = result.details ?? {};
  if (detail.error !== undefined || (detail.queryCount > 0 && detail.successfulQueries === 0) || (detail.urlCount > 0 && detail.successful === 0) || (detail.searchCount > 0 && detail.artifact?.errors?.length === detail.searchCount)) result.isError = true;
  const { exportRepositories } = await import('./web-access-artifacts.mjs');
  const exported = await exportRepositories(root, result, storage.getAllResults(), controller.signal);
  output({ type: 'result', ...exported });
  lines.close();
}

function complete(kind) {
  return async (model, context, options = {}) => {
    const messages = [];
    if (context.systemPrompt) messages.push({ role: 'system', text: context.systemPrompt });
    for (const message of context.messages ?? []) {
      if (!['user', 'system'].includes(message.role)) throw new Error('Unsupported model-tool role');
      messages.push({ role: message.role, text: (message.content ?? []).filter(part => part.type === 'text').map(part => part.text).join('\n') });
    }
    const result = await modelCall({ kind, model: model.id, messages }, options.signal);
    return { role: 'assistant', content: [{ type: 'text', text: result.text }], stopReason: 'stop', model: model.id, provider: model.provider };
  };
}
