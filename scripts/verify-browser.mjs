#!/usr/bin/env node
/**
 * Browser check for a generated transcript.
 *
 * Loads the file in headless Chrome and asserts that the discord-transcript-ui
 * stylesheet and script actually took effect, and that nothing regressed:
 *
 *   * the enhancement script upgraded every message;
 *   * no media URI leaked into text (the vertical-garbage regression);
 *   * every inlined data: image decoded;
 *   * the avatar, badge and reaction chrome rendered;
 *   * a spoiler reveals on click;
 *   * nothing overflows horizontally;
 *   * no console errors.
 *
 * Usage: node scripts/verify-browser.mjs [path/to/transcript.html]
 * Requires Chrome (CHROME env var, or google-chrome on PATH) and Node 21+.
 */

import { spawn } from 'node:child_process';
import { existsSync } from 'node:fs';
import { resolve } from 'node:path';
import { pathToFileURL } from 'node:url';

const target = resolve(process.argv[2] ?? 'examples/transcript.html');
if (!existsSync(target)) {
	console.error(`✗ ${target} does not exist (run the CLI first)`);
	process.exit(1);
}

const CHROME = [process.env.CHROME, 'google-chrome', 'google-chrome-stable', 'chromium', 'chromium-browser']
	.filter(Boolean)
	.find((candidate) => (candidate.includes('/') ? existsSync(candidate) : true));

const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

const child = spawn(
	CHROME,
	[
		'--headless=new',
		'--disable-gpu',
		'--no-sandbox',
		'--hide-scrollbars',
		'--disable-dev-shm-usage',
		'--window-size=1000,1200',
		'--remote-debugging-port=0',
		'about:blank'
	],
	{ stdio: ['ignore', 'ignore', 'pipe'] }
);

const browserSocket = await new Promise((resolve, reject) => {
	let buffer = '';
	const timer = setTimeout(() => reject(new Error('timed out waiting for DevTools')), 20000);
	child.stderr.on('data', (chunk) => {
		buffer += chunk;
		const match = buffer.match(/DevTools listening on (ws:\/\/\S+)/);
		if (match) {
			clearTimeout(timer);
			resolve(match[1]);
		}
	});
	child.on('exit', (code) => reject(new Error(`Chrome exited early (${code})`)));
});

const port = new URL(browserSocket).port;
let page;
for (let attempt = 0; attempt < 60; attempt++) {
	const targets = await (await fetch(`http://127.0.0.1:${port}/json/list`)).json();
	page = targets.find((t) => t.type === 'page');
	if (page) break;
	await sleep(100);
}

const socket = new WebSocket(page.webSocketDebuggerUrl);
await new Promise((resolve, reject) => {
	socket.addEventListener('open', resolve, { once: true });
	socket.addEventListener('error', () => reject(new Error('websocket failed')), { once: true });
});

let nextId = 0;
const pending = new Map();
const errors = [];
socket.addEventListener('message', (event) => {
	const message = JSON.parse(event.data);
	if (message.id && pending.has(message.id)) {
		const { resolve, reject } = pending.get(message.id);
		pending.delete(message.id);
		message.error ? reject(new Error(message.error.message)) : resolve(message.result);
		return;
	}
	if (message.method === 'Runtime.exceptionThrown') {
		errors.push(message.params.exceptionDetails.exception?.description ?? message.params.exceptionDetails.text);
	}
	if (message.method === 'Runtime.consoleAPICalled' && message.params.type === 'error') {
		errors.push(message.params.args.map((a) => a.value ?? a.description).join(' '));
	}
});

const send = (method, params = {}) => {
	const id = ++nextId;
	return new Promise((resolve, reject) => {
		pending.set(id, { resolve, reject });
		socket.send(JSON.stringify({ id, method, params }));
	});
};

await send('Page.enable');
await send('Runtime.enable');
await send('Page.navigate', { url: pathToFileURL(target).href });
for (let attempt = 0; attempt < 80; attempt++) {
	try {
		const state = await send('Runtime.evaluate', { expression: 'document.readyState', returnByValue: true });
		if (state.result.value === 'complete') break;
	} catch {
		/* navigating */
	}
	await sleep(75);
}
await sleep(1500);

const result = await send('Runtime.evaluate', {
	returnByValue: true,
	expression: `(() => {
		const checks = [];
		const check = (name, actual, expected) =>
			checks.push({ name, actual: String(actual), expected: String(expected), pass: String(actual) === String(expected) });
		const style = (node, prop, pseudo) => (node ? getComputedStyle(node, pseudo || null).getPropertyValue(prop) : '<missing>');
		const all = (sel) => Array.from(document.querySelectorAll(sel));
		// Transitions interpolate computed values; make them instant so the
		// interaction assertions below are deterministic.
		document.documentElement.style.setProperty('--dt-transition', '0s');

		const messages = all('discord-message');
		check('messages found', messages.length > 0, true);
		check('script upgraded every message', all('discord-message[data-dt-ready]').length, messages.length);
		check('stylesheet applied', style(document.querySelector('discord-messages'), 'background-color') !== 'rgba(0, 0, 0, 0)', true);
		check('avatars rendered', all('.dt-avatar').length, messages.length);
		check('verified bot tag', all('.dt-badge--verified')[0]?.textContent ?? '', 'APP');
		check('reactions rendered', all('discord-reaction').length > 0, true);

		// A media URI must never appear as visible text. Script and style contents
		// are excluded: the config block legitimately carries base64 avatars.
		const leak = all('*')
			.filter((el) => el.tagName !== 'SCRIPT' && el.tagName !== 'STYLE')
			.some((el) => el.children.length === 0 && (el.textContent || '').includes('data:image'));
		check('no media URI leaked into text', leak, false);

		// Inlined images must actually decode.
		const inlined = all('img[src^="data:image"]');
		check('inlined images present', inlined.length > 0, true);
		check('all inlined images decoded', inlined.every((img) => img.complete && img.naturalWidth > 0), true);

		// Interaction.
		const spoiler = document.querySelector('discord-spoiler');
		if (spoiler) {
			const before = style(spoiler, 'background-color');
			spoiler.click();
			check('spoiler reveals on click', style(spoiler, 'background-color') !== before, true);
		}

		check('no horizontal overflow', document.documentElement.scrollWidth <= document.documentElement.clientWidth + 1, true);
		return checks;
	})()`
});

let failed = 0;
console.log(`\n${target}`);
for (const check of result.result.value) {
	if (check.pass) {
		console.log(`  ✓ ${check.name}`);
	} else {
		failed++;
		console.log(`  ✗ ${check.name}\n      expected ${check.expected}, got ${check.actual}`);
	}
}
if (errors.length) {
	failed += errors.length;
	console.log('\nconsole errors');
	for (const error of errors) console.log(`  ✗ ${error}`);
}

console.log(failed === 0 ? '\n✓ browser check passed\n' : `\n✗ ${failed} check(s) failed\n`);
child.kill('SIGKILL');
process.exit(failed === 0 ? 0 : 1);
