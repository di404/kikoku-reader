// Reader front end: draws the compiled steps the Go side hands over.
// Geometry comes in the original 1024x768 space and is scaled here, so
// images and text are drawn at the window's real resolution.
"use strict";

const W = 1024, H = 768;
const $ = (id) => document.getElementById(id);
const api = () => window.go.main.App;

const state = {
  index: 0, total: 0, scenes: [], settings: null, fonts: [],
  view: null, k: 1, hidden: false, busy: false, queued: null,
  lastLayers: "", movie: false, menu: false,
};

// ---------- layout ----------

function layout() {
  const vw = window.innerWidth, vh = window.innerHeight;
  const k = Math.min(vw / W, vh / H);
  state.k = k;
  const w = W * k, h = H * k;
  const left = (vw - w) / 2, top = (vh - h) / 2;
  for (const el of [$("stage"), $("movie")]) {
    Object.assign(el.style, { left: left + "px", top: top + "px", width: w + "px", height: h + "px" });
  }
  const box = $("box");
  Object.assign(box.style, { left: left + "px", width: w + "px", bottom: top + "px" });
  applyFontSize();
  if (state.view) drawLayers(state.view.step.layers, true);
}

function applyFontSize() {
  const s = state.settings;
  const scale = s ? s.fontScale : 1;
  $("box").style.fontSize = (H * state.k * 0.03 * scale) + "px";
}

// ---------- layers ----------

const blendCSS = { add: "plus-lighter", overlay: "overlay", multiply: "multiply" };

function layerEl(l, k) {
  let el;
  switch (l.kind) {
    case "image":
      el = document.createElement("img");
      el.src = "/res/" + encodeURI(l.src);
      el.decoding = "async";
      el.draggable = false;
      break;
    case "color":
      el = document.createElement("div");
      el.style.background = l.color;
      break;
    case "group":
    case "window":
      el = document.createElement("div");
      el.style.overflow = l.kind === "window" ? "hidden" : "visible";
      for (const c of l.children || []) el.appendChild(layerEl(c, k));
      break;
    default:
      return document.createComment(l.kind);
  }
  el.className = "layer";
  const st = el.style;
  st.left = l.x * k + "px";
  st.top = l.y * k + "px";
  st.width = l.w * k + "px";
  st.height = l.h * k + "px";
  st.zIndex = l.z;
  if (l.opacity < 1) st.opacity = l.opacity;
  if (l.scale) st.transform = `scale(${l.scale}, ${l.scaleY || l.scale})`;
  if (l.blend && blendCSS[l.blend]) st.mixBlendMode = blendCSS[l.blend];
  if (l.filter) st.filter = l.filter;
  if (l.kind === "group") st.isolation = "isolate";
  return el;
}

function drawLayers(layers, instant) {
  const key = JSON.stringify(layers);
  const stage = $("stage");
  if (key === state.lastLayers && !instant) return;
  state.lastLayers = key;
  const scene = document.createElement("div");
  scene.className = "scene";
  for (const l of layers) scene.appendChild(layerEl(l, state.k));
  const old = [...stage.querySelectorAll(".scene")];
  if (instant || old.length === 0) {
    old.forEach((o) => o.remove());
    stage.appendChild(scene);
    return;
  }
  scene.style.opacity = 0;
  stage.appendChild(scene);
  const imgs = [...scene.querySelectorAll("img")];
  const ready = Promise.all(imgs.map((i) => i.decode().catch(() => {})));
  const timeout = new Promise((r) => setTimeout(r, 900));
  Promise.race([ready, timeout]).then(() => {
    requestAnimationFrame(() => {
      scene.style.opacity = 1;
      setTimeout(() => old.forEach((o) => o.remove()), 300);
    });
  });
}

const preloaded = new Map();
function preload(list) {
  for (const src of list || []) {
    if (preloaded.has(src)) continue;
    const img = new Image();
    img.src = "/res/" + encodeURI(src);
    preloaded.set(src, img);
    if (preloaded.size > 40) preloaded.delete(preloaded.keys().next().value);
  }
}

// ---------- text ----------

function drawText(v) {
  const st = v.step;
  const box = $("box"), text = $("text");
  if (!st.html && !v.before) {
    box.classList.add("empty");
    return;
  }
  box.classList.remove("empty");
  $("speaker").textContent = st.speaker || "";
  text.innerHTML = (v.before || "") + `<span class="new">${st.html}</span>`;
}

