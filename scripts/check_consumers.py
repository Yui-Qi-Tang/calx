#!/usr/bin/env python3
"""Bounded source-snapshot conformance; never changes or runs provider stores.

Requires explicit repository roots, pinned tools in CALX_* environment variables
and a fresh output directory. Native workflows beyond these selected boundaries
are not exercised. Source copies and manifests remain in the ignored output.
"""
import argparse
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys

sys.dont_write_bytecode = True
ROOT = Path(__file__).resolve().parents[1]

def sha(raw):
    return hashlib.sha256(raw).hexdigest()

def function(text, name):
    start = text.index('func ' + name + '(')
    nxt = re.search(r'^func ', text[start + 5:], re.M)
    return text[start:start + 5 + nxt.start()] if nxt else text[start:]

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ('core', 'mcp', 'pouch', 'lab', 'out'):
        parser.add_argument('--' + name, required=True, type=Path)
    args = parser.parse_args()
    if sys.version_info < (3, 10):
        parser.error("Python 3.10+ is required by the selected Lab source; use python3.14")
    out = args.out.resolve()
    out.mkdir(parents=True, exist_ok=False)
    env = dict(os.environ, PYTHONDONTWRITEBYTECODE='1', CALX_CONSUMER_OUTPUT=str(out))
    for key in ('CALX_CADICAL','CALX_CADICAL_SHA256','CALX_DRAT','CALX_DRAT_SHA256'):
        if not env.get(key):
            raise RuntimeError(key + ' is required')
    manifest = {}
    results = {}
    def source(path):
        raw = path.read_bytes();manifest[str(path)] = sha(raw);return raw
    def run(command, cwd, name, custom_env=None):
        proc = subprocess.run(command, cwd=cwd, env=custom_env or env, capture_output=True)
        (out/(name+'.stdout')).write_bytes(proc.stdout)
        (out/(name+'.stderr')).write_bytes(proc.stderr)
        results[name] = {'exit_code':proc.returncode}
        if proc.returncode:
            raise RuntimeError(name + ' failed: ' + proc.stderr.decode(errors='replace')[-1500:])
        return proc
    try:
        binary = out/'calx'
        run(['go','build','-o',str(binary),'./cmd/calx'],ROOT,'build-cli')
        for kind in ('core','mcp'):
            root = getattr(args,kind)
            text = source(root/'internal/evidenceingestion/consistency_check.go').decode()
            scope = source(root/'internal/evidenceingestion/consistency_scope.go').decode()
            target = out/kind;target.mkdir()
            types = text[text.index('// ConsistencyAtom'):text.index('type ConsistencyOutcome')]
            stubs = 'type ConsistencyScope struct{}\ntype ConsistencyMember struct{NodeID string}\ntype ConsistencyView struct{Members []ConsistencyMember}\n'
            content = 'package consumer\nimport("fmt";"slices";"strings";"unicode/utf8";logicresolver "github.com/Yui-Qi-Tang/calx/sat")\n'+types+stubs+function(text,'compileConsistency')+function(scope,'consistencyText')
            (target/'compiler.go').write_text(content)
            (target/'compiler_test.go').write_bytes((ROOT/'scripts/testdata/ahe_test.go.txt').read_bytes())
            (target/'go.mod').write_text('module example.com/'+kind+'\ngo 1.27.2\nrequire github.com/Yui-Qi-Tang/calx v0.0.0\nreplace github.com/Yui-Qi-Tang/calx => '+str(ROOT)+'\n')
            run(['gofmt','-w','.'],target,kind+'-format')
            run(['go','test','-json','-count=1','./...'],target,kind,dict(env,CALX_CONSUMER_KIND=kind))
        target = out/'pouch'
        for package in ('planning','validation','projection'):
            dest=target/'internal'/package;dest.mkdir(parents=True)
            for path in (args.pouch/'internal'/package).glob('*.go'):
                # Planning tests include the independent tiny SAT oracle and fixture.
                if package=='validation' and path.name.endswith('_test.go'):
                    continue
                (dest/path.name).write_bytes(source(path))
        (target/'internal/planning/calx_test.go').write_bytes((ROOT/'scripts/testdata/pouch_test.go.txt').read_bytes())
        (target/'internal/projection/calx_test.go').write_bytes((ROOT/'scripts/testdata/projection_test.go.txt').read_bytes())
        (target/'go.mod').write_text('module github.com/Yui-Qi-Tang/pouch\ngo 1.27.2\nrequire github.com/Yui-Qi-Tang/calx v0.0.0\nreplace github.com/Yui-Qi-Tang/calx => '+str(ROOT)+'\n')
        run(['gofmt','-w','internal/planning/calx_test.go'],target,'pouch-format')
        run(['go','test','-json','-count=1','-run','TestCalx','./internal/planning','./internal/projection'],target,'pouch')
        # Snapshot the Python encoder and independent original-rule receiver.
        lab = out/'lab';lab.mkdir()
        for name in ('encoder','h4_encoder','reference','h4_reference'):
            (lab/(name+'.py')).write_bytes(source(args.lab/'experiments/h6/src'/(name+'.py')))
        (lab/'h2.py').write_bytes(source(args.lab/'experiments/h2/src/encoding.py'))
        sys.path.insert(0,str(lab))
        def load(name):
            spec=importlib.util.spec_from_file_location(name,lab/(name+'.py'))
            module=importlib.util.module_from_spec(spec);sys.modules[name]=module;spec.loader.exec_module(module);return module
        h2,encoder,reference=load('h2'),load('encoder'),load('reference')
        def solve(raw,name,command='solve',fmt='dimacs'):
            command=[str(binary),command,'--format',fmt,'--solver',env['CALX_CADICAL'],'--solver-sha256',env['CALX_CADICAL_SHA256'],'--checker',env['CALX_DRAT'],'--checker-sha256',env['CALX_DRAT_SHA256'],'--artifacts',str(lab/name)]
            p=subprocess.run(command,input=raw.encode(),env=env,capture_output=True)
            (lab/(name+'.json')).write_bytes(p.stdout)
            if p.returncode:raise RuntimeError(p.stdout.decode(errors='replace'))
            return json.loads(p.stdout)['data']['result']
        matrix=h2.compile_rule({'id':'guard','guard':{'operator':'all','conditions':['permit','water_ok']},'effect':{'fed':True},'frame':['permit','water_ok'],'action':'feed','bindings':[]})
        counts={'h2':0,'h6':0,'replayed':0}
        for a in (False,True):
            for b in (False,True):
                for enabled in (False,True):
                    cnf,_=h2.truth_query(matrix,{'permit':a,'water_ok':b},enabled)
                    result=solve(cnf,'h2-'+str(counts['h2']))
                    assert result['status']==('SAT' if enabled==(a and b) else 'UNSAT')
                    counts['h2']+=1
        start=[False]*6
        for h in range(5):
            cnf=encoder.encode(start,'forward',h)
            result=solve(cnf.dimacs(),'h6-'+str(h));counts['h6']+=1
            expected=[p for p in reference.enumerate_paths(start,'forward',h) if p['cost']==h]
            assert result['status']==('SAT' if expected else 'UNSAT')
            if expected:
                assignment={int(k):v for k,v in result['assignment'].items()}
                trace=encoder.decode(cnf,assignment,h)
                assert reference.replay(start,'forward',trace,h)['accepted']
                counts['replayed']+=1
        request={'model':{'fields':{'x':['a','b']},'actions':[{'id':'ab','guard':{'x':'a'},'effect':{'x':'b'},'frame':[]}]},'query':{'profile':'categorical-simple-first-hit/v1','start':{'x':'a'},'goal':{'x':'b'},'horizon':1}}
        result=solve(json.dumps(request),'finite-cli','solve-finite','json')
        assert result['status']=='SAT' and result['model']['checked']
        results['lab']=counts|{'status':'PASS','finite_cli':'PASS'}
        # Strictly new typed/copy models are deliberately not coerced into v1.
        invalid={'model':{'fields':{'x':[False,True]},'actions':[]},'query':{'profile':'categorical-simple-first-hit/v1','start':{'x':False},'goal':{'x':True},'horizon':1}}
        p=subprocess.run([str(binary),'encode'],input=json.dumps(invalid).encode(),env=env,capture_output=True)
        assert p.returncode!=0;results['lab']['typed_input_rejected']=True
        (out/'results.json').write_text(json.dumps(results,indent=2)+'\n')
    finally:
        (out/'source-manifest.json').write_text(json.dumps(manifest,indent=2)+'\n')
        (out/'progress.json').write_text(json.dumps(results,indent=2)+'\n')
    print(json.dumps(results,indent=2))

if __name__=='__main__':
    main()
