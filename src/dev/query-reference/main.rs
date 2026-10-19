use libloading::Library;
use serde_json::{Value, json};
use sha2::{Digest, Sha256};
use std::{
    collections::HashMap,
    env,
    ffi::OsString,
    fs,
    io::{Read, Write},
    ops::ControlFlow,
    path::{Path, PathBuf},
    sync::{Arc, Condvar, Mutex},
    thread,
    time::{Duration, Instant},
};
use tree_sitter::{
    CaptureQuantifier, Node, ParseOptions, Parser, Query, QueryCursor, QueryCursorOptions,
    StreamingIterator, ffi,
};

const UPSTREAM: &str = "10eb5ac119eec9cb4267a8e035066473983f5f3a";
const MAX_NODES: usize = 100_000;
const MAX_CAPTURES: usize = 50_000;
const MAX_GROUP_CAPTURES: usize = 200_000;
const MATCH_LIMIT: u32 = 100_000;
const WALL_MS: u64 = 3_000;
const MAX_QUERIES: usize = 16;

struct Watchdog {
    state: Arc<(Mutex<Option<Instant>>, Condvar)>,
    thread: Option<thread::JoinHandle<()>>,
}

impl Watchdog {
    fn new(deadline: Instant) -> Self {
        let state = Arc::new((Mutex::new(Some(deadline)), Condvar::new()));
        let shared = Arc::clone(&state);
        let thread = thread::spawn(move || {
            let (lock, wake) = &*shared;
            let mut state = lock.lock().unwrap_or_else(|_| std::process::exit(2));
            while let Some(deadline) = *state {
                let now = Instant::now();
                if now >= deadline {
                    std::process::exit(2);
                }
                (state, _) = wake
                    .wait_timeout(state, deadline - now)
                    .unwrap_or_else(|_| std::process::exit(2));
            }
        });
        Self {
            state,
            thread: Some(thread),
        }
    }

    fn check(&self) {
        let state = self
            .state
            .0
            .lock()
            .unwrap_or_else(|_| std::process::exit(2));
        if state.is_none_or(|deadline| Instant::now() >= deadline) {
            std::process::exit(2);
        }
    }

    fn arm(&mut self, deadline: Instant) {
        let mut state = self
            .state
            .0
            .lock()
            .unwrap_or_else(|_| std::process::exit(2));
        let now = Instant::now();
        if state.is_none_or(|old| now >= old) || now >= deadline {
            std::process::exit(2);
        }
        *state = Some(deadline);
        self.state.1.notify_one();
    }
}

impl Drop for Watchdog {
    fn drop(&mut self) {
        *self
            .state
            .0
            .lock()
            .unwrap_or_else(|_| std::process::exit(2)) = None;
        self.state.1.notify_one();
        if let Some(thread) = self.thread.take() {
            if thread.join().is_err() {
                std::process::exit(2);
            }
        }
    }
}

struct MatchGroup {
    index: usize,
    pattern: usize,
    signature: Vec<(u32, usize)>,
    keep: bool,
    seen: Vec<bool>,
}

impl MatchGroup {
    fn observe(&mut self, at: usize) -> Result<(), String> {
        let seen = self.seen.get_mut(at).ok_or("CAPTURE_INDEX")?;
        if *seen {
            return Err("CAPTURE_DUPLICATED".into());
        }
        *seen = true;
        Ok(())
    }

    fn finish(&self) -> Result<(), String> {
        if self.seen.iter().any(|seen| !seen) {
            return Err("MATCH_GROUP_INCOMPLETE".into());
        }
        Ok(())
    }
}

fn hash(bytes: &[u8]) -> String {
    format!("{:x}", Sha256::digest(bytes))
}

