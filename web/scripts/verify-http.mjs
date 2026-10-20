import assert from 'node:assert/strict';
import { fixture, json, decode, verifyReferences } from './validation.mjs';

const base = process.argv[2] ?? 'http://127.0.0.1:8080';
const source = fixture('web.input.bin');
const contract = json('http.json');
verifyReferences();
async function get(endpoint) {
  const response = await fetch(base + endpoint, { signal: AbortSignal.timeout(15000) });
  assert.equal(response.status, 200, endpoint);
  return response;
}
async function post(endpoint, fields = {}, bytes = source, filename = 'Ultra Run.fit') {
  const body = new FormData();
  if (bytes !== null) body.append('fit_file', new Blob([bytes]), filename);
  for (const [key, value] of Object.entries(fields)) body.append(key, value);
  return fetch(base + endpoint, { method: 'POST', body, signal: AbortSignal.timeout(15000) });
}
assert.equal(await (await get('/healthz')).text(), 'ok\n');
assert.match(await (await get('/')).text(), /Fittools/);
const presets = await get('/device-presets');
assert.equal(presets.headers.get('cache-control'), 'no-store');
const presetData = await presets.json();
assert.ok(Array.isArray(presetData));
// Optional mounted configuration assertion for container smoke tests.
if (process.env.EXPECT_PRESET_SERIAL)
  assert.equal(presetData[0].serial, Number(process.env.EXPECT_PRESET_SERIAL));
for (const [endpoint, expected] of Object.entries(contract.inspection)) {
  const response = await post('/' + endpoint);
  assert.equal(response.status, 200);
  assert.deepEqual(await response.json(), expected, endpoint);
}
for (const c of contract.transformations) {
  const response = await post('/transform', c.fields);
  assert.equal(response.status, 200);
  assert.equal(response.headers.get('content-type'), 'application/octet-stream');
  const suffix = { pauses: 'timer-pauses', continuous: 'continuous', identity: 'new-device' }[c.fields.action];
  assert.match(response.headers.get('content-disposition'), new RegExp(`^attachment; filename="?Ultra-Run-${suffix}\\.fit"?$`));
  const actual = Buffer.from(await response.arrayBuffer());
  assert.deepEqual(actual, fixture(c.output), c.output);
  decode(actual, c.output);
}
const restored = await post('/transform', {action:'identity',manufacturer:'1',product:'4953',serial:'100000001',firmware:'6.38'}, fixture('web-identity-firmware.output.bin'));
assert.equal(restored.status, 200);
assert.deepEqual(Buffer.from(await restored.arrayBuffer()), source, 'HTTP identity reversal');
for (const [endpoint, fields, data, filename] of [
  ['/transform', {action:'pauses'}, Buffer.from('invalid FIT')],
  ['/inspect-identity', {}, Buffer.from('invalid FIT')],
  ['/activity-details', {}, Buffer.from('invalid FIT')],
  ['/transform', {action:'unknown'}, source],
  ['/transform', {action:'identity',manufacturer:'1',product:'4315',serial:'0'}, source],
  ['/transform', {action:'pauses'}, null],
  ['/transform', {action:'pauses'}, source, 'wrong.txt'],
]) {
  const response = await post(endpoint, fields, data, filename);
  assert.equal(response.status, 400);
  assert.equal(typeof (await response.json()).error, 'string');
}
for (const endpoint of ['/transform', '/inspect-identity', '/activity-details']) {
  const response = await fetch(base + endpoint, { signal: AbortSignal.timeout(15000) });
  assert.equal(response.status, 405);
  assert.equal(typeof (await response.json()).error, 'string');
}
console.log('HTTP: frozen inspection and transformation contracts, SDK decoding, reversal, presets and errors passed.');
