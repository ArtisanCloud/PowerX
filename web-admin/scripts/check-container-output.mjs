// The shipped Nitro server currently contains only platform-independent JS.
// Fail the build if a future module introduces a native runtime dependency.
import { readdirSync, readFileSync, statSync, realpathSync } from 'node:fs';
import { resolve } from 'node:path';
const visited = new Set();
function check(path) {
  const real = realpathSync(path);
  if (visited.has(real)) return;
  visited.add(real);
  if (statSync(real).isDirectory()) {
    for (const file of readdirSync(real)) check(resolve(real, file));
  } else {
    const bytes = readFileSync(real);
    if (real.endsWith('.node') || bytes.subarray(0, 4).equals(Buffer.from([0x7f, 0x45, 0x4c, 0x46]))) {
      throw new Error('Native Nitro runtime artifact requires target-platform build: ' + real);
    }
  }
}
check(resolve('.output/server'));
console.log('Nitro runtime contains no platform-specific native artifacts.');