fn read(path: &Path, limit: u64) -> Result<Vec<u8>, String> {
    if !path.is_absolute()
        || !fs::symlink_metadata(path)
            .map_err(|_| "INPUT_STAT")?
            .is_file()
    {
        return Err("ABSOLUTE_REGULAR_INPUT_REQUIRED".into());
    }
    let mut bytes = Vec::new();
    fs::File::open(path)
        .map_err(|_| "INPUT_OPEN")?
        .take(limit + 1)
        .read_to_end(&mut bytes)
        .map_err(|_| "INPUT_READ")?;
    if bytes.len() as u64 > limit {
        return Err("INPUT_LIMIT".into());
    }
    Ok(bytes)
}

fn point(p: tree_sitter::Point) -> Value {
    json!({"row":p.row,"column":p.column})
}

fn node_fields(n: Node<'_>) -> Value {
    json!({"type": n.kind(), "named": n.is_named(), "extra": n.is_extra(),
        "is_error": n.is_error(), "has_error": n.has_error(), "is_missing": n.is_missing(),
        "start_byte": n.start_byte(), "end_byte": n.end_byte(),
        "start_point": point(n.start_position()), "end_point": point(n.end_position())})
}

fn capture_doc(
    query: &Query,
    node: Node<'_>,
    capture: u32,
    match_id: usize,
    pattern: usize,
    nodes: &HashMap<usize, usize>,
    tree: &[Value],
) -> Result<Value, String> {
    let index = *nodes.get(&node.id()).ok_or("CAPTURE_NODE_UNMAPPED")?;
    let mut value = node_fields(node);
    for (field, value) in value.as_object().ok_or("NODE_OBJECT")? {
        if tree[index].get(field) != Some(value) {
            return Err("CAPTURE_NODE_FIELDS_DIFFER".into());
        }
    }
    let name = query
        .capture_names()
        .get(capture as usize)
        .ok_or("CAPTURE_ID")?;
    let object = value.as_object_mut().ok_or("NODE_OBJECT")?;
    object.insert("match".into(), json!(match_id));
    object.insert("pattern".into(), json!(pattern));
    object.insert("capture".into(), json!(capture));
    object.insert("name".into(), json!(name));
    object.insert("node".into(), json!(index));
    Ok(value)
}

// These are compiled predicate steps, not a second predicate evaluator.
fn predicates(query: &Query) -> Result<Vec<Value>, String> {
    let mut result = Vec::new();
    let raw = query.tsgk_raw();
    let mut total_steps = 0usize;
    for pattern in 0..query.pattern_count() {
        let mut count = 0u32;
        let ptr = unsafe { ffi::ts_query_predicates_for_pattern(raw, pattern as u32, &mut count) };
        total_steps += count as usize;
        if total_steps > 100_000 {
            return Err("PREDICATE_STEP_LIMIT".into());
        }
        let steps = if count == 0 {
            &[][..]
        } else {
            if ptr.is_null() {
                return Err("PREDICATE_POINTER".into());
            }
            unsafe { std::slice::from_raw_parts(ptr, count as usize) }
        };
        let mut group = Vec::<Value>::new();
        for step in steps {
            match step.type_ {
                ffi::TSQueryPredicateStepTypeDone => {
                    if group.is_empty() {
                        return Err("PREDICATE_EMPTY".into());
                    }
                    let op = group[0]["value"].as_str().ok_or("PREDICATE_OPERATOR")?;
                    if !matches!(op, "eq?" | "not-eq?" | "any-of?" | "not-any-of?") {
                        return Err("PREDICATE_OUTSIDE_REFERENCE_SUBSET".into());
                    }
                    if group.len() < 3 || group[1]["kind"] != "capture" {
                        return Err("PREDICATE_OPERANDS".into());
                    }
                    if matches!(op, "eq?" | "not-eq?") && group.len() != 3 {
                        return Err("PREDICATE_OPERANDS".into());
                    }
                    if matches!(op, "any-of?" | "not-any-of?")
                        && group[2..].iter().any(|s| s["kind"] != "string")
                    {
                        return Err("PREDICATE_OPERANDS".into());
                    }
                    result.push(json!({"pattern": pattern, "steps": group}));
                    group = Vec::new();
                }
                ffi::TSQueryPredicateStepTypeCapture => {
                    let name = query
                        .capture_names()
                        .get(step.value_id as usize)
                        .ok_or("PREDICATE_CAPTURE")?;
                    if query
                        .capture_quantifiers(pattern)
                        .get(step.value_id as usize)
                        != Some(&CaptureQuantifier::One)
                    {
                        return Err("PREDICATE_QUANTIFIER_OUTSIDE_REFERENCE_SUBSET".into());
                    }
                    group.push(json!({"kind":"capture","value":name,"quantifier":"ONE"}));
                }
                ffi::TSQueryPredicateStepTypeString => {
                    let mut length = 0u32;
                    let ptr = unsafe {
                        ffi::ts_query_string_value_for_id(raw, step.value_id, &mut length)
                    };
                    if length > 65_536 || (length != 0 && ptr.is_null()) {
                        return Err("PREDICATE_STRING".into());
                    }
                    let bytes = if length == 0 {
                        &[][..]
                    } else {
                        unsafe { std::slice::from_raw_parts(ptr.cast::<u8>(), length as usize) }
                    };
                    let text = std::str::from_utf8(bytes).map_err(|_| "PREDICATE_UTF8")?;
                    group.push(json!({"kind":"string","value":text,"quantifier":null}));
                }
                _ => return Err("PREDICATE_STEP_KIND".into()),
            }
        }
        if !group.is_empty() {
            return Err("PREDICATE_UNTERMINATED".into());
        }
    }
    Ok(result)
}

