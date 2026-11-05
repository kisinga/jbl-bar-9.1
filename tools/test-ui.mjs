// Shared local/CI runner. Each suite owns its fixture server and browser.
import {spawn} from 'node:child_process';
import {fileURLToPath} from 'node:url';
import {performance} from 'node:perf_hooks';

const root = fileURLToPath(new URL('../', import.meta.url));
const groups = {
  remote: ['webremote-ui', 'component-ui', 'bass-offset-ui'],
  setup: ['setup-ui', 'setup-local'],
};
const args = process.argv.slice(2);
if (args.length && (args.length !== 2 || args[0] !== '--suite' || !['all', ...Object.keys(groups)].includes(args[1]))) {
  console.error('Usage: npm run test:ui -- [--suite all|remote|setup]');
  process.exit(2);
}
const group = args[1] || 'all';
try {
  await import(process.env.PLAYWRIGHT_MODULE || 'playwright');
} catch {
  console.error('Browser tools missing. Run: npm ci && npm run test:ui:install');
  process.exit(1);
}
const pending = group === 'all' ? Object.values(groups).flat() : [...groups[group]];
const children = new Set();
let failed = false;
for (const signal of ['SIGINT', 'SIGTERM']) {
  process.on(signal, () => {
    pending.length = 0;
    failed = true;
    for (const child of children) child.kill(signal);
  });
}
async function worker() {
  while (pending.length) {
    const suite = pending.shift();
    const start = performance.now();
    console.log(`${suite}: RUN`);
    const child = spawn(process.execPath, [`tests/${suite}.mjs`], {cwd: root, stdio: 'inherit'});
    children.add(child);
    const code = await new Promise(resolve => {
      child.once('error', error => {console.error(error.message); resolve(1);});
      child.once('exit', code => resolve(code ?? 1));
    });
    children.delete(child);
    if (code !== 0) failed = true;
    console.log(`${suite}: ${code === 0 ? 'PASS' : 'FAIL'} (${((performance.now() - start) / 1000).toFixed(1)}s)`);
  }
}
// Two workers bound Chromium memory use on small CI runners.
await Promise.all([worker(), worker()]);
process.exitCode = failed ? 1 : 0;
