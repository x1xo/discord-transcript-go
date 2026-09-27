#!/usr/bin/env node
/**
 * Browser check for a generated transcript.
 *
 * Loads the file in headless Chrome and asserts that it renders from the
 * stylesheet alone — the generated markup is complete, so avatars, author rows,
 * badges, timestamps and reactions must appear with no JavaScript at all. If the
 * document happens to include the enhancement script (the `-script` flag), it
 * additionally asserts the script does not rebuild what is already there.
 *
 * Usage: node scripts/verify-browser.mjs [path/to/transcript.html]
 * Requires Chrome (CHROME env var, or google-chrome on PATH) and Node 21+.
 */

import { spawn } from 'node:child_process';
import { existsSync } from 'node:fs';
import { resolve } from 'node:path';
import { pathToFileURL } from 'node:url';

const target = resolve(process.argv[2] ?? 'examples/transcript.html');

// Optional local stylesheet. The generated document links a pinned CDN build;
// if that release is not published yet (or the network is unavailable), pass
// --css ../dist/discord-transcript.min.css and the check swaps it in, so the
// markup is still verified against the real stylesheet.
const cssFlag = process.argv.indexOf('--css');
const localCSS = cssFlag === -1 ? null : resolve(process.argv[cssFlag + 1]);
if (!existsSync(target)) {
	console.error(`✗ ${target} does not exist (run the CLI first)`);
	process.exit(1);
}

const CHROME = [process.env.CHROME, 'google-chrome', 'google-chrome-stable', 'chromium', 'chromium-browser']
	.filter(Boolean)
	.find((candidate) => (candidate.includes('/') ? existsSync(candidate) : true));

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

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
await sleep(1200);

if (localCSS) {
	const probe = await send('Runtime.evaluate', {
		returnByValue: true,
		expression: `(() => {
			const el = document.querySelector('discord-messages, dms');
			return el ? getComputedStyle(el).backgroundColor !== 'rgba(0, 0, 0, 0)' : false;
		})()`
	});
	if (!probe.result.value) {
		await send('Runtime.evaluate', {
			expression: `(() => {
				const link = document.querySelector('link[rel=stylesheet]');
				if (link) {
					// crossorigin makes this a CORS request, which file:// cannot
					// satisfy, so drop it along with the now-wrong integrity hash.
					link.removeAttribute('integrity');
					link.removeAttribute('crossorigin');
					link.href = ${JSON.stringify(pathToFileURL(localCSS).href)};
				}
			})()`
		});
		await sleep(900);
		console.log(`  (the CDN stylesheet did not load; checked against ${localCSS})`);
	}
}

