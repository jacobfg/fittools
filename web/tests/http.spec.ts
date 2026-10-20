import { test } from '@playwright/test';
import { execFileSync } from 'node:child_process';

test('live HTTP contracts and independent FIT decoding', () => {
  execFileSync(process.execPath, ['scripts/verify-http.mjs'], { stdio: 'pipe', timeout: 25000 });
});
