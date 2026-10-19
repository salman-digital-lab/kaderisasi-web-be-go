// Sorts each list of keys from stdin with localeCompare (Node's default ICU
// collation), as form_session_service.ts schemaHash does.
import { readFileSync } from 'node:fs';
const lists = JSON.parse(readFileSync(0, 'utf8'));
console.log(JSON.stringify(lists.map((keys) => [...keys].sort((a, b) => a.localeCompare(b)))));
