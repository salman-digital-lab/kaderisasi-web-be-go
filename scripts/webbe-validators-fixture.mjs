// Runs kaderisasi-web-be's own validator modules (app/validators/*.ts) on the
// cases from stdin: [{ validator: "file.exportName", data }]. Start it with
// cwd = ../kaderisasi-web-be and `node --import ts-node-maintained/register/esm`.
import { readFileSync } from 'node:fs';
import { pathToFileURL } from 'node:url';
import { resolve } from 'node:path';

// vine.file() is registered by @adonisjs/core's VineJS provider at boot.
const { Vine } = await import(resolve(process.cwd(), 'node_modules/@vinejs/vine/build/index.js'));
const { VineMultipartFile } = await import(resolve(process.cwd(), 'node_modules/@adonisjs/core/build/src/vine.js'));
Vine.macro('file', (options) => new VineMultipartFile(options));
const cases = JSON.parse(readFileSync(0, 'utf8'));
const modules = {};
const { errors } = await import(resolve(process.cwd(), 'node_modules/@vinejs/vine/build/index.js'));
const out = [];
for (const { validator, data } of cases) {
  const [file, name] = validator.split('.');
  modules[file] ??= await import(pathToFileURL(resolve(process.cwd(), `app/validators/${file}.ts`)).href);
  try {
    out.push({ value: await modules[file][name].validate(data) });
  } catch (error) {
    if (!(error instanceof errors.E_VALIDATION_ERROR)) throw error;
    out.push({ issues: error.messages });
  }
}
console.log(JSON.stringify(out));
