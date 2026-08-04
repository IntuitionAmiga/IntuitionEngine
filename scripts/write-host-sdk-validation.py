#!/usr/bin/env python3
import hashlib, json, os, sys
stage, output, revision, epoch, qbe, cproc, picolibc = sys.argv[1:]
files=[]
for root, _, names in os.walk(stage):
    for name in names:
        path=os.path.join(root,name)
        if os.path.isfile(path):
            rel=os.path.relpath(path, stage).replace(os.sep, '/')
            files.append({'path':rel,'sha256':hashlib.file_digest(open(path,'rb'),'sha256').hexdigest()})
files.sort(key=lambda f:f['path'])
value={'format':'intuition-engine-host-sdk-validation-1','revision':revision,'release_epoch':int(epoch),'header_targets':['IE_TARGET_IE64','IE_TARGET_M68K','IE_TARGET_Z80','IE_TARGET_6502','IE_TARGET_X86'],'components':[{'name':'QBE','revision':qbe,'licence':'QBE-LICENSE'},{'name':'cproc','revision':cproc,'licence':'cproc-LICENSE'},{'name':'Picolibc','revision':picolibc,'licence':'COPYING.picolibc'}],'executables':sorted(os.listdir(os.path.join(stage,'bin'))),'files':files}
with open(output,'w',encoding='utf-8',newline='\n') as f: json.dump(value,f,ensure_ascii=False,sort_keys=True,separators=(',',':')); f.write('\n')
