#!/usr/bin/env python3
import hashlib, json, os, sys
stage, inventory = sys.argv[1:]
value=json.load(open(inventory,encoding='utf-8'))
assert value['format'] == 'intuition-engine-host-sdk-validation-1'
assert value['executables'] == ['cproc-qbe','ie32asm','ie32to64','ie64-ar','ie64-cproc','ie64-ranlib','ie64asm','ie64dis','ie64ld','qbe']
for item in value['files']:
    path=os.path.join(stage,item['path'])
    assert os.path.isfile(path) and hashlib.file_digest(open(path,'rb'),'sha256').hexdigest() == item['sha256']
