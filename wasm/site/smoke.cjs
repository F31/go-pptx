// TOOL-02 manual smoke test: run pptx_check.wasm in node and call each method.
const fs = require('fs');
const path = require('path');

globalThis.performance = globalThis.performance || { now: () => Date.now() };
require(__dirname + '/wasm_exec.js');
const wasmBytes = fs.readFileSync(__dirname + '/pptx_check.wasm');

const logFile = path.resolve(__dirname, '../../_smoke_out.txt');
fs.writeFileSync(logFile, 'start\n');

const go = new Go();
go.exit = (code) => {
	fs.appendFileSync(logFile, 'go.exit=' + code + '\n');
	process.exit(code);
};

async function main() {
	const res = await WebAssembly.instantiate(wasmBytes, go.importObject);
	fs.appendFileSync(logFile, 'instantiated\n');
	const runPromise = go.run(res.instance);
	fs.appendFileSync(logFile, 'run started\n');

	const ctx = globalThis.GoPptxCheck;
	if (!ctx) throw new Error('GoPptxCheck namespace missing after run');

	const fns = Object.keys(ctx).filter((k) => typeof ctx[k] === 'function');
	fs.appendFileSync(logFile, 'exposed: ' + fns.join(',') + '\n');

	fs.appendFileSync(logFile, 'schemeVersion: ' + ctx.schemeVersion() + '\n');

	const capRes = JSON.parse(ctx.capability('demo.pptx'));
	fs.appendFileSync(logFile, 'capability.ok=' + capRes.ok + ' dims=' +
		Object.keys((capRes.manifest && capRes.manifest.dimensions) || {}).length + '\n');

	const fakeBytes = new Uint8Array([0x50, 0x4b, 0x05, 0x06]);
	const inspectRes = JSON.parse(ctx.inspect(fakeBytes, 'fake.pptx'));
	fs.appendFileSync(logFile, 'inspect.ok=' + inspectRes.ok + ' error=' + (inspectRes.error || 'none').slice(0, 80) + '\n');

	const valRes = JSON.parse(ctx.validate(fakeBytes, 'fake.pptx'));
	fs.appendFileSync(logFile, 'validate.ok=' + valRes.ok + ' error=' + (valRes.error || 'none').slice(0, 80) + '\n');

	fs.appendFileSync(logFile, 'TOOL-02 manual smoke: OK\n');

	// Trigger go exit via syscall (not standard JS).
	// Direct call to process.exit might not interrupt Go's event loop, but the
	// tests have proven the methods work — close cleanly here.
	process.exit(0);
}

main().catch((err) => {
	fs.appendFileSync(logFile, 'FAIL: ' + (err && err.stack ? err.stack : err) + '\n');
	process.exit(2);
});
