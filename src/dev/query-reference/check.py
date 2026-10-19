"""Manual owned-grammar controls, separate from kit results and all-route qualification."""
from pathlib import Path
import hashlib, json, os, subprocess
import argparse
import sys
from checks import execution, snapshot, unchanged, unchanged_bytes, read_bounded, file_sha, build_identity, batch
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
identity_bytes = read_bounded(args.identity, 1024**2)
identity = build_identity(identity_bytes)
identity_sha = hashlib.sha256(identity_bytes).hexdigest()
binary_sha = file_sha(binary, 64 * 1024**2)
selected_tree_path = args.identity.parent / 'selected-tree.json'
selected_tree_bytes = read_bounded(selected_tree_path, 4 * 1024**2)
assert hashlib.sha256(selected_tree_bytes).hexdigest() == identity['selected_tree_sha256']
assert binary_sha == identity['binary_sha256']
assert identity['upstream_commit'] == '10eb5ac119eec9cb4267a8e035066473983f5f3a'
for name, digest in identity['inputs'].items():
    assert name in ['Cargo.toml', 'main.rs', 'instrumentation.patch', 'build.py', 'checks.py']
    assert hashlib.sha256((Path(__file__).resolve().parent / name).read_bytes()).hexdigest() == digest
assert set(identity['inputs']) == {'Cargo.toml', 'main.rs', 'instrumentation.patch', 'build.py', 'checks.py'}
control_inputs = {name: file_sha(path) for name, path in [('check.py', Path(__file__)), ('checks.py', Path(__file__).with_name('checks.py')), ('supervisor', runner), ('cc', args.cc), ('python', Path(sys.executable).resolve(strict=True))]}
env = {k: os.environ[k] for k in ['SystemRoot', 'WINDIR', 'COMSPEC', 'PATHEXT'] if k in os.environ}
base = 'C:/Windows/System32;C:/Windows' if os.name == 'nt' else '/usr/bin:/bin'
env.update({'PATH': str(args.cc.parent) + os.pathsep + base, 'TEMP': str(dest / 'tmp'), 'TMP': str(dest / 'tmp'), 'TMPDIR': str(dest / 'tmp')})

def sha(data):
    return hashlib.sha256(data).hexdigest()

def execute(name, path, argv, wall=10, memory=1024 ** 3, expected_exit=0):
    spec = {'Path': str(path), 'Args': list(map(str, argv)), 'Dir': str(dest), 'Env': [k + '=' + v for k, v in sorted(env.items())], 'StdoutBytes': 64 * 1024 ** 2, 'StderrBytes': 1024 ** 2, 'Wall': wall * 10 ** 9, 'Grace': 2 * 10 ** 9, 'Memory': {'Bytes': memory, 'Hard': sys.platform != 'darwin'}, 'CgroupParent': args.cgroup_parent}
    (dest / (name + '-request.json')).write_text(json.dumps(spec), encoding='utf-8', newline='\n')
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
library_sha = file_sha(library, 64 * 1024**2)

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
batch_source = dest / 'batch.txt'
batch_source.write_bytes(b'x;\n1;\n')
interleaved = dest / 'interleaved.scm'
interleaved.write_bytes(b'(source_file (expression_statement (identifier) @x) @statement)\n(identifier) @x')
query_paths = [dest/'structural.scm', dest/'eq.scm', dest/'duplicate-captures.scm', interleaved]
ids, pairs, singles = [], [], []
for i in range(16):
    query_id, query_path = f'b{i}', query_paths[i % len(query_paths)]
    ids.append(query_id)
    pairs.extend([query_id, query_path])
    _, receipt = execute(f'batch-single-{i}', binary, [library, 'tree_sitter_tsgk_plain', batch_source, query_path, query_id, library_sha])
    result = json.loads(receipt['stdout'])
    assert result['tree']['nodes'] == nodes and result['complete'] is True
    singles.append(result)
prefix = ['--batch', library, 'tree_sitter_tsgk_plain', batch_source, library_sha]
_, receipt = execute('batch-sixteen', binary, prefix+pairs, wall=50)
assert batch(receipt['stdout'], ids) == singles
passed.append('batch-sixteen-full-field-single-parity')
_, receipt = execute('batch-one', binary, prefix+pairs[:2])
assert batch(receipt['stdout'], ids[:1]) == singles[:1]
passed.append('batch-one-full-field-single-parity')
invalid = dest / 'invalid.scm'
invalid.write_bytes(b'(')
_, receipt = execute('batch-middle-failure', binary, prefix+['first',query_paths[0],'bad',invalid,'later',query_paths[1]], expected_exit=1)
rows = [json.loads(line) for line in receipt['stdout'].split('\n') if line]
assert len(rows) == 3 and rows[0]['complete'] is True and rows[0]['query']['id'] == 'first'
assert rows[1] == {'schema':'tsgk-instrumented-query-reference/r1','status':'FAILED','code':'QUERY_COMPILE','complete':False}
assert rows[2] == {'schema':'tsgk-instrumented-query-reference-batch/r1','status':'FAILED','complete':False,'queries':['first','bad','later'],'responses_completed':1}
try:
    batch(receipt['stdout'], ['first','bad','later'])
