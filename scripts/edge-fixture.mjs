// Renders the reset password email with Edge 6, as web-be's
// `.htmlView('emails/reset_password', { resetUrl })` does. Reads the URL from stdin.
import { createRequire } from 'node:module';
import { readFileSync } from 'node:fs';
import { resolve, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const require = createRequire(resolve(root, 'tests/interop/package.json'));
const { Edge } = await import(require.resolve('edge.js'));
const edge = Edge.create();
edge.registerTemplate('emails/reset_password', { template: readFileSync(resolve(root, 'internal/mail/templates/reset_password.html'), 'utf8') });
process.stdout.write(await edge.render('emails/reset_password', { resetUrl: readFileSync(0, 'utf8') }));