#[cfg(windows)]
unsafe fn load(path: &Path) -> Result<Library, String> {
    // DLL's own directory plus System32; do not alter process/global DLL search state.
    unsafe { libloading::os::windows::Library::load_with_flags(path, 0x100 | 0x800) }
        .map(Library::from)
        .map_err(|_| "LIBRARY_LOAD".into())
}

#[cfg(not(windows))]
unsafe fn load(path: &Path) -> Result<Library, String> {
    unsafe { Library::new(path) }.map_err(|_| "LIBRARY_LOAD".into())
}

fn requests(args: Vec<OsString>) -> Result<Vec<Vec<OsString>>, String> {
    if args.first().is_some_and(|a| a == "--batch") {
        if args.len() < 7 || (args.len() - 5) % 2 != 0 || (args.len() - 5) / 2 > MAX_QUERIES {
            return Err("USAGE_LIBRARY_SYMBOL_SOURCE_QUERY_QUERY_ID_LIBRARY_SHA256".into());
        }
        return Ok(args[5..]
            .chunks_exact(2)
            .map(|q| {
                vec![
                    args[1].clone(),
                    args[2].clone(),
                    args[3].clone(),
                    q[1].clone(),
                    q[0].clone(),
                    args[4].clone(),
                ]
            })
            .collect());
    }
    if args.len() != 6 {
        return Err("USAGE_LIBRARY_SYMBOL_SOURCE_QUERY_QUERY_ID_LIBRARY_SHA256".into());
    }
    Ok(vec![args])
}

fn query_deadline(started: Instant, common: Duration, count: usize, now: Instant) -> Instant {
    let wall = Duration::from_millis(WALL_MS);
    if count == 1 {
        started + wall
    } else {
        (now + wall.saturating_sub(common)).min(started + wall * count as u32)
    }
}

