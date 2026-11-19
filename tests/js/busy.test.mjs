// runWithBtnBusy must give the button its ORIGINAL child nodes back. Regression: it restored innerHTML, which
// created new elements, so cached references like el.pruneCount ended up detached and the "🧹 Temizle (3)" counter
// froze after the first click. Run: node --test tests/js
import { test } from 'node:test';
import assert from 'node:assert/strict';

class FakeNode {
  constructor(tag, text = '') { this.tag = tag; this.text = text; this.childNodes = []; this.disabled = false; this.cls = new Set(); this.classList = { add: (c) => this.cls.add(c), remove: (c) => this.cls.delete(c) }; }
  set textContent(v) { this.childNodes = [new FakeNode('#text', v)]; }
  get textContent() { return this.childNodes.map((c) => c.text || c.textContent).join(''); }
  get innerHTML() { return 'RENDERED-AGAIN'; }
  replaceChildren(...nodes) { this.childNodes = nodes; }
}
globalThis.document = { getElementById: () => null };
const { runWithBtnBusy } = await import('../../web/modules/utils.js');

test('the same child nodes (and the counter span inside) are restored after the busy state', async () => {
  const btn = new FakeNode('button');
  const label = new FakeNode('#text', '🧹 Temizle ');
  const count = new FakeNode('span', '3');
  btn.childNodes = [label, count];

  let during;
  await runWithBtnBusy(btn, '🧹 Temizleniyor...', async () => {
    during = btn.textContent;
    count.text = '0'; // render sırasında güncellenen sayaç (kopuk düğüm)
  });
  assert.equal(during, '🧹 Temizleniyor...');
  assert.equal(btn.childNodes[0], label);
  assert.equal(btn.childNodes[1], count, 'the very same span must be back in the button');
  assert.equal(btn.textContent, '🧹 Temizle 0', 'the update made while busy is visible afterwards');
  assert.equal(btn.disabled, false);
});

test('without a button the function still runs', async () => {
  assert.equal(await runWithBtnBusy(null, 'x', async () => 42), 42);
});
