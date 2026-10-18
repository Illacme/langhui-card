import re, os, glob
from collections import defaultdict
root = '/Users/eason/WorkBuddy/名片管理/miniprogram'
wxss = open(os.path.join(root,'app.wxss'), encoding='utf-8').read()

# app.wxss 中无祖先约束的独立类（会命中任何同名元素）
global_cls = set()
for m in re.finditer(r'(?m)^([^\n{]*)\{', wxss):
    sel = m.group(1).strip()
    if not sel or sel.startswith('/*') or sel.startswith('@'): continue
    for part in sel.split(','):
        t = re.split(r'\s+|>', part.strip())[0].split(':')[0]
        if re.fullmatch(r'\.[A-Za-z0-9_-]+', t): global_cls.add(t[1:])

def classes(path):
    s = open(path, encoding='utf-8').read()
    out = set()
    for m in re.finditer(r'class="([^"]*)"', s):
        for c in re.split(r'[\s{}]+', m.group(1)):
            if re.fullmatch(r'[A-Za-z][A-Za-z0-9_-]*', c): out.add(c)
    return out

usage = defaultdict(set)
for f in glob.glob(os.path.join(root,'components','*','*.wxml')) + glob.glob(os.path.join(root,'pages','**','*.wxml'), recursive=True):
    area = f.replace(root+'/','').split('/')[1] if '/pages/' in f else os.path.basename(os.path.dirname(f))
    for c in classes(f): usage[c].add(area)

print('=== 危险项：独立全局类被“组件 与 其他页面/组件”同时使用（会被跨模块污染）===')
bad = []
for c in sorted(global_cls):
    users = usage.get(c, set())
    comps = {u for u in users if u in {os.path.basename(os.path.dirname(p)) for p in glob.glob(os.path.join(root,'components','*','index.wxml'))}}
    others = users - comps
    if comps and others:
        bad.append((c, sorted(comps), sorted(others)))
for c, a, b in bad: print(f'  .{c:14s} 组件={a}  其它页面={b}')
if not bad: print('  无 —— 组件与页面之间已无同名全局类冲突')

print()
print('=== 提示项：组件内使用了通用字类名（cell/row/label/val/name/title/info 等），未来新增全局同名类仍有泄漏风险 ===')
GENERIC = {'cell','row','rows','label','val','name','title','info','head','item','list','cover','text','icon','empty','tip','avatar','logo','link','btn','body','foot','bar','row-line','line','wrap'}
for f in sorted(glob.glob(os.path.join(root,'components','*','*.wxml'))):
    comp = os.path.basename(os.path.dirname(f))
    hit = sorted(c for c in classes(f) if c in GENERIC)
    if hit: print(f'  {comp:18s} {hit}')
