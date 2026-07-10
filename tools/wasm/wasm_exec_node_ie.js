// wasm_exec_node_ie.js - node runner for IntuitionEngine js/wasm tests.
//
// Identical to the toolchain's wasm_exec_node.js with one addition: it
// exposes the Go module's exported linear memory as globalThis.__goMem
// before go.run, which the IE64 wasm JIT runtime imports into every
// generated block module (env.mem). The demo page does the same in the
// browser.
"use strict";

if (process.argv.length < 3) {
	console.error("usage: go_js_wasm_exec [wasm binary] [arguments]");
	process.exit(1);
}

globalThis.require = require;
globalThis.fs = require("fs");
globalThis.path = require("path");
globalThis.TextEncoder = require("util").TextEncoder;
globalThis.TextDecoder = require("util").TextDecoder;

globalThis.performance ??= require("perf_hooks").performance;

globalThis.crypto ??= require("crypto");

require(process.env.IE_WASM_EXEC_JS);

const go = new Go();
go.argv = process.argv.slice(2);
go.env = Object.assign({ TMPDIR: require("os").tmpdir() }, process.env);
go.exit = process.exit;
WebAssembly.instantiate(fs.readFileSync(process.argv[2]), go.importObject).then((result) => {
	globalThis.__goMem = result.instance.exports.mem;
	process.on("exit", (code) => {
		if (code === 0 && !go.exited) {
			go._pendingEvent = { id: 0 };
			go._resume();
		}
	});
	return go.run(result.instance);
}).catch((err) => {
	console.error(err);
	process.exit(1);
});
