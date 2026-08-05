// e2e_crt_filter.mjs - browser GPU smoke gate for the final CRT filter.
// Usage: node e2e_crt_filter.mjs <baseURL>

const [base] = process.argv.slice(2);
if (!base) throw new Error("usage: node e2e_crt_filter.mjs <baseURL>");

const { spawn, spawnSync } = await import("node:child_process");
const { inflateSync } = await import("node:zlib");
const candidates = process.env.IE_CHROME ? [process.env.IE_CHROME] : ["chromium", "google-chrome"];
const chromeBin = candidates.find((candidate) => spawnSync("which", [candidate]).status === 0);
if (!chromeBin) throw new Error("neither chromium nor google-chrome is available; set IE_CHROME to the browser executable");
const chrome = spawn(chromeBin, ["--headless=new", "--no-sandbox", "--remote-debugging-port=9334", "--window-size=960,720", "about:blank"]);
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

function decodePNG(data) {
  const png = Buffer.from(data, "base64");
  if (png.subarray(0, 8).compare(Buffer.from([137, 80, 78, 71, 13, 10, 26, 10])) !== 0) throw new Error("browser did not return a PNG");
  let offset = 8;
  let width, height, bitDepth, colourType;
  const compressed = [];
  while (offset < png.length) {
    const length = png.readUInt32BE(offset);
    const type = png.toString("ascii", offset + 4, offset + 8);
    const chunk = png.subarray(offset + 8, offset + 8 + length);
    offset += length + 12;
    if (type === "IHDR") [width, height, bitDepth, colourType] = [chunk.readUInt32BE(0), chunk.readUInt32BE(4), chunk[8], chunk[9]];
    if (type === "IDAT") compressed.push(chunk);
    if (type === "IEND") break;
  }
  if (bitDepth !== 8 || (colourType !== 2 && colourType !== 6)) throw new Error(`unsupported browser PNG format depth=${bitDepth} colour=${colourType}`);
  const bytesPerPixel = colourType === 6 ? 4 : 3;
  const rowBytes = width * bytesPerPixel;
  const filtered = inflateSync(Buffer.concat(compressed));
  const pixels = Buffer.alloc(rowBytes * height);
  let source = 0;
  for (let y = 0; y < height; y++) {
    const filter = filtered[source++];
    const row = y * rowBytes;
    for (let x = 0; x < rowBytes; x++) {
      const left = x >= bytesPerPixel ? pixels[row + x - bytesPerPixel] : 0;
      const above = y > 0 ? pixels[row + x - rowBytes] : 0;
      const upperLeft = y > 0 && x >= bytesPerPixel ? pixels[row + x - rowBytes - bytesPerPixel] : 0;
      const value = filtered[source++];
      let predictor = 0;
      if (filter === 1) predictor = left;
      else if (filter === 2) predictor = above;
      else if (filter === 3) predictor = Math.floor((left + above) / 2);
      else if (filter === 4) {
        const p = left + above - upperLeft;
        const pa = Math.abs(p - left), pb = Math.abs(p - above), pc = Math.abs(p - upperLeft);
        predictor = pa <= pb && pa <= pc ? left : pb <= pc ? above : upperLeft;
      } else if (filter !== 0) throw new Error(`unsupported PNG filter ${filter}`);
      pixels[row + x] = (value + predictor) & 255;
    }
  }
  return { width, height, bytesPerPixel, pixels };
}

function changedPixelFraction(a, b) {
  if (a.width !== b.width || a.height !== b.height || a.bytesPerPixel !== b.bytesPerPixel) throw new Error("browser screenshot dimensions changed");
  let changed = 0;
  for (let pixel = 0, i = 0; pixel < a.width * a.height; pixel++, i += a.bytesPerPixel) {
    if (Math.abs(a.pixels[i] - b.pixels[i]) > 8 || Math.abs(a.pixels[i + 1] - b.pixels[i + 1]) > 8 || Math.abs(a.pixels[i + 2] - b.pixels[i + 2]) > 8) changed++;
  }
  return changed / (a.width * a.height);
}

async function capturePixels() {
  return decodePNG((await cdp("Page.captureScreenshot", { format: "png" })).data);
}

function requireVisualChange(label, baseline, before, after) {
  const noise = changedPixelFraction(baseline, before);
  const changed = changedPixelFraction(before, after);
  // A changing guest frame establishes the tolerated noise floor. A CRT pass
  // changes the displayed image across far more than the compact status token.
  const minimum = Math.max(0.005, noise * 4 + 0.002);
  if (changed <= minimum) throw new Error(`${label} changed ${(changed * 100).toFixed(2)}% of pixels; need more than ${(minimum * 100).toFixed(2)}% (same-mode noise ${(noise * 100).toFixed(2)}%)`);
}
await cdp("Runtime.enable");
await cdp("Page.enable");
for (let i = 0; i < 300 && !await evaluate("!!window.ieFirstFrame"); i++) await sleep(200);
if (!await evaluate("!!window.ieFirstFrame")) throw new Error("demo did not render a first frame");
async function waitForState(expected) {
  for (let i = 0; i < 100; i++) {
    if (await evaluate("window.ieCRTPresentationState") === expected) return;
    await sleep(50);
  }
  throw new Error(`CRT presentation state did not become ${expected}; got ${await evaluate("window.ieCRTPresentationState")}`);
}
await waitForState("flat-active");
const flatBaseline = await capturePixels();
await sleep(100);
const flatPixels = await capturePixels();
// Ebiten's key state is fed by browser input events, not merely DOM listeners.
// Drive the CDP input pipeline so inpututil.IsKeyJustPressed observes F7.
async function pressF7(expected) {
  await cdp("Input.dispatchKeyEvent", { type: "keyDown", key: "F7", code: "F7", windowsVirtualKeyCode: 118, nativeVirtualKeyCode: 118 });
  await cdp("Input.dispatchKeyEvent", { type: "keyUp", key: "F7", code: "F7", windowsVirtualKeyCode: 118, nativeVirtualKeyCode: 118 });
  await waitForState(expected);
}
await pressF7("curved-active");
const curvedBaseline = await capturePixels();
await sleep(100);
const curvedPixels = await capturePixels();
requireVisualChange("flat to curved CRT", flatBaseline, flatPixels, curvedPixels);
await pressF7("off");
const offBaseline = await capturePixels();
await sleep(100);
const offPixels = await capturePixels();
requireVisualChange("curved CRT to unfiltered output", curvedBaseline, curvedPixels, offPixels);
await pressF7("flat-active");
const finalFlatPixels = await capturePixels();
requireVisualChange("unfiltered output to flat CRT", offBaseline, offPixels, finalFlatPixels);
console.log("CRT browser gate passed");
ws.close();
chrome.kill();
