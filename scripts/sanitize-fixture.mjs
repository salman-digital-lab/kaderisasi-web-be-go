// Sanitizes each input string from stdin (a JSON array) with web-be's
// rich_text_service options, for the Go interop tests.
import { createRequire } from 'node:module';
import { readFileSync } from 'node:fs';
import { resolve, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';

const require = createRequire(resolve(dirname(fileURLToPath(import.meta.url)), '../tests/interop/package.json'));
const sanitizeHtml = require('sanitize-html');
// Copied from kaderisasi-web-be/app/services/rich_text_service.ts.
const RICH_TEXT_OPTIONS = {
  allowedTags: ['p', 'br', 'strong', 'b', 'em', 'i', 'u', 's', 'h1', 'h2', 'h3', 'h4', 'h5', 'h6', 'ul', 'ol', 'li', 'blockquote', 'pre', 'code', 'a', 'hr'],
  allowedAttributes: { a: ['href', 'title'], p: ['style'], h1: ['style'], h2: ['style'], h3: ['style'], h4: ['style'], h5: ['style'], h6: ['style'] },
  allowedStyles: { '*': { 'text-align': [/^(?:left|center|right|justify)$/] } },
  allowedSchemes: ['http', 'https', 'mailto'],
  allowedSchemesAppliedToAttributes: ['href'],
  allowProtocolRelative: false,
  nestingLimit: 20,
  disallowedTagsMode: 'discard',
};
const inputs = JSON.parse(readFileSync(0, 'utf8'));
console.log(JSON.stringify(inputs.map((html) => sanitizeHtml(html, RICH_TEXT_OPTIONS))));
