// e2e_jit_parity.mjs - scripted browser parity check for the IE64 wasm JIT.
//
// Drives the /demo/ page twice in headless Chromium over CDP: once with the
// JIT (default) and once with ?jit=0. In each run it waits for the first
// rendered frame, types a BASIC programme exercising integer and FP64 code,
// RUNs it, and captures a burst of screenshots. The caller composes each
// burst into a cursor-free image (per-pixel minimum: the blinking cursor is
// bright-on-dark, so any frame with it off wins) and byte-compares the two.
//
// Usage: node e2e_jit_parity.mjs <baseURL> <jit|nojit> <outPrefix>

const [base, mode, prefix] = process.argv.slice(2);
if (!base || !mode || !prefix) {
  console.error("usage: node e2e_jit_parity.mjs <baseURL> <jit|nojit> <outPrefix>");
  process.exit(2);
}
const url = base + "/demo/" + (mode === "nojit" ? "?jit=0" : "");

const PROGRAM = [
  "10 A=0",
  "20 FOR I=1 TO 50000",
  "30 A=A+I*0.25+SQR(I)",
  "40 NEXT I",
  "50 B=0",
  "60 FOR I=1 TO 200000",
  "70 B=B+I MOD 7",
  "80 NEXT I",
  '90 PRINT "R1=";A',
  '100 PRINT "R2=";B',
  "RUN",
];

const debugPort = 9333;
const { spawn } = await import("node:child_process");
const chrome = spawn("chromium", [
  "--headless=new", "--disable-gpu", "--no-sandbox",
  `--remote-debugging-port=${debugPort}`,
  "--window-size=1280,800", "about:blank",
], { stdio: "ignore" });
process.on("exit", () => chrome.kill());
process.on("unhandledRejection", (e) => { console.error("UNHANDLED: " + e); process.exit(3); });

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

async function newTarget() {
  for (let i = 0; i < 50; i++) {
    try {
      const r = await fetch(`http://127.0.0.1:${debugPort}/json/new?${encodeURIComponent(url)}`, { method: "PUT" });
      if (r.ok) return r.json();
    } catch {}
    await sleep(200);
  }
  throw new Error("chromium devtools never came up");
}

console.error("devtools up, opening " + url);
const target = await newTarget();
console.error("target " + target.id);
const ws = new WebSocket(target.webSocketDebuggerUrl);
await new Promise((res, rej) => { ws.onopen = res; ws.onerror = rej; });

let seq = 0;
let jitInstalled = false;
const pending = new Map();
ws.onmessage = (ev) => {
  const msg = JSON.parse(ev.data);
  if (msg.method === "Runtime.consoleAPICalled") {
    const text = (msg.params.args || []).map((a) => a.value).join(" ");
    if (text.includes("wasm JIT: first block installed")) jitInstalled = true;
    return;
  }
  if (msg.id && pending.has(msg.id)) {
    const { res, rej } = pending.get(msg.id);
    pending.delete(msg.id);
    msg.error ? rej(new Error(msg.error.message)) : res(msg.result);
  }
};
function cdp(method, params = {}) {
  const id = ++seq;
  ws.send(JSON.stringify({ id, method, params }));
  return new Promise((res, rej) => {
    pending.set(id, { res, rej });
    setTimeout(() => {
      if (pending.has(id)) {
        pending.delete(id);
        rej(new Error("CDP timeout: " + method + " id " + id));
      }
    }, 120000);
  });
}

console.error("ws connected");
await cdp("Runtime.enable");
await cdp("Page.enable");

async function evalJS(expr) {
  const r = await cdp("Runtime.evaluate", { expression: expr, returnByValue: true });
  return r.result ? r.result.value : undefined;
}

// Wait for the machine's first rendered frame.
for (let i = 0; i < 300; i++) {
  if (await evalJS("!!window.ieFirstFrame")) break;
  await sleep(200);
  if (i === 299) throw new Error("machine never rendered a frame");
}
console.error("first frame seen");
await sleep(1500); // BASIC prompt settles

// Type the programme from inside the page: DOM KeyboardEvents paced by
// page timers. Ebiten reads chars from keydown "key" values and key state
// from "code", and the page's own event loop stays in sync with the
// machine's cooperative yields; driving the CDP input pipeline from outside
// stalls behind the busy main thread instead.
function send(method, params) {
  ws.send(JSON.stringify({ id: ++seq, method, params }));
}

async function typeProgram(lines) {
  const script = `(async () => {
    const t = document.querySelector('canvas') || document;
    const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
    const ev = (type, key, code) => t.dispatchEvent(new KeyboardEvent(type, { key, code, bubbles: true, cancelable: true }));
    for (const line of ${JSON.stringify(lines)}) {
      for (const ch of line) {
        ev('keydown', ch, ''); await sleep(25);
        ev('keyup', ch, ''); await sleep(10);
      }
      window.__typedProgress = line; ev('keydown', 'Enter', 'Enter'); await sleep(120);
      ev('keyup', 'Enter', 'Enter'); await sleep(150);
    }
    window.__typedDone = true;
  })()`;
  const r = await cdp("Runtime.evaluate", { expression: script });
  if (r.exceptionDetails) throw new Error("typing script threw: " + JSON.stringify(r.exceptionDetails).slice(0, 300));
  console.error("typing script accepted");
  for (let i = 0; i < 60; i++) {
    await sleep(15000);
    if (await evalJS("!!window.__typedDone")) return;
    if (i % 10 === 0) console.error("still typing... " + (await evalJS("window.__typedProgress ?? ''")));
  }
  throw new Error("typing never completed");
}

console.error("typing");
await typeProgram(PROGRAM);
console.error("typed; waiting for RUN");

// Let the programme finish (interpreter run takes the longest).
await sleep(45000);

// Screencast instead of captureScreenshot: the compositor pushes frames
// even while the machine saturates the main thread, whereas one-shot
// capture can starve behind it indefinitely.
const { writeFileSync } = await import("node:fs");
let saved = 0;
let lastSave = 0;
const done = new Promise((resolve) => {
  const base = ws.onmessage;
  ws.onmessage = (ev) => {
    const msg = JSON.parse(ev.data);
    if (msg.method === "Page.screencastFrame") {
      send("Page.screencastFrameAck", { sessionId: msg.params.sessionId });
      const now = Date.now();
      if (now - lastSave >= 400 && saved < 6) {
        writeFileSync(`${prefix}_${saved}.png`, Buffer.from(msg.params.data, "base64"));
        saved++;
        lastSave = now;
        if (saved === 6) resolve();
      }
      return;
    }
    if (base) base(ev);
  };
});
send("Page.startScreencast", { format: "png", everyNthFrame: 2 });
await Promise.race([done, sleep(120000)]);
send("Page.stopScreencast", {});
if (saved < 6) throw new Error("only " + saved + " screencast frames captured");

if (mode !== "nojit" && !jitInstalled) {
  console.error("FAIL: JIT never installed a block during the run");
  process.exit(4);
}
if (mode === "nojit" && jitInstalled) {
  console.error("FAIL: JIT installed a block despite ?jit=0");
  process.exit(4);
}
console.log("captured " + prefix + (jitInstalled ? " (JIT engaged)" : " (interpreter only)"));
ws.close();
chrome.kill();
process.exit(0);
