import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { createHash } from 'node:crypto';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import { Decoder, Stream } from '@garmin/fitsdk';

export const root = fileURLToPath(new URL('../../', import.meta.url));
export const fixture = name => readFileSync(path.join(root, 'testdata', name));
export const json = name => JSON.parse(fixture(name));
export function decode(bytes, label) {
  const decoder = new Decoder(Stream.fromByteArray(bytes));
  assert.equal(decoder.checkIntegrity(), true, `${label}: FIT integrity`);
  const { messages, errors } = decoder.read();
  assert.deepEqual(errors, [], `${label}: independent SDK decoding`);
  return messages;
}
export function verifyReferences() {
  for (const [name, metadata] of Object.entries(json('provenance.json').fixtures)) {
    const bytes = fixture(name);
    assert.equal(createHash('sha256').update(bytes).digest('hex'), metadata.sha256, `${name}: frozen reference hash`);
    const decoder = new Decoder(Stream.fromByteArray(bytes));
    assert.equal(decoder.checkIntegrity(), true, `${name}: reference FIT integrity`);
    // Garmin JS SDK does not support compressed inputs; all output goldens decode.
    if (name.endsWith('.output.bin')) decode(bytes, name);
  }
}
