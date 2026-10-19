// fileTypeFromBuffer (file-type 20.5, as used by Adonis's bodyparser) for
// each base64 buffer on stdin.
import { createRequire } from 'node:module';
import { readFileSync } from 'node:fs';
import { resolve, dirname } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const require = createRequire(resolve(root, 'tests/interop/package.json'));
const { fileTypeFromBuffer } = await import(pathToFileURL(require.resolve('file-type')).href);
const inputs = JSON.parse(readFileSync(0, 'utf8'));
const out = [];
for (const b64 of inputs) out.push((await fileTypeFromBuffer(Buffer.from(b64, 'base64'))) ?? null);
console.log(JSON.stringify(out));
