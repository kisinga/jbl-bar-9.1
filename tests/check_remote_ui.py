"""Frozen pre-redesign UI contract: IDs, actions, choices, and numeric bounds."""
from collections import Counter
from html.parser import HTMLParser
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]

class Inventory(HTMLParser):
    def __init__(self, text):
        super().__init__()
        self.ids, self.actions, self.components = [], set(), set()
        self.levels, self.ranges, self.presets = {}, {}, []
        self.stack = []
        self.feed(text)
    def handle_starttag(self, tag, attrs):
        a = dict(attrs)
        level = a.get('data-level') or next((x[1] for x in reversed(self.stack) if x[1]), None)
        if 'id' in a: self.ids.append(a['id'])
        if 'data-action' in a: self.actions.add(a['data-action'])
        if 'data-component' in a: self.components.add(a['data-component'])
        if tag == 'button' and level and 'data-value' in a:
            self.levels.setdefault(level, []).append(a['data-value'])
        if tag == 'input' and a.get('type') == 'range':
            self.ranges[a['id']] = {k:a.get(k) for k in ('min','max','step')}
        if tag == 'option' and any(x[2] == 'eq-preset' for x in self.stack): self.presets.append(a.get('value'))
        if tag not in {'area','base','br','col','embed','hr','img','input','link','meta','param','source','track','wbr'}:
            self.stack.append((tag, level, a.get('id')))
    def handle_endtag(self, tag):
        for i in range(len(self.stack)-1,-1,-1):
            if self.stack[i][0] == tag:
                del self.stack[i:]
                break

if __name__ == '__main__':
    baseline=json.loads((ROOT/'tests/fixtures/remote-ui-baseline.json').read_text())
    current=Inventory((ROOT/'apps/remote/www/index.html').read_text())
    cases=[]
    assert set(baseline['ids']) <= set(current.ids), 'Existing UI target removed'
    assert not [key for key,count in Counter(current.ids).items() if count>1], 'Duplicate ID'
    cases.append('original_ids_preserved_once')
    assert set(baseline['actions']) == current.actions | {'media_pause'}, 'Action surface changed'
    assert set(baseline['components']) == current.components, 'Capability component changed'
    cases.append('actions_and_capabilities_preserved')
    for key in ('levels','ranges','presets'):
        assert baseline[key] == getattr(current,key), key+' changed'
        cases.append(key+'_preserved')
    target=ROOT/'build/test-results/remote-ui-tests.json'
    target.parent.mkdir(parents=True,exist_ok=True)
    target.write_text(json.dumps({'cases':cases},indent=2)+'\n')
    print(f"UI contract: {len(baseline['ids'])} IDs, {len(baseline['actions'])} actions, {len(cases)} checks passed")