fn run(
    requests: &[Vec<OsString>],
    emit: &mut impl FnMut(Value),
    completed: &mut usize,
    ids: &mut Vec<String>,
    watchdog: &mut Option<Watchdog>,
) -> Result<(), String> {
    let args = &requests[0];
    let library_path = PathBuf::from(&args[0]);
    let symbol = args[1].to_str().ok_or("SYMBOL_UTF8")?;
    if !symbol.starts_with("tree_sitter_")
        || symbol.len() > 128
        || !symbol
            .bytes()
            .all(|b| b.is_ascii_alphanumeric() || b == b'_')
    {
        return Err("IDENTIFIER_INVALID".into());
    }
    for request in requests {
        let id = request[4].to_str().ok_or("QUERY_ID_UTF8")?;
        if id.is_empty()
            || id.len() > 128
            || !id
                .bytes()
                .all(|b| b.is_ascii_alphanumeric() || b"._-".contains(&b))
            || ids.iter().any(|prior| prior == id)
        {
            return Err("IDENTIFIER_INVALID".into());
        }
        ids.push(id.to_owned());
    }
    let source = read(Path::new(&args[2]), 32 * 1024 * 1024)?;
    let grammar = read(&library_path, 64 * 1024 * 1024)?;
    let expected_library = args[5].to_str().ok_or("LIBRARY_SHA256_UTF8")?;
    if expected_library.len() != 64
        || !expected_library
            .bytes()
            .all(|b| b.is_ascii_digit() || (b'a'..=b'f').contains(&b))
        || hash(&grammar) != expected_library
    {
        return Err("LIBRARY_SHA256_MISMATCH".into());
    }
    std::str::from_utf8(&source).map_err(|_| "SOURCE_UTF8")?;
    let started = Instant::now();
    let deadline = started + Duration::from_millis(WALL_MS);
    // The caller keeps the guard armed through failure output and the footer.
    let watchdog = watchdog.insert(Watchdog::new(deadline));
    // The handle is declared first and outlives all grammar-dependent objects.
    let library = unsafe { load(&library_path)? };
    let constructor =
        unsafe { library.get::<unsafe extern "C" fn() -> *const ()>(symbol.as_bytes()) }
            .map_err(|_| "LANGUAGE_SYMBOL")?;
    if unsafe { constructor() }.is_null() {
        return Err("LANGUAGE_NULL".into());
    }
    let language = tree_sitter::Language::new(unsafe {
        tree_sitter_language::LanguageFn::from_raw(*constructor)
    });
    let mut parser = Parser::new();
    parser.set_language(&language).map_err(|_| "LANGUAGE_ABI")?;
    let mut parse_limited = false;
    let tree = {
        let mut progress = |_: &tree_sitter::ParseState| {
            if Instant::now() >= deadline {
                parse_limited = true;
                ControlFlow::Break(())
            } else {
                ControlFlow::Continue(())
            }
        };
        parser.parse_with_options(
            &mut |offset, _| &source[offset..],
            None,
            Some(ParseOptions::new().progress_callback(&mut progress)),
        )
    }
    .ok_or("PARSE_INCOMPLETE")?;
    if parse_limited || Instant::now() >= deadline {
        return Err("PARSE_TIME_LIMIT".into());
    }
    let mut nodes = Vec::<Value>::new();
    let mut indices = HashMap::new();
    let mut parents = Vec::<usize>::new();
    let mut cursor = tree.walk();
    'walk: loop {
        if nodes.len() >= MAX_NODES || Instant::now() >= deadline {
            return Err("CST_LIMIT".into());
        }
        let node = cursor.node();
        let index = nodes.len();
        if indices.insert(node.id(), index).is_some() {
            return Err("CST_NODE_ID_DUPLICATE".into());
        }
        let mut value = node_fields(node);
        value["parent"] = json!(parents.last().map_or(-1, |i| *i as i64));
        value["field"] = json!(cursor.field_name());
        nodes.push(value);
        if cursor.goto_first_child() {
            parents.push(index);
            continue;
        }
        loop {
            if cursor.goto_next_sibling() {
                break;
            }
            if !cursor.goto_parent() {
                break 'walk;
            }
            parents.pop();
        }
    }
    let common_elapsed = started.elapsed();
    for (request, query_id) in requests.iter().zip(ids.iter()) {
        // Charge common parsing and CST export to every query's original wall budget.
        let remaining = Duration::from_millis(WALL_MS).saturating_sub(common_elapsed);
        let deadline = query_deadline(started, common_elapsed, requests.len(), Instant::now());
        if remaining.is_zero() || Instant::now() >= deadline {
            return Err("QUERY_INCOMPLETE".into());
        }
        watchdog.arm(deadline);
        let query_bytes = read(Path::new(&request[3]), 65_536)?;
        let query_source = std::str::from_utf8(&query_bytes).map_err(|_| "QUERY_UTF8")?;
        let query = Query::new(&language, query_source).map_err(|_| "QUERY_COMPILE")?;
        let metadata = predicates(&query)?;
        let mut query_cursor = QueryCursor::new();
        query_cursor.set_match_limit(MATCH_LIMIT);
        let mut raw = Vec::<Value>::new();
        let mut evaluated = Vec::<Value>::new();
        let mut match_ids = Vec::<Value>::new();
        let mut groups = HashMap::<u32, MatchGroup>::new();
        let mut group_captures = 0usize;
        let mut query_limited = false;
        {
            let mut progress = |_: &tree_sitter::QueryCursorState| {
                if Instant::now() >= deadline {
                    query_limited = true;
                    ControlFlow::Break(())
                } else {
                    ControlFlow::Continue(())
                }
            };
            let mut captures = query_cursor.captures_raw_with_options(
                &query,
                tree.root_node(),
                source.as_slice(),
                QueryCursorOptions::new().progress_callback(&mut progress),
            );
            let mut buffer1 = Vec::new();
            let mut buffer2 = Vec::new();
            let mut text = source.as_slice();
            while let Some((m, at)) = captures.next() {
                if raw.len() >= MAX_CAPTURES || Instant::now() >= deadline {
                    return Err("CAPTURE_LIMIT".into());
                }
                if m.captures().len() > MAX_GROUP_CAPTURES {
                    return Err("MATCH_GROUP_LIMIT".into());
                }
                let signature = m
                    .captures()
                    .iter()
                    .map(|c| {
                        indices
                            .get(&c.node.id())
                            .map(|index| (c.index, *index))
                            .ok_or("CAPTURE_NODE_UNMAPPED".to_string())
                    })
                    .collect::<Result<Vec<_>, _>>()?;
                // Evaluate the original complete QueryMatch before advancing the raw cursor.
                let keep =
                    m.satisfies_text_predicates(&query, &mut buffer1, &mut buffer2, &mut text);
                let match_id = if let Some(group) = groups.get_mut(&m.id()) {
                    if group.pattern != m.pattern_index
                        || group.signature != signature
                        || group.keep != keep
                    {
                        return Err("MATCH_GROUP_CHANGED".into());
                    }
                    group.observe(*at)?;
                    group.index
                } else {
                    group_captures += signature.len();
                    if groups.len() >= MATCH_LIMIT as usize || group_captures > MAX_GROUP_CAPTURES {
                        return Err("MATCH_GROUP_LIMIT".into());
                    }
                    let index = groups.len();
                    let mut group = MatchGroup {
                        index,
                        pattern: m.pattern_index,
                        seen: vec![false; signature.len()],
                        signature,
                        keep,
                    };
                    group.observe(*at)?;
                    groups.insert(m.id(), group);
                    match_ids.push(json!({"match":index,"runtime_id":m.id()}));
                    index
                };
                let c = m.captures().get(*at).ok_or("CAPTURE_INDEX")?;
                let value = capture_doc(
                    &query,
                    c.node,
                    c.index,
                    match_id,
                    m.pattern_index,
                    &indices,
                    &nodes,
                )?;
                if keep {
                    evaluated.push(value.clone());
                }
                raw.push(value);
            }
        }
        if query_limited || query_cursor.did_exceed_match_limit() || Instant::now() >= deadline {
            return Err("QUERY_INCOMPLETE".into());
        }
        for group in groups.values() {
            group.finish()?;
        }
        if hash(&read(&library_path, 64 * 1024 * 1024)?) != hash(&grammar) {
            return Err("LIBRARY_CHANGED".into());
        }
        let response = json!({"schema":"tsgk-instrumented-query-reference/r1", "status":"COMPLETED", "code":"", "upstream_commit":UPSTREAM,
        "input":{"encoding":"UTF-8","bytes":source.len(),"sha256":hash(&source)},
        "grammar":{"library_sha256":hash(&grammar),"symbol":symbol},
        "tree":{"nodes":nodes}, "query":{"id":query_id,"sha256":hash(&query_bytes),"patterns":query.pattern_count(),"capture_names":query.capture_names(),
            "predicates":metadata,"matches":groups.len(),"match_mapping":match_ids,"structural":raw,"evaluated":evaluated},
        "limits":{"nodes":MAX_NODES,"captures":MAX_CAPTURES,"group_captures":MAX_GROUP_CAPTURES,"matches":MATCH_LIMIT,"wall_ms":WALL_MS},
        "cursor_exhausted":true,"did_exceed_match_limit":false,"complete":true});
        watchdog.check();
        emit(response);
        watchdog.check();
        *completed += 1;
    }
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn watchdog_exits_during_blocking_work_and_expired_rearm() {
        if let Ok(mode) = env::var("TSGK_REFERENCE_WATCHDOG_TEST") {
            if mode == "failed" {
                let dir = env::temp_dir().join(format!("tsgk-watchdog-{}", std::process::id()));
                fs::create_dir_all(&dir).unwrap();
                let library = dir.join("library.bin");
                fs::write(&library, b"not a library").unwrap();
                fs::write(dir.join("source.txt"), b"x").unwrap();
                fs::write(dir.join("query.scm"), b"(x) @x").unwrap();
                let request = [
                    library.as_os_str(),
                    "tree_sitter_x".as_ref(),
                    dir.join("source.txt").as_os_str(),
                    dir.join("query.scm").as_os_str(),
                    "q".as_ref(),
                    hash(b"not a library").as_ref(),
                ]
                .map(OsString::from)
                .to_vec();
                let mut slot = None;
                let result = run(&[request], &mut |_| {}, &mut 0, &mut Vec::new(), &mut slot);
                fs::remove_dir_all(&dir).unwrap();
                assert_eq!(result.unwrap_err(), "LIBRARY_LOAD");
                assert!(slot.is_some());
                thread::sleep(Duration::from_millis(WALL_MS + 1_000));
                return;
            }
            let mut watchdog = Watchdog::new(Instant::now() + Duration::from_millis(50));
            if mode == "delayed" {
                watchdog.arm(Instant::now() + Duration::from_secs(2));
                {
                    let _locked = watchdog.state.0.lock().unwrap();
                    thread::sleep(Duration::from_millis(100));
                }
                watchdog.check();
                return;
            }
            if mode == "expired" {
                watchdog.arm(Instant::now() - Duration::from_millis(1));
            }
            thread::sleep(Duration::from_millis(300));
            return;
        }
        for mode in ["blocked", "expired", "delayed", "failed"] {
            let output = std::process::Command::new(env::current_exe().unwrap())
                .args([
                    "--exact",
                    "tests::watchdog_exits_during_blocking_work_and_expired_rearm",
                ])
                .env("TSGK_REFERENCE_WATCHDOG_TEST", mode)
                .output()
                .unwrap();
            assert_eq!(
                output.status.code(),
                Some(if mode == "delayed" { 0 } else { 2 }),
                "{mode}: {output:?}"
            );
        }
        let mut watchdog = Watchdog::new(Instant::now() + Duration::from_secs(5));
        watchdog.arm(Instant::now() + Duration::from_secs(5));
        watchdog.check();
        drop(watchdog);
    }

    #[test]
    fn batch_requests_preserve_order_and_bounds() {
        let values = |args: &[&str]| -> Vec<OsString> {
            args.iter().map(|arg| OsString::from(*arg)).collect()
        };
        let single = values(&["library", "symbol", "source", "query", "q", "hash"]);
        assert_eq!(requests(single.clone()).unwrap(), vec![single]);
        let batch = requests(values(&[
            "--batch", "library", "symbol", "source", "hash", "q1", "query1", "q2", "query2",
        ]))
        .unwrap();
        assert_eq!(
            batch,
            vec![
                values(&["library", "symbol", "source", "query1", "q1", "hash"]),
                values(&["library", "symbol", "source", "query2", "q2", "hash"])
            ]
        );
        for bad in [
            values(&[]),
            values(&["--batch", "library", "symbol", "source", "hash"]),
            values(&["--batch", "library", "symbol", "source", "hash", "q"]),
        ] {
            assert!(requests(bad).is_err());
        }
        let mut limit = values(&["--batch", "library", "symbol", "source", "hash"]);
        for _ in 0..MAX_QUERIES {
            limit.extend(values(&["q", "query"]));
        }
        assert_eq!(requests(limit.clone()).unwrap().len(), MAX_QUERIES);
        limit.extend(values(&["q", "query"]));
        assert!(requests(limit).is_err());
    }

    #[test]
    fn batch_budget_charges_common_work_and_caps_total_wall() {
        let started = Instant::now();
        let common = Duration::from_secs(1);
        let now = started + common;
        assert_eq!(
            query_deadline(started, common, 1, now),
            started + Duration::from_secs(3)
        );
        assert_eq!(
            query_deadline(started, common, 16, now),
            now + Duration::from_secs(2)
        );
        assert_eq!(
            query_deadline(started, common, 16, started + Duration::from_secs(47)),
            started + Duration::from_secs(48)
        );
        assert!(query_deadline(started, Duration::from_secs(4), 16, now) <= now);
    }

    #[test]
    fn capture_occurrences_are_complete_and_unique() {
        for keep in [true, false] {
            let mut group = MatchGroup {
                index: 0,
                pattern: 0,
                signature: vec![(0, 2), (0, 2)],
                keep,
                seen: vec![false; 2],
            };
            assert_eq!(group.finish().unwrap_err(), "MATCH_GROUP_INCOMPLETE");
            assert!(group.observe(0).is_ok());
            assert_eq!(group.finish().unwrap_err(), "MATCH_GROUP_INCOMPLETE");
            assert_eq!(group.observe(0).unwrap_err(), "CAPTURE_DUPLICATED");
            assert_eq!(group.observe(2).unwrap_err(), "CAPTURE_INDEX");
            assert!(group.observe(1).is_ok());
            assert!(group.finish().is_ok());
        }
    }
}