// ---------- stepping ----------

async function show(i, jumped) {
  if (i < 0 || i >= state.total) return;
  if (state.busy) { state.queued = [i, jumped]; return; }
  state.busy = true;
  try {
    const v = await api().Show(i, !!jumped);
    state.view = v;
    state.index = i;
    drawLayers(v.step.layers, jumped);
    drawText(v);
    preload(v.next);
    if (!jumped && v.step.movies && v.step.movies.length) playMovie(v.step.movies[0].src);
  } catch (e) {
    hint("出错：" + e);
  } finally {
    state.busy = false;
    if (state.queued) {
      const [qi, qj] = state.queued;
      state.queued = null;
      show(qi, qj);
    }
  }
}

function next() {
  if (state.menu) return;
  if (state.movie) { stopMovie(); return; }
  if (state.hidden) { toggleHidden(); return; }
  if (state.index + 1 >= state.total) { hint("全文完"); return; }
  show(state.index + 1, false);
}

function prev() {
  if (state.menu || state.movie) return;
  if (state.hidden) toggleHidden();
  // Step back to the start of the previous box, not mid-box.
  let i = state.index - 1;
  show(Math.max(0, i), true);
}

function toggleHidden() {
  state.hidden = !state.hidden;
  $("box").classList.toggle("hidden", state.hidden);
}

let hintTimer;
function hint(msg) {
  const h = $("hint");
  h.textContent = msg;
  h.classList.add("on");
  clearTimeout(hintTimer);
  hintTimer = setTimeout(() => h.classList.remove("on"), 2200);
}

// ---------- movies ----------

function playMovie(src) {
  const m = $("movie");
  state.movie = true;
  m.src = "/movie/" + encodeURI(src) + ".mp4";
  m.classList.add("on");
  api().StopVoice();
  m.play().catch(() => stopMovie());
  m.onended = stopMovie;
  m.onerror = () => { hint("过场无法播放（需要 ffmpeg）"); stopMovie(); };
}

function stopMovie() {
  const m = $("movie");
  state.movie = false;
  m.pause();
  m.classList.remove("on");
  m.removeAttribute("src");
  m.load();
}

// ---------- settings ----------

function applySettings() {
  const s = state.settings;
  const root = document.documentElement.style;
  root.setProperty("--font", `"${(s.fontFamily || "Songti SC").replace(/"/g, "")}"`);
  root.setProperty("--font-scale", s.fontScale);
  root.setProperty("--line", s.lineHeight);
  root.setProperty("--box", s.boxOpacity);
  applyFontSize();
  loadUserFont(s.fontFile);
}

let userFont = null;
function loadUserFont(file) {
  if (userFont) { document.fonts.delete(userFont); userFont = null; }
  $("fontFileName").textContent = file ? file.split("/").pop() : "";
  if (!file) return;
  const f = new FontFace("UserFont", `url(/font/user?${encodeURIComponent(file)})`);
  f.load().then(() => { document.fonts.add(f); userFont = f; })
    .catch(() => hint("字体文件载入失败"));
}

function fillMenu() {
  const s = state.settings;
  const sel = $("fontFamily");
  sel.innerHTML = "";
  let known = false;
  for (const f of state.fonts) {
    const o = new Option(f.label, f.family);
    if (f.family === s.fontFamily) { o.selected = true; known = true; }
    sel.add(o);
  }
  $("fontCustom").value = known ? "" : s.fontFamily;
  for (const id of ["fontScale", "lineHeight", "boxOpacity", "music", "voice", "effect"]) $(id).value = s[id];

  const ol = $("scenes");
  ol.innerHTML = "";
  state.scenes.forEach((sc, n) => {
    const li = document.createElement("li");
    li.textContent = sc.title;
    if (state.view && state.view.scene === n) li.className = "cur";
    li.onclick = () => { closeMenu(); show(sc.first, true); };
    ol.appendChild(li);
  });
  const cur = state.view ? state.scenes[state.view.scene] : null;
  $("where").textContent = cur ? `${cur.title} · ${state.index + 1} / ${state.total}` : "";
}

