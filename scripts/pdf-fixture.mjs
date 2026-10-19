// `generate`: writes a corpus of PDFs made with pdf-lib (valid, object
// streams, many pages, and damaged variants) as base64 JSON.
// `check`: for base64 PDFs on stdin, whether PDFDocument.load succeeds with
// at least one page (form_file_service.ts).
import { createRequire } from 'node:module';
import { readFileSync } from 'node:fs';
import { resolve, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';

const require = createRequire(resolve(dirname(fileURLToPath(import.meta.url)), '../tests/interop/package.json'));
const { PDFDocument } = require('pdf-lib');

if (process.argv[2] === 'generate') {
  const out = [];
  for (const [pages, useObjectStreams] of [[1, true], [1, false], [3, true], [12, false], [0, true], [0, false]]) {
    const doc = await PDFDocument.create();
    for (let i = 0; i < pages; i++) doc.addPage([200, 200]).drawText(`page ${i}`);
    const bytes = Buffer.from(await doc.save({ useObjectStreams }));
    out.push(bytes.toString('base64'));
    out.push(bytes.subarray(0, Math.floor(bytes.length / 2)).toString('base64')); // truncated
    out.push(Buffer.concat([bytes, Buffer.from('\n% trailing junk')]).toString('base64'));
    out.push(Buffer.from(bytes.toString('latin1').replace(/\/Root \d+ 0 R/, '/Root 999 0 R'), 'latin1').toString('base64'));
    out.push(Buffer.from(bytes.toString('latin1').replace('trailer', 'trailer\n<< /Encrypt 1 0 R >>\ntrailer'), 'latin1').toString('base64'));
  }
  out.push(Buffer.from('%PDF-1.4\n').toString('base64'));
  out.push(Buffer.from('%PDF-1.4\n1 0 obj << /Type /Catalog /Pages 2 0 R >> endobj\n2 0 obj << /Type /Pages /Kids [3 0 R] /Count 1 >> endobj\n3 0 obj << /Type /Page /Parent 2 0 R >> endobj\n').toString('base64'));
  console.log(JSON.stringify(out));
} else {
  const inputs = JSON.parse(readFileSync(0, 'utf8'));
  const out = [];
  for (const b64 of inputs) {
    try {
      const pdf = await PDFDocument.load(Buffer.from(b64, 'base64'));
      out.push(pdf.getPageCount() >= 1);
    } catch {
      out.push(false);
    }
  }
  console.log(JSON.stringify(out));
}
