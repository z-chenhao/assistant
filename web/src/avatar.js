import {t} from './i18n';
/* Steward: pre-rendered complete frames; no runtime WebGL or mesh deformation. */
export const StewardGhost = (() => {
  const reduced = matchMedia('(prefers-reduced-motion: reduce)');
  const states = new Map(), atlases = new Map();
  let observer, timer, paused = false, hidden = document.hidden;
  try { paused = localStorage.getItem('steward-avatar-motion') === 'paused'; } catch {}
  document.documentElement.classList.toggle('ghost-motion-paused', paused);
  function markup(style = 'auto') {
    return `<button type="button" class="steward-avatar" data-ghost-style="${style}" data-action="peek" aria-label="${t('Say hello to Ghost')}" title="${t('Click to say hello')}"><span class="ghost-shadow" aria-hidden="true"></span><span class="ghost-art" aria-hidden="true"><span class="ghost-rest"></span><canvas class="ghost-frames"></canvas></span><span class="ghost-feedback sr-only" role="status"></span></button>`;
  }
  function image(path) {
    if (!atlases.has(path)) {
      const item = new Image(); item.src = path;
      atlases.set(path, item.decode().then(() => item));
    }
    return atlases.get(path);
  }
  async function prepare(state) {
    const tone = getComputedStyle(state.button).getPropertyValue('--ghost-tone').trim();
    const size = state.button.clientWidth >= 100 ? 256 : 96;
    const key = `${tone}-${size}`;
    if (state.assetKey === key) return;
    state.assetKey = key;
    try {
      const atlas = await image(`/brand/ghost-${key}.webp`);
      if (state.assetKey !== key || !state.button.isConnected) return;
      state.atlas = atlas; state.size = size;
      state.canvas.width = state.canvas.height = size;
      state.lastFrame = -1;
      draw(state, 0);
      state.button.classList.add('is-ready');
      state.button.dataset.renderer = 'prerendered';
    } catch { state.button.dataset.renderer = 'static'; }
  }
  function draw(state, frame) {
    if (!state.atlas || state.lastFrame === frame) return;
    state.ctx.globalCompositeOperation = 'copy';
    const size = state.size;
    state.ctx.drawImage(state.atlas, frame % 8 * size, Math.floor(frame / 8) * size, size, size, 0, 0, size, size);
    state.lastFrame = frame;
    state.button.dataset.frame = String(frame);
  }
  function updateVisibility(state) {
    state.button.classList.toggle('is-visible', state.visible && !hidden);
  }
  function tick() {
    clearTimeout(timer);
    const now = performance.now();
    let next = Infinity;
    for (const [button, state] of states) {
      if (!button.isConnected) { states.delete(button); continue; }
      const blocked = paused || reduced.matches || hidden || !state.visible;
      updateVisibility(state);
      let frame = 0;
      if (blocked) { state.waveAt = null; state.blinkAt = null; }
      else if (state.waveAt !== null) {
        const progress = (now - state.waveAt) / 2100;
        if (progress < 1) { frame = Math.floor(progress * 48); next = Math.min(next, 1000 / 24); }
        else { state.waveAt = null; state.nextBlink = now + 8400; next = Math.min(next, 8400); }
      } else {
        if (state.blinkAt === null && now >= state.nextBlink) state.blinkAt = now;
        if (state.blinkAt !== null) {
          const progress = (now - state.blinkAt) / 320;
          if (progress < 1) { frame = 49 + Math.floor(progress * 7); next = Math.min(next, 1000 / 24); }
          else { state.blinkAt = null; state.nextBlink = now + 8400; }
        }
        if (state.blinkAt === null) next = Math.min(next, state.nextBlink - now);
      }
      button.classList.toggle('is-waving', state.waveAt !== null);
      draw(state, frame);
    }
    if (Number.isFinite(next)) timer = setTimeout(tick, Math.max(16, next));
  }
  function react(button) {
    const feedback = button.querySelector('.ghost-feedback');
    if (feedback) feedback.textContent = t('Ghost waves hello.');
    const state = states.get(button), now = performance.now();
    // The fixed button owns hit testing; animated descendants never trigger re-entry.
    if (!state || paused || reduced.matches || hidden || state.waveAt !== null || now < state.cooldown) return;
    state.waveAt = now; state.blinkAt = null; state.cooldown = now + 2800;
    tick();
  }
  function observe() {
    observer?.disconnect();
    for (const [button] of states) if (!button.isConnected) states.delete(button);
    document.querySelectorAll('.steward-avatar').forEach(button => {
      if (!states.has(button)) {
        const canvas = button.querySelector('canvas');
        const ctx = canvas.getContext('2d');
        if (!ctx) { button.dataset.renderer = 'static'; return; }
        states.set(button, {button, canvas, ctx, visible:false, waveAt:null, blinkAt:null, nextBlink:performance.now()+5000+states.size*400, cooldown:0, lastFrame:-1});
      }
      prepare(states.get(button));
      button.onpointerenter = event => { if (event.pointerType !== 'touch') react(button); };
    });
    if ('IntersectionObserver' in window) {
      observer = new IntersectionObserver(entries => {
        entries.forEach(entry => { const state = states.get(entry.target); if (state) state.visible = entry.isIntersecting; });
        tick();
      });
      states.forEach(state => observer.observe(state.button));
    } else states.forEach(state => { state.visible = true; });
    tick();
  }
  function pause(value) {
    paused = value;
    document.documentElement.classList.toggle('ghost-motion-paused', paused);
    try { localStorage.setItem('steward-avatar-motion', paused ? 'paused' : 'on'); } catch {}
    tick();
  }
  document.addEventListener('visibilitychange', () => { hidden = document.hidden; tick(); });
  reduced.addEventListener('change', tick);
  new MutationObserver(() => { states.forEach(prepare); }).observe(document.documentElement, {attributes:true, attributeFilter:['class']});
  window.addEventListener('resize', () => { states.forEach(prepare); });
  return {markup, react, observe, pause, get isPaused() { return paused; }};
})();
