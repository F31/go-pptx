// check.js —— TOOL-02 浏览器端检查工具的 JS 胶水。
//
// 加载 wasm_exec.js 后通过 fetch("pptx_check.wasm") + WebAssembly.instantiateStreaming
// 启动 Go runtime，运行时把 GoPptxCheck.* 注入 window。下面 handle* 把
// 文件输入（仅 FileReader，不上传）转成 Uint8Array 并交付 wasm 处理。
//
// 完全离线：无 fetch 触发到外网、无 XHR；fetch 同源是 file:///wasm/site/ 下的
// pptx_check.wasm。wasm_exec.js 由 Go 官方 runtime 提供（路径 /d/Go/lib/wasm/
// wasm_exec.js，本仓库 wasm/site/build.ps1 一并拷贝）。
(function () {
	'use strict';

	const $ = (id) => document.getElementById(id);

	const fileInput = $('fileInput');
	const fileMeta = $('fileMeta');
	const inspectBtn = $('inspectBtn');
	const validateBtn = $('validateBtn');
	const capabilityBtn = $('capabilityBtn');
	const clearBtn = $('clearBtn');
	const resultCard = $('result');
	const resultMeta = $('resultMeta');
	const resultJSON = $('resultJSON');
	const errorCard = $('errorCard');
	const errorText = $('errorText');
	const sdkVersionEl = $('sdkVersion');
	const schemaVersionEl = $('schemaVersion');

	let goPptx = null;       // window.GoPptxCheck 命名空间
	let currentFile = null;  // File 句柄（不缓存字节）

	function sizeOf(bytes) {
		if (bytes < 1024) return `${bytes} B`;
		if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KiB`;
		return `${(bytes / 1024 / 1024).toFixed(2)} MiB`;
	}

	function displaySize(elId, url) {
		const el = $(elId);
		if (!el) return;
		fetch(url, { method: 'HEAD' }).then(r => {
			const n = Number(r.headers.get('content-length') || 0);
			el.textContent = n ? sizeOf(n) : 'unknown';
		}).catch(() => { el.textContent = 'unavailable'; });
	}

	function showError(msg) {
		errorCard.style.display = '';
		errorText.textContent = msg;
		resultCard.style.display = 'none';
	}

	function showResult(label, obj) {
		errorCard.style.display = 'none';
		resultCard.style.display = '';
		const ts = new Date().toISOString();
		resultMeta.innerHTML = `${label} · <code>${ts}</code>` +
			(obj && obj.error ? ` · <span class="error">错误：${escapeHtml(obj.error)}</span>` : '');
		let body = obj;
		if (obj && obj.manifestJson) {
			// capability：优先用 SDK 已解析的视图 + 维度小卡片
			body = obj.manifestJson;
			const dimHtml = renderDims(obj.manifestJson.dimensions || {});
			resultMeta.innerHTML += `<div class="dim-grid" style="margin-top:12px">${dimHtml}</div>`;
		}
		resultJSON.textContent = JSON.stringify(body, null, 2);
	}

	function escapeHtml(s) {
		return String(s).replace(/[&<>"']/g, c =>
			({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
	}

	function renderDims(dims) {
		const keys = ['inspect', 'create', 'edit', 'preserve', 'render', 'play'];
		const out = [];
		for (const k of keys) {
			const d = dims[k];
			if (!d) continue;
			out.push(`<div class="dim-key">${k}</div>` +
				`<div class="dim-status"><span class="tag tag-${d.status}">${d.status}</span></div>` +
				`<div>${escapeHtml(d.notes || '')}</div>`);
		}
		return out.join('');
	}

	async function readFile(file) {
		const buf = await file.arrayBuffer();
		return new Uint8Array(buf);
	}

	async function runOp(opName, label) {
		if (!goPptx) return showError('WASM 尚未加载完成');
		try {
			let raw;
			if (opName === 'capability') {
				raw = goPptx.capability(currentFile ? currentFile.name : '');
			} else {
				if (!currentFile) return showError('请选择 PPTX 文件');
				const bytes = await readFile(currentFile);
				if (opName === 'inspect') {
					raw = goPptx.inspect(bytes, currentFile.name);
				} else if (opName === 'validate') {
					raw = goPptx.validate(bytes, currentFile.name);
				}
			}
			// 兼容返回：字符串 JSON（推荐）+ 偶尔的对象
			let jsonStr;
			if (typeof raw === 'string') {
				jsonStr = raw;
			} else if (raw && typeof raw === 'object') {
				// 旧版 object 形态回退
				jsonStr = raw.json || raw.result || JSON.stringify(raw);
			} else {
				jsonStr = String(raw);
			}
			let parsed = null;
			try { parsed = JSON.parse(jsonStr); } catch (e) {
				return showError('WASM 返回非 JSON：' + jsonStr.slice(0, 200));
			}
			showResult(label, parsed);
		} catch (e) {
			showError(`${label} 失败：${e && e.message ? e.message : e}`);
		}
	}

	function updateButtons() {
		const has = !!currentFile;
		inspectBtn.disabled = !goPptx || !has;
		validateBtn.disabled = !goPptx || !has;
		capabilityBtn.disabled = !goPptx;
	}

	function attachFile(file) {
		currentFile = file;
		fileMeta.innerHTML = `文件：<code>${escapeHtml(file.name)}</code> · ${sizeOf(file.size)} · ${escapeHtml(file.type || 'unknown')}`;
		updateButtons();
	}

	fileInput.addEventListener('change', (e) => {
		const f = e.target.files && e.target.files[0];
		if (f) attachFile(f);
	});

	inspectBtn.addEventListener('click', () => runOp('inspect', 'Inspect (IR 投影)'));
	validateBtn.addEventListener('click', () => runOp('validate', 'Validate (L0 结构校验)'));
	capabilityBtn.addEventListener('click', () => runOp('capability', 'Capability (六维能力报告)'));
	clearBtn.addEventListener('click', () => {
		currentFile = null;
		fileInput.value = '';
		fileMeta.textContent = '';
		resultCard.style.display = 'none';
		errorCard.style.display = 'none';
		updateButtons();
	});

	// 启动 Go WASM。
	const go = new Go();
	WebAssembly.instantiateStreaming(fetch('pptx_check.wasm'), go.importObject).then((res) => {
		go.run(res.instance);
		goPptx = window.GoPptxCheck;
		sdkVersionEl.textContent = goPptx && typeof goPptx === 'object' ? 'ok' : 'not-ready';
		try {
			schemaVersionEl.textContent = goPptx.schemeVersion();
			sdkVersionEl.textContent = 'loaded';
		} catch (e) {
			schemaVersionEl.textContent = 'unavailable';
		}
		updateButtons();
	}).catch((err) => {
		showError('WASM 加载失败：' + err + ' — 请确认通过 HTTP 服务（file:// 协议可能拒绝 fetch）');
	});

	displaySize('wasmSize', 'pptx_check.wasm');
	displaySize('execSize', 'wasm_exec.js');
})();
