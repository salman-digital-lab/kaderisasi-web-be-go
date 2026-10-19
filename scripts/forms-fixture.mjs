// Runs web-be's validateCustomFormSubmission and formRoute
// (app/services/custom_form_submission_service.ts, form_routing.ts) on the
// cases from stdin. Start with cwd = ../kaderisasi-web-be and
// `node --import ts-node-maintained/register/esm`.
import { readFileSync } from 'node:fs';
import { pathToFileURL } from 'node:url';
import { resolve } from 'node:path';

const load = (file) => import(pathToFileURL(resolve(process.cwd(), file)).href);
const { validateCustomFormSubmission } = await load('app/services/custom_form_submission_service.ts');
const { formRoute, pruneFormAnswers } = await load('app/services/form_routing.ts');
const cases = JSON.parse(readFileSync(0, 'utf8'));
const run = (fn) => { try { return { value: fn() }; } catch (error) { return { error: error.message }; } };
console.log(JSON.stringify(cases.map(({ schema, submission }) => ({
  validation: run(() => validateCustomFormSubmission(schema, submission)),
  route: run(() => formRoute(schema, submission)),
  pruned: run(() => pruneFormAnswers(schema, submission)),
}))));
