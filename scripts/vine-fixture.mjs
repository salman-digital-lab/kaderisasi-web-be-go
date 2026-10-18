// Runs web-be's Vine validators (app/validators/auth_validator.ts) and
// validator.js isEmail on inputs from stdin, for the Go interop tests.
import { createRequire } from 'node:module';
import { readFileSync } from 'node:fs';
import { resolve, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';

const require = createRequire(resolve(dirname(fileURLToPath(import.meta.url)), '../tests/interop/package.json'));
const { default: vine, errors } = await import(require.resolve('@vinejs/vine'));
const isEmail = require('validator/lib/isEmail.js').default;

const email = () => vine.string().trim().toLowerCase().email();
const schemas = {
  google: vine.compile(vine.object({ code: vine.string().minLength(1).maxLength(4096), codeVerifier: vine.string().minLength(43).maxLength(128), nonce: vine.string().minLength(32).maxLength(128) })),
  register: vine.compile(vine.object({ fullname: vine.string(), email: email(), password: vine.string() })),
  login: vine.compile(vine.object({ email: email(), password: vine.string() })),
  checkEmail: vine.compile(vine.object({ email: email() })),
  resetPassword: vine.compile(vine.object({ password: vine.string() })),
};
const input = JSON.parse(readFileSync(0, 'utf8'));
const cases = [];
for (const { schema, data } of input.cases) {
  try { cases.push({ value: await schemas[schema].validate(data) }); }
  catch (error) { if (!(error instanceof errors.E_VALIDATION_ERROR)) throw error; cases.push({ message: error.message, issues: error.messages }); }
}
console.log(JSON.stringify({ cases, emails: input.emails.map(value => isEmail(value)) }));