fn main() {
    let args = env::args_os().skip(1).collect::<Vec<_>>();
    let batch = args.first().is_some_and(|a| a == "--batch");
    let mut total_bytes = 0usize;
    let mut output = std::io::stdout().lock();
    let mut emit = |value| {
        let bytes = serde_json::to_vec(&value).unwrap_or_else(|_| std::process::exit(2));
        if bytes.len() >= 64 * 1024 * 1024 || total_bytes + bytes.len() + 1 > 64 * 1024 * 1024 {
            std::process::exit(2);
        }
        total_bytes += bytes.len() + 1;
        if output
            .write_all(&bytes)
            .and_then(|_| output.write_all(b"\n"))
            .is_err()
        {
            std::process::exit(2);
        }
    };
    let mut completed = 0;
    let mut ids = Vec::new();
    let mut watchdog = None;
    let result = requests(args).and_then(|requests| {
        run(
            &requests,
            &mut emit,
            &mut completed,
            &mut ids,
            &mut watchdog,
        )
    });
    if let Err(code) = &result {
        emit(
            json!({"schema":"tsgk-instrumented-query-reference/r1","status":"FAILED","code":code,"complete":false}),
        );
    }
    if batch {
        emit(
            json!({"schema":"tsgk-instrumented-query-reference-batch/r1","status":if result.is_ok() {"COMPLETED"} else {"FAILED"},
            "complete":result.is_ok(),"queries":ids,"responses_completed":completed}),
        );
    }
    if output.flush().is_err() {
        std::process::exit(2);
    }
    if let Some(watchdog) = &watchdog {
        watchdog.check();
    }
    std::process::exit(if result.is_ok() { 0 } else { 1 });
}
