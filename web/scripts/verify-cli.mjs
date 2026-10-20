import assert from 'node:assert/strict';
import { mkdtempSync, readFileSync, writeFileSync, rmSync, symlinkSync, linkSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { spawnSync } from 'node:child_process';
import { root, fixture, json, decode, verifyReferences } from './validation.mjs';

const binary = path.resolve(process.argv[2] ?? path.join(root, 'bin/fittools'));
verifyReferences();
const directory = mkdtempSync(path.join(tmpdir(), 'fit-cli-'));
function run(args, success = true) {
  const result = spawnSync(binary, args, { encoding: 'utf8', timeout: 15000 });
  assert.ifError(result.error);
  if (success) assert.equal(result.status, 0, result.stderr);
  else assert.notEqual(result.status, 0, 'invalid CLI invocation succeeded');
  return result;
}
try {
  for (const c of json('cases.json')) {
    const input = path.join(directory, 'input.fit');
    const output = path.join(directory, 'output.fit');
    const original = fixture(`${c.name}.input.bin`);
    writeFileSync(input, original);
    const args = [c.action, input, output];
    if (c.action === 'identity') {
      for (const [prefix, identity] of [['from', c.source], ['to', c.target]])
        for (const [key, value] of Object.entries(identity)) args.push(`--${prefix}-${key}`, String(value));
      if (c.firmware !== null) args.push('--to-firmware', (c.firmware / 100).toFixed(2));
    }
    run(args);
    const actual = readFileSync(output);
    assert.deepEqual(actual, fixture(`${c.name}.output.bin`), c.name);
    assert.deepEqual(readFileSync(input), original, 'input must remain unchanged');
    decode(actual, c.name);
  }
  const input = path.join(directory, 'source.fit');
  const output = path.join(directory, 'protected.fit');
  writeFileSync(input, fixture('web.input.bin'));
  writeFileSync(output, 'existing output');
  run(['unknown'], false);
  run(['pauses', input, input], false);
  for (const [name, create] of [['symlink.fit', symlinkSync], ['hardlink.fit', linkSync]]) {
    const alias = path.join(directory, name);
    create(input, alias);
    run(['pauses', input, alias], false);
  }
  writeFileSync(input, 'invalid FIT');
  run(['pauses', input, output], false);
  assert.equal(readFileSync(output, 'utf8'), 'existing output');
  console.log('CLI: eight frozen goldens, independent SDK decoding, input preservation, errors and output safety passed.');
} finally { rmSync(directory, { recursive: true, force: true }); }
