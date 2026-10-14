import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import ts from 'typescript';

// Pin the tokens-column cache line and the shared hit-rate colour helper.
const source = readFileSync(new URL('./requests-columns.tsx', import.meta.url), 'utf8');
const ast = ts.createSourceFile('requests-columns.tsx', source, ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX);

let helper;
async function loadHelper() {
  if (helper) return helper;

  const node = ast.statements.find((n) => ts.isFunctionDeclaration(n) && n.name?.text === 'cacheHitRateColor');
  assert.ok(node, 'cacheHitRateColor must stay exported from requests-columns.tsx');

  const { outputText } = ts.transpileModule(`export ${node.getText(ast).replace(/^export\s+/, '')}`, {
    compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2023 },
  });
  helper = await import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`);
  return helper;
}

test('cacheHitRateColor bands the hit rate from green down to red', async () => {
  const { cacheHitRateColor } = await loadHelper();

  assert.match(cacheHitRateColor(100), /green/, '>=95 is green');
  assert.match(cacheHitRateColor(95), /green/, 'the 95 boundary is green');
  assert.match(cacheHitRateColor(90), /blue/, '90..95 is blue');
  assert.equal(cacheHitRateColor(80), '', '80..90 keeps the default colour');
  assert.match(cacheHitRateColor(50), /yellow/, '50..80 is yellow');
  assert.match(cacheHitRateColor(49), /red/, 'below 50 is red');
  assert.match(cacheHitRateColor(0), /red/, '0 is red');
});

test('the tokens column shows a coloured cache rate line only when the cache was hit', () => {
  const tokensColumn = source.slice(source.indexOf("id: 'tokens'"), source.indexOf("id: 'readCache'"));

  assert.match(tokensColumn, /cachedTokens > 0 &&/, 'the cache line is conditional on a non-zero cache read');
  assert.match(tokensColumn, /requests\.columns\.cacheRate/, 'the cache line uses the cacheRate label');
  assert.match(tokensColumn, /cacheHitRateColor\(hitRate\)/, 'the cache line is coloured by the shared helper');
  assert.match(tokensColumn, /promptCachedTokens \|\| 0/, 'the cache line reads promptCachedTokens');
});

test('the read-cache column reuses the shared helper instead of a private ladder', () => {
  const readCacheColumn = source.slice(source.indexOf("id: 'readCache'"), source.indexOf("id: 'writeCache'"));

  assert.match(readCacheColumn, /cacheHitRateColor\(hitRate\)/, 'read-cache must reuse the shared helper');
  assert.doesNotMatch(readCacheColumn, /hitRate >= 95\s*\n?\s*\?/, 'the private colour ladder must be gone');
});
