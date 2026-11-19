#!/usr/bin/env python3
"""Lists utility classes used in ui/src that neither Kandev's compiled
stylesheet nor ui/plugin.css defines.

Usage: python3 scripts/check-host-css.py <kandev>/apps/web/dist/assets
"""
import re,glob,sys
css="".join(open(f).read() for f in glob.glob(sys.argv[1].rstrip('/') + '/*.css'))
own=open('ui/plugin.css').read()
def escaped(cls):
    return "."+re.sub(r'([:/\[\]\.%()#,>+~=!*&])',lambda m:"\\"+m.group(1),cls)
tokens=set()
for f in glob.glob('ui/src/**/*.ts*',recursive=True):
    if '.test.' in f: continue
    src=open(f).read()
    for s in re.findall(r'"([^"\n]*)"|`([^`\n]*)`',src):
        s=s[0] or s[1]
        for t in s.split():
            if re.match(r'^-?[a-z][a-z0-9-]*(?::[a-z0-9-]+)*(?:-|:)?[\w\[\]().,%/:-]*$',t) and ('-' in t or t in {'flex','grid','hidden','block','contents','truncate','border','rounded','shadow','italic','underline','relative','absolute','fixed','sticky','inline','uppercase','sr-only'}):
                tokens.add(t)
missing=sorted(t for t in tokens if escaped(t) not in css and escaped(t) not in own)
print(" ".join(missing))