const result = await send('Runtime.evaluate', {
	returnByValue: true,
	expression: `(() => {
		const checks = [];
		const check = (name, actual, expected) =>
			checks.push({ name, actual: String(actual), expected: String(expected), pass: String(actual) === String(expected) });
		const style = (node, prop) => (node ? getComputedStyle(node).getPropertyValue(prop) : '<missing>');
		// The document may use either vocabulary; match both.
		const T = (long, short) => ':is(discord-' + long + ',' + short + ')';
		const all = (sel) => Array.from(document.querySelectorAll(sel));
		document.documentElement.style.setProperty('--dt-transition', '0s');

		const messages = all(T('message','dm'));
		check('messages found', messages.length > 0, true);
		check('stylesheet applied', style(document.querySelector(T('messages','dms')), 'background-color') !== 'rgba(0, 0, 0, 0)', true);

		// The static structure the stylesheet needs must be present already.
		check('every message has its own layout', all('.dt-msg').length, messages.length);
		check('every message has an avatar slot', all('.dt-avatar').length, messages.length);
		check('author rows rendered', all('.dt-header').length > 0, true);
		check('author names rendered', all('.dt-author').every((el) => el.textContent.trim().length > 0), true);
		check('role colour applied', all('.dt-author').some((el) => el.style.color !== ''), true);
		check('timestamps rendered', all('.dt-timestamp').every((el) => el.textContent.trim().length > 0), true);
		check('verified bot tag', all('.dt-badge--verified')[0]?.textContent ?? '', 'APP');
		check('reactions rendered', all(T('reaction','dr')).length > 0, true);
		check('reply rendered', all(T('reply','drp')).length > 0, true);

		// Action rows: a bot's buttons must appear even though a reader cannot
		// press most of them. Label, emoji and disabled state come straight from
		// the markup, and a button with a URL is a real anchor, so it stays
		// clickable with no script at all.
		const rows = all(T('action-row', 'dar'));
		if (rows.length) {
			const buttons = all(T('button', 'dbtn'));
			check('buttons rendered', buttons.length > 0, true);
			check('every button sits in a row', buttons.every((b) => b.closest(T('action-row', 'dar'))), true);
			check('button labels rendered', buttons.some((b) => b.textContent.trim().length > 0), true);
			check('custom button emoji is an image', buttons.some((b) => b.querySelector('img.dt-button-emoji')), true);
			check('disabled button marked', buttons.some((b) => b.hasAttribute('disabled')), true);
			const primaryButton = buttons.find((b) => b.getAttribute('type') === 'primary');
			check('primary button colour', primaryButton ? style(primaryButton, 'background-color') : '', 'rgb(88, 101, 242)');
			const disabledButton = buttons.find((b) => b.hasAttribute('disabled'));
			check('disabled button is dimmed', disabledButton ? parseFloat(style(disabledButton, 'opacity')) : 1, 0.5);
			const linkButton = document.querySelector('a.dt-button-link');
			check('link button is a real anchor', !!(linkButton && /^https?:/.test(linkButton.href)), true);
			check('link button anchor adds no underline', linkButton ? style(linkButton, 'text-decoration-line') : '', 'none');
		}

		// Headers: markdown headings render, and the channel header comes from the
		// channel-name attribute via the stylesheet.
		const headings = all(T('header','dh'));
		check('markdown headings rendered', headings.length > 0, true);
		check('heading levels set', headings.every((h) => h.getAttribute('level')), true);
		check('level 1 heading is larger', parseFloat(getComputedStyle(headings[0]).fontSize) > 16, true);
		const channelHeader = getComputedStyle(document.querySelector(T('messages','dms')), '::before').content;
		check('channel header rendered', channelHeader.includes('general'), true);

		// Embeds: the accent colour must reach the border, the footer must sit below
		// the thumbnail rather than beside it, and a description must keep its line
		// breaks. Each of these was broken once.
		const embed = document.querySelector(T('embed', 'de'));
		if (embed) {
			check('embed accent colour applied', getComputedStyle(embed).borderLeftColor, 'rgb(88, 101, 242)');
			check('embed accent is 4px', getComputedStyle(embed).borderLeftWidth, '4px');
			const thumbnail = embed.querySelector('.dt-embed-thumbnail, [slot="thumbnail"]');
			const footer = embed.querySelector(T('embed-footer', 'defo'));
			if (thumbnail && footer) {
				const thumbBox = thumbnail.getBoundingClientRect();
				const footerBox = footer.getBoundingClientRect();
				check('embed footer clears the thumbnail', footerBox.top >= thumbBox.bottom - 1, true);
				check('embed footer spans the row', footerBox.width > thumbBox.width * 1.5, true);
			}
			const description = embed.querySelector(T('embed-description', 'ded'));
			if (description) {
				check('description keeps its line breaks', description.innerHTML.includes('<br>'), true);
			}
		}

		// Width: an embed is a block box, so it used to stretch to the whole
		// 516px column even when it held one short line. It must now hug its
		// content, and still stop at Discord's 516px cap when the text is long.
		check('embeds exist to measure', all(T('embed', 'de')).length > 0, true);
		check('no embed exceeds the 516px cap', all(T('embed', 'de')).every((node) => node.getBoundingClientRect().width <= 516), true);
		const longEmbed = all(T('embed', 'de')).find((node) => node.querySelector(T('pre', 'dp')));
		if (longEmbed) {
			const longWidth = longEmbed.getBoundingClientRect().width;
			check('a long embed stops at the 516px cap', longWidth > 500 && longWidth <= 516, true);
		}
		const shortEmbed = all(T('embed', 'de')).find((node) => {
			if (node.querySelector(T('embed-field', 'def') + ', .dt-embed-thumbnail, .dt-embed-image, [slot="thumbnail"], [slot="image"]')) return false;
			const text = node.querySelector(T('embed-description', 'ded'));
			const length = text ? text.textContent.trim().length : 0;
			return length > 0 && length < 60;
		});
		if (shortEmbed) {
			const shortWidth = Math.round(shortEmbed.getBoundingClientRect().width);
			check('a short embed hugs its content', shortWidth > 120 && shortWidth < 400, true);
			check('a short embed is narrower than the cap', shortWidth < 516, true);
		}

		// Embed text is a document of its own. A description or a field value may
		// carry a heading, a list, a quote and a fenced block, and each has to
		// render as the block it is instead of collapsing into one flat line.
		const styledDescription = all(T('embed-description', 'ded')).find((node) => node.querySelector(T('header', 'dh')));
		if (styledDescription) {
			const styledHeader = styledDescription.querySelector(T('header', 'dh'));
			check('embed description heading keeps its level', styledHeader.getAttribute('level'), '1');
			check(
				'embed description heading is larger than its text',
				parseFloat(style(styledHeader, 'font-size')) > parseFloat(style(styledDescription, 'font-size')),
				true
			);
			const styledList = styledDescription.querySelector(T('unordered-list', 'dul'));
			check(
				'embed description list keeps its items',
				styledList ? styledList.querySelectorAll(T('list-item', 'dli')).length : 0,
				2
			);
			check('embed description list draws bullets', styledList ? style(styledList.children[0], 'list-style-type') : '', 'disc');
			check(
				'embed description quote keeps its bar',
				parseFloat(style(styledDescription.querySelector(T('quote', 'dq')), 'border-left-width')) > 0,
				true
			);
			const styledCode = styledDescription.querySelector(T('pre', 'dp'));
			check('embed description code block keeps its text', !!styledCode && styledCode.textContent.includes('adapter.Transcript'), true);
			check('embed description code block keeps its newlines', styledCode ? style(styledCode.querySelector(T('code', 'dc')), 'white-space') : '', 'pre');
			check('embed description keeps its small print', styledDescription.querySelector(T('subscript', 'dsub')) !== null, true);
		}
		const styledField = all(T('embed-field', 'def')).find((node) => node.querySelector(T('pre', 'dp')));
		if (styledField) {
			check('embed field code block keeps its text', styledField.textContent.includes('panic: assignment to entry in nil map'), true);
		}

		// Timestamps: message headers show the short date and time for anything
		// older than yesterday ("3/15/24, 2:28 PM"). The compact form
		// ("11:49PM") and the "Yesterday at …" prefix only appear for recent
		// messages, which the Go unit tests cover with dates relative to now.
		const headerStamp = document.querySelector('.dt-timestamp');
		check('message header timestamp', headerStamp?.textContent ?? '', '3/15/24, 2:28 PM');
		const inlineStamp = document.querySelector(T('time', 'dti'));
		if (inlineStamp && !window.DiscordTranscript) {
			// Without the script the renderer's own fallback stands: the same
			// short date and time shape.
			check('inline timestamp', inlineStamp.textContent, '3/15/24, 2:28 PM');
		}

		// Small print (#- subtext) must be the subtle grey at 14px, not body text.
		const small = all(T('subscript','dsub'))[0];
		check('small print rendered', small !== undefined, true);
		if (small) {
			const smallStyle = getComputedStyle(small);
			check('small print is 14px', smallStyle.fontSize, '14px');
			// --dt-text-subtle in the dark theme; the light theme uses #595a63.
			check('small print is the subtle grey', smallStyle.color, 'rgb(197, 198, 202)');
			check('small print is not body coloured', smallStyle.color !== getComputedStyle(document.querySelector('.dt-author')).color, true);
		}

		// Media: inlined images must decode, no URI may appear as text, and a
		// blob used more than once must be stored once. The last one is the whole
		// point of the media pool: an avatar repeated in every message used to be
		// copied in full every time.
		const inlined = all('img[src^="data:image"]');
		const pooled = all('.dt-media');
		check('inlined images present', inlined.length + pooled.length > 0, true);
		check('all inlined images decoded', inlined.every((img) => img.complete && img.naturalWidth > 0), true);
		const leak = all('*')
			.filter((el) => el.tagName !== 'SCRIPT' && el.tagName !== 'STYLE')
			.some((el) => el.children.length === 0 && (el.textContent || '').includes('data:image'));
		check('no media URI leaked into text', leak, false);

		const poolStyle = document.querySelector('style[data-dt-media-pool]');
		const pooledBlobs = poolStyle
			? poolStyle.textContent
					.split('background-image:url("')
					.slice(1)
					.map((part) => part.slice(0, part.indexOf('"')))
			: [];
		// Each hoisted blob is stored once in the pool, and several uses share it.
		// A content image that happens to be the same bytes keeps its own <img>,
		// which is why this counts the pool and not the whole document.
		check('pooled blobs are stored once each', new Set(pooledBlobs).size, pooledBlobs.length);
		check('pooled media shares fewer blobs than uses', pooled.length > pooledBlobs.length, true);
		if (pooled.length > 0) {
			check(
				'pooled media draws its image',
				pooled.every((el) => getComputedStyle(el).backgroundImage.startsWith('url("data:image')),
				true
			);
			check('pooled media crops like an image', getComputedStyle(pooled[0]).backgroundSize, 'cover');
			const pooledAvatar = document.querySelector('.dt-avatar > .dt-media');
			if (pooledAvatar) {
				const box = pooledAvatar.getBoundingClientRect();
				const parent = pooledAvatar.parentElement.getBoundingClientRect();
				check('pooled avatar fills its box', Math.abs(box.width - parent.width) < 1 && Math.abs(box.height - parent.height) < 1, true);
			}
			check('pool marker never reaches the page', document.documentElement.outerHTML.includes(' data-dt-media="'), false);
		}

		// Without the script a spoiler is hidden but peekable on hover; with it,
		// a click reveals it.
		const spoiler = document.querySelector(T('spoiler','dsp'));
		if (spoiler) {
			const hidden = style(spoiler, 'background-color');
			check('spoiler starts hidden', style(spoiler, 'color') === 'rgba(0, 0, 0, 0)', true);
			if (window.DiscordTranscript) {
				spoiler.click();
				check('script: spoiler reveals on click', style(spoiler, 'background-color') !== hidden, true);
			}
		}

		// A grouped line has no avatar of its own. The slot stays for the columns,
		// but it must not keep an avatar's height: an invisible 32px box used to
		// hold every continuation row open, spacing grouped lines apart.
		const grouped = document.querySelector(T('message', 'dm') + '[data-dt-continuation]');
		if (grouped) {
			const groupedAvatar = grouped.querySelector('.dt-avatar');
			check(
				'continuation avatar takes no height',
				groupedAvatar ? Math.round(groupedAvatar.getBoundingClientRect().height) : -1,
				0
			);
			const columns = grouped.querySelector('.dt-content');
			if (groupedAvatar && columns) {
				// The content still starts where the avatars end, so the columns line up.
				check('continuation keeps its column', Math.round(columns.getBoundingClientRect().left) > Math.round(grouped.getBoundingClientRect().left), true);
			}
		}

		// When the script is included it must recognise the finished markup
		// instead of duplicating it.
		if (window.DiscordTranscript) {
			check('script: no duplicate avatars', all('.dt-avatar').length, messages.length);
			check('script: no duplicate bodies', all('.dt-body').length, messages.length);
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
