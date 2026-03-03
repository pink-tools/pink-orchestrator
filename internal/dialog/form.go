package dialog

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
)

// ShowForm spawns a subprocess to render a FormSpec JSON as a WebView form.
// Subprocess is needed because macOS requires NSWindow on the main thread,
// and systray already owns it.
// Returns (values, true) on save, (nil, false) on cancel/error.
func ShowForm(specJSON []byte) (map[string]any, bool) {
	exe, err := os.Executable()
	if err != nil {
		return nil, false
	}

	cmd := exec.Command(exe, "--dialog")
	cmd.Stdin = bytes.NewReader(specJSON)
	cmd.Stderr = os.Stderr
	output, err := cmd.Output()
	if err != nil {
		return nil, false
	}

	output = bytes.TrimSpace(output)
	if len(output) == 0 {
		return nil, false
	}

	var values map[string]any
	if err := json.Unmarshal(output, &values); err != nil {
		return nil, false
	}

	return values, true
}

func formHTML(specJSON []byte) string {
	return `<!DOCTYPE html>
<html>
<head>
<meta charset="UTF-8">
<style>
* { margin: 0; padding: 0; box-sizing: border-box; }
body {
	font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
	background: #1a1a2e;
	color: #eee;
	padding: 24px;
	display: flex;
	flex-direction: column;
	height: 100vh;
}
h1 {
	font-size: 18px;
	font-weight: 600;
	margin-bottom: 16px;
	color: #fff;
}
.fields {
	flex: 1;
	overflow-y: auto;
	padding-right: 8px;
}
.field {
	margin-bottom: 14px;
}
.field label {
	display: block;
	font-size: 13px;
	font-weight: 500;
	color: #aaa;
	margin-bottom: 4px;
}
.field .desc {
	font-size: 11px;
	color: #666;
	margin-bottom: 4px;
}
input[type="text"],
input[type="password"],
input[type="number"],
select {
	width: 100%;
	padding: 8px 10px;
	background: #16213e;
	border: 1px solid #0f3460;
	border-radius: 4px;
	color: #eee;
	font-size: 14px;
	outline: none;
}
input:focus, select:focus {
	border-color: #e94560;
}
.range-wrap {
	display: flex;
	align-items: center;
	gap: 10px;
}
.range-wrap input[type="range"] {
	flex: 1;
	accent-color: #e94560;
}
.range-wrap span {
	min-width: 36px;
	text-align: right;
	font-size: 13px;
	color: #ccc;
}
.check-wrap {
	display: flex;
	align-items: center;
	gap: 8px;
}
.check-wrap input[type="checkbox"] {
	accent-color: #e94560;
	width: 16px;
	height: 16px;
}
.check-wrap span {
	font-size: 14px;
}
.sound-wrap select {
	margin-bottom: 6px;
}
.sound-wrap .custom-input {
	display: none;
}
.sound-wrap .custom-input.visible {
	display: block;
}
.buttons {
	display: flex;
	gap: 12px;
	margin-top: 20px;
	justify-content: flex-end;
}
button {
	padding: 10px 20px;
	border: none;
	border-radius: 6px;
	font-size: 14px;
	font-weight: 500;
	cursor: pointer;
	transition: opacity 0.2s;
}
button:hover { opacity: 0.9; }
.save { background: #e94560; color: white; }
.cancel { background: #0f3460; color: white; }
</style>
</head>
<body>
<h1 id="title"></h1>
<div class="fields" id="fields"></div>
<div class="buttons">
	<button class="save" onclick="save()">Save</button>
	<button class="cancel" onclick="onCancel()">Cancel</button>
</div>
<script>
const spec = ` + string(specJSON) + `;

document.getElementById('title').textContent = spec.title || 'Settings';

const container = document.getElementById('fields');
const fields = spec.fields || [];

fields.forEach(f => {
	const div = document.createElement('div');
	div.className = 'field';

	const label = document.createElement('label');
	label.textContent = f.label || f.name;
	div.appendChild(label);

	if (f.desc) {
		const desc = document.createElement('div');
		desc.className = 'desc';
		desc.textContent = f.desc;
		div.appendChild(desc);
	}

	const type = f.type || 'text';
	const val = f.current !== undefined ? f.current : (f.value !== undefined ? f.value : (f.default !== undefined ? f.default : ''));

	function optLabel(o) {
		if (o.label) return o.label;
		const s = String(o.value !== undefined ? o.value : o);
		const parts = s.split('/');
		const name = parts[parts.length - 1];
		const dot = name.lastIndexOf('.');
		return dot > 0 ? name.substring(0, dot) : name;
	}

	if (type === 'select') {
		const sel = document.createElement('select');
		sel.dataset.name = f.name;
		(f.options || []).forEach(o => {
			const opt = document.createElement('option');
			opt.value = o.value !== undefined ? o.value : o;
			opt.textContent = optLabel(o);
			if (String(opt.value) === String(val)) opt.selected = true;
			sel.appendChild(opt);
		});
		div.appendChild(sel);
	} else if (type === 'sound') {
		const wrap = document.createElement('div');
		wrap.className = 'sound-wrap';
		const sel = document.createElement('select');
		sel.dataset.name = f.name;
		sel.dataset.type = 'sound';
		(f.options || []).forEach(o => {
			const opt = document.createElement('option');
			opt.value = o.value !== undefined ? o.value : o;
			opt.textContent = optLabel(o);
			if (String(opt.value) === String(val)) opt.selected = true;
			sel.appendChild(opt);
		});
		const customOpt = document.createElement('option');
		customOpt.value = '__custom__';
		customOpt.textContent = 'Custom path...';
		sel.appendChild(customOpt);

		const customInput = document.createElement('input');
		customInput.type = 'text';
		customInput.className = 'custom-input';
		customInput.placeholder = '/path/to/sound.wav';
		customInput.dataset.name = f.name + '__custom';

		// Check if current value is not in options
		const optValues = (f.options || []).map(o => String(o.value !== undefined ? o.value : o));
		if (val && !optValues.includes(String(val))) {
			sel.value = '__custom__';
			customInput.value = val;
			customInput.classList.add('visible');
		}

		sel.addEventListener('change', () => {
			if (sel.value === '__custom__') {
				customInput.classList.add('visible');
			} else {
				customInput.classList.remove('visible');
			}
		});

		wrap.appendChild(sel);
		wrap.appendChild(customInput);
		div.appendChild(wrap);
	} else if (type === 'range') {
		const wrap = document.createElement('div');
		wrap.className = 'range-wrap';
		const input = document.createElement('input');
		input.type = 'range';
		input.dataset.name = f.name;
		input.min = f.min !== undefined ? f.min : 0;
		input.max = f.max !== undefined ? f.max : 100;
		input.step = f.step !== undefined ? f.step : 1;
		input.value = val;
		const span = document.createElement('span');
		span.textContent = input.value;
		input.addEventListener('input', () => { span.textContent = input.value; });
		wrap.appendChild(input);
		wrap.appendChild(span);
		div.appendChild(wrap);
	} else if (type === 'confirm') {
		const wrap = document.createElement('div');
		wrap.className = 'check-wrap';
		const cb = document.createElement('input');
		cb.type = 'checkbox';
		cb.dataset.name = f.name;
		cb.checked = !!val;
		const span = document.createElement('span');
		span.textContent = f.label || f.name;
		wrap.appendChild(cb);
		wrap.appendChild(span);
		div.appendChild(wrap);
		// Hide the label above since it's in the checkbox row
		label.style.display = 'none';
	} else if (type === 'number') {
		const input = document.createElement('input');
		input.type = 'number';
		input.dataset.name = f.name;
		if (f.min !== undefined) input.min = f.min;
		if (f.max !== undefined) input.max = f.max;
		if (f.step !== undefined) input.step = f.step;
		input.value = val;
		div.appendChild(input);
	} else if (type === 'password') {
		const input = document.createElement('input');
		input.type = 'password';
		input.dataset.name = f.name;
		input.value = val;
		div.appendChild(input);
	} else {
		// text, url, hotkey, file
		const input = document.createElement('input');
		input.type = 'text';
		input.dataset.name = f.name;
		input.value = val;
		div.appendChild(input);
	}

	container.appendChild(div);
});

function save() {
	const result = {};
	fields.forEach(f => {
		const type = f.type || 'text';
		if (type === 'confirm') {
			const el = document.querySelector('[data-name="' + f.name + '"]');
			result[f.name] = el ? el.checked : false;
		} else if (type === 'sound') {
			const sel = document.querySelector('select[data-name="' + f.name + '"]');
			if (sel && sel.value === '__custom__') {
				const ci = document.querySelector('[data-name="' + f.name + '__custom"]');
				result[f.name] = ci ? ci.value : '';
			} else {
				result[f.name] = sel ? sel.value : '';
			}
		} else if (type === 'range' || type === 'number') {
			const el = document.querySelector('[data-name="' + f.name + '"]');
			result[f.name] = el ? Number(el.value) : 0;
		} else {
			const el = document.querySelector('[data-name="' + f.name + '"]');
			result[f.name] = el ? el.value : '';
		}
	});
	onSave(JSON.stringify(result));
}
</script>
</body>
</html>`
}
