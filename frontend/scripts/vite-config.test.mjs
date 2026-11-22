import assert from 'node:assert/strict'
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join, normalize } from 'node:path'
import { pathToFileURL } from 'node:url'
import test from 'node:test'
import { loadConfigFromFile } from 'vite'
import { rolldown } from 'rolldown'
import { frontendRoot } from './source-analysis.mjs'

// vite-config.test.mjs verifies configuration selection for frontend CLI scripts.
// Older checkouts can retain compiled JavaScript beside the TypeScript source.
// Each command must explicitly load the current source even when that obsolete
// artifact exists, rather than relying on Vite's default filename precedence.
// A production-config fixture also checks emitted static imports and executes lazy
// chunks, protecting feature download boundaries and dependency initialization order.
//
// Rolldown is declared as a development dependency because the splitting fixture
// imports the compiler directly. This keeps clean package installations able to
// execute the same compiler contract used by Vite.

// Development, build, and preview scripts run in a directory containing both
// config formats. Their configured paths must select TypeScript and ignore obsolete JavaScript.
test('Vite scripts select TypeScript despite stale JavaScript configuration', async (context) => {
  const directory = mkdtempSync(join(tmpdir(), 'tokenrouter-vite-config-'))
  context.after(() => rmSync(directory, { recursive: true, force: true }))
  writeFileSync(join(directory, 'package.json'), JSON.stringify({ type: 'module' }))
  writeFileSync(join(directory, 'vite.config.js'), 'throw new Error("Stale configuration loaded")')
  writeFileSync(
    join(directory, 'vite.config.ts'),
    'export default { define: { __CONFIG_SOURCE__: "true" } }',
  )
  const { scripts } = JSON.parse(readFileSync(join(frontendRoot, 'package.json'), 'utf8'))
  for (const name of ['dev', 'build', 'build:assets', 'preview']) {
    const command = scripts[name].split('&&').at(-1).trim().split(/\s+/)
    assert.equal(command[0], 'vite', name)
    const optionIndex = command.indexOf('--config')
    const configFile = optionIndex < 0 ? undefined : join(directory, command[optionIndex + 1])
    const loaded = await loadConfigFromFile(
      { command: command.includes('build') ? 'build' : 'serve', mode: 'production' },
      configFile,
      directory,
    )
    assert.equal(normalize(loaded.path), join(directory, 'vite.config.ts'), name)
    assert.equal(loaded.config.define.__CONFIG_SOURCE__, 'true', name)
  }
})

// A shared HTTP dependency starts before unrelated editors, effects, and payment
// features. Production splitting must defer their code and initialization until
// the corresponding dynamic import runs, including packages without a named group.
test('production vendor chunks preserve lazy feature downloads and execution', async (context) => {
  const directory = mkdtempSync(join(tmpdir(), 'tokenrouter-vite-splitting-'))
  const executionKey = `tokenrouter-splitting-${directory}`
  globalThis[executionKey] = []
  context.after(() => {
    delete globalThis[executionKey]
    delete globalThis[`${executionKey}-app`]
    rmSync(directory, { recursive: true, force: true })
  })
  const features = [
    'fabric',
    'cropperjs',
    'matter-js',
    'ogl',
    'lottie-web',
    'marked',
    'driver.js',
    'qrcode',
    'xlsx',
    '@stripe/stripe-js',
    '@airwallex/components-sdk',
    'unclassified-feature',
  ]
  for (const name of ['axios', ...features]) {
    const packageRoot = join(directory, 'node_modules', name)
    mkdirSync(packageRoot, { recursive: true })
    writeFileSync(
      join(packageRoot, 'package.json'),
      JSON.stringify({ name, type: 'module', main: 'index.js', sideEffects: true }),
    )
    writeFileSync(
      join(packageRoot, 'index.js'),
      `globalThis[${JSON.stringify(executionKey)}].push(${JSON.stringify(name)});
export const value = ${JSON.stringify(name)};`,
    )
  }
  writeFileSync(
    join(directory, 'entry.js'),
    `import { value } from 'axios';
globalThis[${JSON.stringify(`${executionKey}-app`)}] = {
  initialValue: value,
  features: [${features.map((name) => `() => import(${JSON.stringify(name)})`).join(',')}]
};`,
  )
  const loaded = await loadConfigFromFile(
    { command: 'build', mode: 'production' },
    join(frontendRoot, 'vite.config.ts'),
    frontendRoot,
  )
  const buildOptions = loaded.config.build.rolldownOptions
  const splitting = buildOptions.output
  assert.equal(splitting.strictExecutionOrder, true)
  const bundle = await rolldown({
    input: join(directory, 'entry.js'),
    preserveEntrySignatures: buildOptions.preserveEntrySignatures,
  })
  context.after(() => bundle.close())
  const emitted = await bundle.write({
    ...splitting,
    dir: join(directory, 'dist'),
    format: 'es',
    entryFileNames: 'entry.mjs',
    chunkFileNames: '[name]-[hash].mjs',
  })
  const chunks = emitted.output.filter((chunk) => chunk.type === 'chunk')
  const initial = chunks.find((chunk) => chunk.isEntry)
  assert.ok(initial, 'The fixture must emit an entry chunk')
  const staticChunks = new Set()
  const queue = [initial.fileName]
  while (queue.length) {
    const name = queue.pop()
    if (staticChunks.has(name)) continue
    staticChunks.add(name)
    const chunk = chunks.find((candidate) => candidate.fileName === name)
    assert.ok(chunk, `Missing emitted static dependency: ${name}`)
    queue.push(...chunk.imports)
    assert.equal(
      Object.keys(chunk.modules).some((id) =>
        features.some((feature) => id.replaceAll('\\', '/').includes(`/node_modules/${feature}/`)),
      ),
      false,
      `${name} must not download a feature library during startup`,
    )
  }
  await import(pathToFileURL(join(directory, 'dist', initial.fileName)).href)
  const app = globalThis[`${executionKey}-app`]
  assert.equal(app.initialValue, 'axios')
  assert.deepEqual(globalThis[executionKey], ['axios'])
  for (const [index, name] of features.entries()) {
    assert.equal((await app.features[index]()).value, name)
    assert.deepEqual(globalThis[executionKey], ['axios', ...features.slice(0, index + 1)])
  }
})