function readMenu() {
  const s = state.settings;
  s.fontFamily = $("fontCustom").value.trim() || $("fontFamily").value;
  for (const id of ["fontScale", "lineHeight", "boxOpacity", "music", "voice", "effect"]) s[id] = parseFloat($(id).value);
  applySettings();
  clearTimeout(readMenu.t);
  readMenu.t = setTimeout(() => api().SaveSettings(s).then((r) => { state.settings = r; }), 250);
}

function openMenu() { state.menu = true; fillMenu(); $("menu").classList.remove("hidden"); }
function closeMenu() { state.menu = false; $("menu").classList.add("hidden"); }

// ---------- input ----------

function wire() {
  $("screen").addEventListener("click", (e) => { if (e.button === 0) next(); });
  $("screen").addEventListener("contextmenu", (e) => { e.preventDefault(); if (!state.menu) toggleHidden(); });
  let wheelAt = 0;
  window.addEventListener("wheel", (e) => {
    if (state.menu) return;
    const now = Date.now();
    if (now - wheelAt < 180) return;
    wheelAt = now;
    if (e.deltaY < 0) prev(); else if (e.deltaY > 0) next();
  }, { passive: true });

  let skip = null;
  window.addEventListener("keydown", (e) => {
    if (e.target.tagName === "INPUT" && e.key !== "Escape") return;
    switch (e.key) {
      case " ": case "Enter": case "ArrowRight": case "ArrowDown": case "PageDown":
        e.preventDefault(); next(); break;
      case "ArrowLeft": case "ArrowUp": case "PageUp": case "Backspace":
        e.preventDefault(); prev(); break;
      case "h": case "H":
        if (!state.menu) toggleHidden(); break;
      case "f": case "F":
        api().ToggleFullscreen(); break;
      case "Escape":
        state.menu ? closeMenu() : openMenu(); break;
      case "Control":
        if (!skip && !state.menu) skip = setInterval(() => { if (!state.busy) next(); }, 70);
        break;
    }
  });
  window.addEventListener("keyup", (e) => {
    if (e.key === "Control" && skip) { clearInterval(skip); skip = null; }
  });
  window.addEventListener("blur", () => { if (skip) { clearInterval(skip); skip = null; } });

  $("close").onclick = closeMenu;
  $("menu").addEventListener("click", (e) => { if (e.target.id === "menu") closeMenu(); });
  for (const id of ["fontFamily", "fontCustom", "fontScale", "lineHeight", "boxOpacity", "music", "voice", "effect"]) {
    $(id).addEventListener("input", readMenu);
  }
  $("fontFamily").addEventListener("change", () => { $("fontCustom").value = ""; readMenu(); });
  $("fontFile").onclick = async () => {
    const p = await api().ChooseFontFile();
    if (p) { state.settings.fontFile = p; applySettings(); }
  };
  $("fontFileName").ondblclick = () => { state.settings.fontFile = ""; readMenu(); };
  $("fullscreen").onclick = () => api().ToggleFullscreen();
  $("quit").onclick = () => api().Quit();
  $("gamedir").onclick = pickGameDir;
  $("errorPick").onclick = pickGameDir;
  window.addEventListener("resize", layout);
}

async function pickGameDir() {
  const r = await api().ChooseGameDir();
  closeMenu();
  start(r);
}

function start(r) {
  state.settings = r.settings;
  state.fonts = r.fonts || [];
  applySettings();
  if (r.error) {
    $("errorText").textContent = "无法载入游戏数据：\n" + r.error +
      "\n\n请选择《鬼哭街》2011 重制版的目录（包含 nss.npa、cg.npa、voice.npa 的文件夹）。";
    $("error").classList.remove("hidden");
    return;
  }
  $("error").classList.add("hidden");
  state.scenes = r.scenes;
  state.total = r.total;
  layout();
  $("box").classList.remove("hidden");
  // "#123" in the URL starts at that step (used by the browser preview)
  const at = parseInt(location.hash.slice(1), 10);
  if (at >= 0) { show(Math.min(at, r.total - 1), true); return; }
  const last = r.settings.last || 0;
  show(Math.min(last, r.total - 1), true);
  if (last > 0) hint("从上次读到的地方继续 · Esc 打开章节");
}

window.addEventListener("DOMContentLoaded", async () => {
  wire();
  layout();
  start(await api().Init());
});
