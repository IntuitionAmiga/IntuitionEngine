// e2e_crt_filter.mjs - browser GPU smoke gate for the final CRT filter.
// Usage: node e2e_crt_filter.mjs <baseURL>

const [base] = process.argv.slice(2);
if (!base) throw new Error("usage: node e2e_crt_filter.mjs <baseURL>");

const { spawn } = await import("node:child_process");
const chrome = spawn("chromium", ["--headless=new", "--no-sandbox", "--remote-debugging-port=9334", "--window-size=960,720", "about:blank"]);
process.on("exit", () => chrome.kill());
const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

let target;
for (let i = 0; i < 50; i++) {
  try {
    const response = await fetch("http://127.0.0.1:9334/json/new?" + encodeURIComponent(base + "/demo/"), { method: "PUT" });
    if (response.ok) { target = await response.json(); break; }
  } catch {}
  await sleep(200);
}
if (!target) throw new Error("Chromium devtools did not start");
const ws = new WebSocket(target.webSocketDebuggerUrl);
await new Promise((resolve, reject) => { ws.onopen = resolve; ws.onerror = reject; });
let nextID = 0;
const pending = new Map();
ws.onmessage = (event) => {
  const message = JSON.parse(event.data);
  if (message.id && pending.has(message.id)) {
    const { resolve, reject } = pending.get(message.id);
    pending.delete(message.id);
    message.error ? reject(new Error(message.error.message)) : resolve(message.result);
  }
};
function cdp(method, params = {}) {
  const id = ++nextID;
  ws.send(JSON.stringify({ id, method, params }));
  return new Promise((resolve, reject) => pending.set(id, { resolve, reject }));
}
async function evaluate(expression) {
  const result = await cdp("Runtime.evaluate", { expression, returnByValue: true });
  return result.result?.value;
}
await cdp("Runtime.enable");
await cdp("Page.enable");
for (let i = 0; i < 300 && !await evaluate("!!window.ieFirstFrame"); i++) await sleep(200);
if (!await evaluate("!!window.ieFirstFrame")) throw new Error("demo did not render a first frame");
await sleep(500);
const before = (await cdp("Page.captureScreenshot", { format: "png" })).data;
await evaluate(`(() => {
  const target = document.querySelector('canvas');
  for (const type of ['keydown', 'keyup']) target.dispatchEvent(new KeyboardEvent(type, {key: 'F7', code: 'F7', bubbles: true}));
})()`);
await sleep(500);
const after = (await cdp("Page.captureScreenshot", { format: "png" })).data;
if (before === after) throw new Error("F7 did not change the browser presentation");
console.log("CRT browser gate passed");
ws.close();
chrome.kill();
