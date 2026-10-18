"""Manual owned-grammar controls, separate from kit results and all-route qualification."""
from pathlib import Path
import hashlib, json, os, subprocess
import argparse
import sys
from checks import execution, snapshot, unchanged, unchanged_bytes
if sys.flags.optimize:
    raise RuntimeError('assertions must be enabled')
parser = argparse.ArgumentParser(description=__doc__)
for name in ['reference', 'supervisor', 'cc', 'identity', 'work']:
    parser.add_argument('--' + name, type=Path, required=True)
parser.add_argument('--cgroup-parent', default='')
args = parser.parse_args()
for name in ['reference', 'supervisor', 'cc', 'identity', 'work']:
    assert getattr(args, name).is_absolute(), name
root = Path(__file__).resolve().parents[3]
dest = args.work
dest.mkdir(exist_ok=False)
(dest / 'tmp').mkdir()
runner = args.supervisor
binary = args.reference
identity_bytes = args.identity.read_bytes()
identity = json.loads(identity_bytes)
identity_sha = hashlib.sha256(identity_bytes).hexdigest()
binary_sha = hashlib.sha256(binary.read_bytes()).hexdigest()
assert identity['schema'] == 'tsgk-instrumented-reference-build/r1' and identity['complete']
assert binary_sha == identity['binary_sha256']
assert identity['upstream_commit'] == '10eb5ac119eec9cb4267a8e035066473983f5f3a'
for name, digest in identity['inputs'].items():
    assert name in ['Cargo.toml', 'main.rs', 'instrumentation.patch', 'build.py', 'checks.py']
    assert hashlib.sha256((Path(__file__).resolve().parent / name).read_bytes()).hexdigest() == digest
assert set(identity['inputs']) == {'Cargo.toml', 'main.rs', 'instrumentation.patch', 'build.py', 'checks.py'}
control_inputs = {name: hashlib.sha256(path.read_bytes()).hexdigest() for name, path in [('check.py', Path(__file__)), ('checks.py', Path(__file__).with_name('checks.py')), ('supervisor', runner), ('cc', args.cc)]}
env = {k: os.environ[k] for k in ['SystemRoot', 'WINDIR', 'COMSPEC', 'PATHEXT'] if k in os.environ}
base = 'C:/Windows/System32;C:/Windows' if os.name == 'nt' else '/usr/bin:/bin'
env.update({'PATH': str(args.cc.parent) + os.pathsep + base, 'TEMP': str(dest / 'tmp'), 'TMP': str(dest / 'tmp'), 'TMPDIR': str(dest / 'tmp')})

def sha(data):
    return hashlib.sha256(data).hexdigest()

def execute(name, path, argv, wall=10, memory=1024 ** 3, expected_exit=0):
    spec = {'Path': str(path), 'Args': list(map(str, argv)), 'Dir': str(dest), 'Env': [k + '=' + v for k, v in sorted(env.items())], 'StdoutBytes': 64 * 1024 ** 2, 'StderrBytes': 1024 ** 2, 'Wall': wall * 10 ** 9, 'Grace': 2 * 10 ** 9, 'Memory': {'Bytes': memory, 'Hard': sys.platform != 'darwin'}, 'CgroupParent': args.cgroup_parent}
    with (dest / (name + '.json')).open('xb') as out, (dest / (name + '.stderr')).open('xb') as err:
        p = subprocess.run([str(runner)], input=json.dumps(spec).encode(), env=env, stdout=out, stderr=err, timeout=wall + 15)
    receipt = json.loads((dest / (name + '.json')).read_bytes())
    execution(receipt, expected_exit)
    assert p.returncode == (0 if expected_exit == 0 else 1)
    return (p.returncode, receipt)
original_grammar = root / 'src/testdata/native/plain/src'
grammar = dest / 'grammar'
grammar.mkdir()
grammar_inputs = {}
for name in ['parser.c', 'tree_sitter/parser.h', 'tree_sitter/alloc.h', 'tree_sitter/array.h']:
    original = original_grammar / name
    assert original.is_file() and not original.is_symlink()
    data = original.read_bytes()
    assert len(data) <= 64 * 1024**2
    copied = grammar / name
    copied.parent.mkdir(parents=True, exist_ok=True)
    copied.write_bytes(data)
    grammar_inputs[name] = {'bytes': len(data), 'sha256': sha(data)}
assert snapshot(grammar) == grammar_inputs
library = dest / ('plain.dll' if os.name == 'nt' else 'plain.dylib' if sys.platform == 'darwin' else 'plain.so')
code, receipt = execute('compile', args.cc, ['-dynamiclib' if sys.platform == 'darwin' else '-shared', '-fPIC', '-O2', '-I', grammar, grammar / 'parser.c', '-o', library], wall=30)
assert code == 0, receipt
library_sha = sha(library.read_bytes())

