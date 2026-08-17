#!/usr/bin/env python3
import hashlib, json, os, sys
stage, inventory = sys.argv[1:]
value=json.load(open(inventory,encoding='utf-8'))
assert value['format'] == 'intuition-engine-host-sdk-validation-1'
expected_executables = ['cproc-qbe','ie32asm','ie32to64','ie64-ar','ie64-cproc','ie64-ranlib','ie64asm','ie64dis','ie64ld','qbe']
if all(name.endswith('.exe') for name in value['executables']):
    expected_executables = [name + '.exe' for name in expected_executables]
assert value['executables'] == expected_executables
for item in value['files']:
    path=os.path.join(stage,item['path'])
    assert os.path.isfile(path) and hashlib.file_digest(open(path,'rb'),'sha256').hexdigest() == item['sha256']