except RuntimeError:
    pass
else:
    raise AssertionError('incomplete batch accepted')
passed.append('batch-middle-failure-no-later-result')
usage = 'USAGE_LIBRARY_SYMBOL_SOURCE_QUERY_QUERY_ID_LIBRARY_SHA256'
oversized = dest / 'oversized.scm'
oversized.write_bytes(b' ' * 65537)
for label, arguments, expected_code in [
        ('zero', prefix, usage), ('pair', prefix+['orphan'], usage),
        ('seventeen', prefix+pairs+['b16',query_paths[0]], usage),
        ('duplicate', prefix+['same',query_paths[0],'same',query_paths[1]], 'IDENTIFIER_INVALID'),
        ('oversized-query', prefix+['q',oversized], 'INPUT_LIMIT')]:
    _, receipt = execute('batch-reject-'+label, binary, arguments, expected_exit=1)
    rows = [json.loads(line) for line in receipt['stdout'].split('\n') if line]
    assert len(rows) == 2 and rows[0]['code'] == expected_code and rows[0]['complete'] is False
    assert rows[-1]['complete'] is False and rows[-1]['responses_completed'] == 0
    passed.append('batch-reject-'+label)
invalid_utf8 = dest / 'invalid-utf8.scm'
invalid_utf8.write_bytes(b'(identifier) @\xff')
for label, bad_path, expected_code in [('missing', dest/'missing.scm', 'INPUT_STAT'), ('oversized', oversized, 'INPUT_LIMIT'), ('utf8', invalid_utf8, 'QUERY_UTF8')]:
    _, receipt = execute('batch-later-input-'+label, binary, prefix+['first',query_paths[0],'bad',bad_path,'later',query_paths[1]], expected_exit=1)
    rows = [json.loads(line) for line in receipt['stdout'].split('\n') if line]
    assert len(rows) == 3 and rows[0]['query']['id'] == 'first' and dict(rows[0], query=dict(rows[0]['query'], id='b0')) == singles[0]
    assert rows[1] == {'schema':'tsgk-instrumented-query-reference/r1','status':'FAILED','code':expected_code,'complete':False}
    assert rows[2] == {'schema':'tsgk-instrumented-query-reference-batch/r1','status':'FAILED','complete':False,'queries':['first','bad','later'],'responses_completed':1}
    try:
        batch(receipt['stdout'], ['first','bad','later'])
    except RuntimeError:
        pass
    else:
        raise AssertionError('incomplete batch accepted')
    passed.append('batch-later-input-'+label+'-keeps-prefix')
spec = json.loads((dest/'batch-sixteen-request.json').read_bytes())
spec['StdoutBytes'] = 1024
with (dest/'batch-truncated.json').open('xb') as out, (dest/'batch-truncated.stderr').open('xb') as err:
    process = subprocess.run([str(runner)], input=json.dumps(spec).encode(), env=env, stdout=out, stderr=err, timeout=65)
receipt = json.loads((dest/'batch-truncated.json').read_bytes())
assert process.returncode == 1 and receipt['execution']['stdout_truncated']
assert receipt['execution']['cleanup']['verified'] and receipt['execution']['cleanup']['residual_after_exit'] == 0
for check in (lambda: execution(receipt, 0), lambda: batch(receipt['stdout'], ids)):
    try:
        check()
    except RuntimeError:
        pass
    else:
        raise AssertionError('truncated batch accepted')
passed.append('batch-supervisor-output-limit-cleanup-rejected')
unchanged(grammar, grammar_inputs)
assert file_sha(binary, 64 * 1024**2) == binary_sha and file_sha(library, 64 * 1024**2) == library_sha
unchanged_bytes(args.identity, identity_bytes)
unchanged_bytes(selected_tree_path, selected_tree_bytes)
for name, path in [('check.py', Path(__file__)), ('checks.py', Path(__file__).with_name('checks.py')), ('supervisor', runner), ('cc', args.cc), ('python', Path(sys.executable).resolve(strict=True))]:
    assert file_sha(path) == control_inputs[name]
(dest / 'summary.json').write_text(json.dumps({'kind': 'OWNED_MANUAL_CONTROLS_NOT_ALL_ROUTE_ELIGIBILITY', 'passed': passed, 'binary_sha256': binary_sha, 'library_sha256': library_sha, 'grammar_inputs': grammar_inputs, 'identity_sha256': identity_sha, 'control_inputs': control_inputs}, indent=2), encoding='utf-8')
print(json.dumps({'passed': len(passed), 'kind': 'OWNED_MANUAL_CONTROLS_NOT_ALL_ROUTE_ELIGIBILITY'}))