def node(kind, parent, start, end, start_point, end_point, named=True):
    return {'type': kind, 'parent': parent, 'field': None, 'named': named, 'extra': False, 'is_error': False, 'has_error': False, 'is_missing': False, 'start_byte': start, 'end_byte': end, 'start_point': dict(zip(['row', 'column'], start_point)), 'end_point': dict(zip(['row', 'column'], end_point))}
nodes = [node('source_file', -1, 0, 6, (0, 0), (2, 0)), node('expression_statement', 0, 0, 2, (0, 0), (0, 2)), node('identifier', 1, 0, 1, (0, 0), (0, 1)), node(';', 1, 1, 2, (0, 1), (0, 2), False), node('expression_statement', 0, 3, 5, (1, 0), (1, 2)), node('number', 4, 3, 4, (1, 0), (1, 1)), node(';', 4, 4, 5, (1, 1), (1, 2), False)]
anchor = {k: v for k, v in nodes[2].items() if k not in ('parent', 'field')}
anchor.update({'match': 0, 'pattern': 0, 'capture': 0, 'name': 'x', 'node': 2})
cases = [('structural', '', [True, True]), ('eq', ' (#eq? @x "x")', [True, False]), ('not-eq', ' (#not-eq? @x "x")', [False, True]), ('any-of', ' (#any-of? @x "x" "z")', [True, False]), ('not-any-of', ' (#not-any-of? @x "x" "z")', [False, True]), ('capture-pair', ' (#eq? @x @x)', [True, True]), ('multiple', ' (#eq? @x "x") (#not-eq? @x "z")', [True, False])]
literal = lambda value: {'kind': 'string', 'value': value, 'quantifier': None}
operand = lambda name: {'kind': 'capture', 'value': name, 'quantifier': 'ONE'}
predicate = lambda op, values: {'pattern': 0, 'steps': [literal(op), operand('x'), *values]}
metadata = {'structural': [], 'eq': [predicate('eq?', [literal('x')])], 'not-eq': [predicate('not-eq?', [literal('x')])], 'any-of': [predicate('any-of?', [literal('x'), literal('z')])], 'not-any-of': [predicate('not-any-of?', [literal('x'), literal('z')])], 'capture-pair': [predicate('eq?', [operand('x')])], 'multiple': [predicate('eq?', [literal('x')]), predicate('not-eq?', [literal('z')])]}
passed = []
for name, clause, keep in cases:
    query = ('((identifier) @x' + clause + ')').encode()
    q = dest / (name + '.scm')
    q.write_bytes(query)
    for step, source in enumerate([b'x;\n1;\n', b'y;\n1;\n']):
        label = f'{name}-{step}'
        s = dest / (label + '.txt')
        s.write_bytes(source)
        code, receipt = execute(label, binary, [library, 'tree_sitter_tsgk_plain', s, q, 'q0', library_sha])
        assert code == 0, (label, receipt)
        r = json.loads(receipt['stdout'])
        assert r['schema'] == 'tsgk-instrumented-query-reference/r1' and r['status'] == 'COMPLETED' and r['complete'] and r['cursor_exhausted'] and (not r['did_exceed_match_limit'])
        assert r['input'] == {'encoding': 'UTF-8', 'bytes': len(source), 'sha256': sha(source)} and r['grammar']['library_sha256'] == library_sha
        assert r['tree']['nodes'] == nodes, (label, 'manual CST mismatch', r['tree'])
        assert r['query']['patterns'] == 1 and r['query']['capture_names'] == ['x'] and (r['query']['sha256'] == sha(query))
        assert r['query']['predicates'] == metadata[name], (label, 'manual metadata mismatch')
        assert r['query']['structural'] == [anchor] and r['query']['evaluated'] == ([anchor] if keep[step] else []), (label, 'manual capture mismatch', r['query'])
        passed.append(label)
for label, source in [('two-captures-kept', b'x;\n1;\n'), ('two-captures-rejected', b'y;\n1;\n')]:
    q = dest / (label + '.scm')
    q.write_bytes(b'((identifier) @x @y (#eq? @x "x"))')
    s = dest / (label + '.txt')
    s.write_bytes(source)
    code, receipt = execute(label, binary, [library, 'tree_sitter_tsgk_plain', s, q, 'q0', library_sha])
    r = json.loads(receipt['stdout'])
    second = dict(anchor, capture=1, name='y')
    assert code == 0 and r['complete'] and (r['tree']['nodes'] == nodes)
    assert r['query']['predicates'] == metadata['eq'] and r['query']['capture_names'] == ['x', 'y']
    assert r['query']['structural'] == [anchor, second] and r['query']['evaluated'] == ([anchor, second] if source.startswith(b'x') else [])
    passed.append(label)
for op in ['eq?', 'not-eq?']:
    for equal in [True, False]:
        label = 'distinct-captures-' + op[:-1] + ('-equal' if equal else '-different')
        q = dest / (label + '.scm')
        q.write_bytes(('(source_file (expression_statement (identifier) @x) (expression_statement (identifier) @y) (#' + op + ' @x @y))').encode())
        s = dest / (label + '.txt')
        s.write_bytes(b'x;\nx;\n' if equal else b'x;\ny;\n')
        code, receipt = execute(label, binary, [library, 'tree_sitter_tsgk_plain', s, q, 'q0', library_sha])
        r = json.loads(receipt['stdout'])
        expected_nodes = [dict(n) for n in nodes]
        expected_nodes[5]['type'] = 'identifier'
        second = dict(anchor, capture=1, name='y', node=5, start_byte=3, end_byte=4, start_point={'row': 1, 'column': 0}, end_point={'row': 1, 'column': 1})
        expected = [anchor, second]
        keep = equal if op == 'eq?' else not equal
        assert code == 0 and r['complete'] and r['tree']['nodes'] == expected_nodes
        assert r['query']['predicates'] == [predicate(op, [operand('y')])] and r['query']['capture_names'] == ['x', 'y']
        assert r['query']['structural'] == expected and r['query']['evaluated'] == (expected if keep else [])
        passed.append(label)
q = dest / 'duplicate-captures.scm'
q.write_bytes(b'(identifier) @x @x')
s = dest / 'duplicate-captures.txt'
s.write_bytes(b'x;\n1;\n')
code, receipt = execute('duplicate-captures', binary, [library, 'tree_sitter_tsgk_plain', s, q, 'q0', library_sha])
r = json.loads(receipt['stdout'])
assert code == 0 and r['complete'] and (r['query']['structural'] == r['query']['evaluated'] == [anchor, anchor])
passed.append('duplicate-captures-preserved')
q = dest / 'empty.scm'
q.write_bytes(b'(source_file) @x')
s = dest / 'empty.txt'
s.write_bytes(b'')
code, receipt = execute('empty', binary, [library, 'tree_sitter_tsgk_plain', s, q, 'q0', library_sha])
r = json.loads(receipt['stdout'])
assert code == 0 and r['tree']['nodes'] == [node('source_file', -1, 0, 0, (0, 0), (0, 0))] and (r['query']['structural'] == r['query']['evaluated'] == [])
passed.append('empty')
q = dest / 'unsupported.scm'
q.write_bytes(b'((identifier) @x (#match? @x "x"))')
code, receipt = execute('unsupported', binary, [library, 'tree_sitter_tsgk_plain', s, q, 'q0', library_sha], expected_exit=1)
r = json.loads(receipt['stdout'])
assert code != 0 and r['status'] == 'FAILED' and (not r['complete']) and (r['code'] == 'PREDICATE_OUTSIDE_REFERENCE_SUBSET')
passed.append('unsupported-rejected')
code, receipt = execute('wrong-library-sha', binary, [library, 'tree_sitter_tsgk_plain', s, q, 'q0', '0' * 64], expected_exit=1)
r = json.loads(receipt['stdout'])
assert code != 0 and r['code'] == 'LIBRARY_SHA256_MISMATCH' and (not r['complete'])
passed.append('wrong-library-sha-rejected')
unchanged(grammar, grammar_inputs)
assert sha(binary.read_bytes()) == binary_sha and sha(library.read_bytes()) == library_sha
unchanged_bytes(args.identity, identity_bytes)
for name, path in [('check.py', Path(__file__)), ('checks.py', Path(__file__).with_name('checks.py')), ('supervisor', runner), ('cc', args.cc)]:
    assert sha(path.read_bytes()) == control_inputs[name]
(dest / 'summary.json').write_text(json.dumps({'kind': 'OWNED_MANUAL_CONTROLS_NOT_ALL_ROUTE_ELIGIBILITY', 'passed': passed, 'binary_sha256': binary_sha, 'library_sha256': library_sha, 'grammar_inputs': grammar_inputs, 'identity_sha256': identity_sha, 'control_inputs': control_inputs}, indent=2), encoding='utf-8')
print(json.dumps({'passed': len(passed), 'kind': 'OWNED_MANUAL_CONTROLS_NOT_ALL_ROUTE_ELIGIBILITY'}))
