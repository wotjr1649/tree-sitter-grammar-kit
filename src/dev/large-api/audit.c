//go:build ignore

/* Development-only whole-tree API audit. Reuse the pinned driver parser,
 * allocator, framing and cursor walk; do not change the product wire protocol. */
#define main protocol_main
#include "driver/driver.c"
#undef main

typedef struct {
  int32_t v[13], last, last_named;
  uint32_t start, end;
  TSFieldId field;
  bool named;
} AuditNode;
typedef struct { AuditNode *nodes; IdMap map; const TSLanguage *lang; } Audit;

static void collect(Walk *w, TSNode node, uint32_t i, int64_t parent, const char *field) {
  Audit *a = w->ctx;
  AuditNode *x = &a->nodes[i];
  x->start = ts_node_start_byte(node); x->end = ts_node_end_byte(node);
  x->named = ts_node_is_named(node);
  x->field = field ? ts_language_field_id_for_name(a->lang, field, (uint32_t)strlen(field)) : 0;
  if (field && !x->field) fatal_limit("FIELD_CATALOG_INVALID");
  for (int k = 0; k < 13; k++) x->v[k] = -1;
  x->v[0] = (int32_t)parent; x->v[7] = x->v[8] = 0; x->v[9] = 1;
  x->v[10] = parent < 0 ? 0 : a->nodes[parent].v[10] + 1;
  x->v[11] = x->v[12] = (int32_t)i; x->last = x->last_named = -1;
  *idmap_slot(&a->map, node.id, true) = i;
  if (parent >= 0) {
    AuditNode *p = &a->nodes[parent];
    x->v[1] = p->last; x->v[3] = p->last_named;
    if (p->last >= 0) a->nodes[p->last].v[2] = (int32_t)i;
    else p->v[5] = (int32_t)i;
    p->last = (int32_t)i; p->v[7]++;
    if (x->named) { if (p->v[6] < 0) p->v[6] = (int32_t)i; p->last_named = (int32_t)i; p->v[8]++; }
  }
}

/* Canonical stream: consecutive equal int32 values become (value, uint32 run),
 * both big endian. Compression changes output cost, never the comparison loops. */
typedef struct { Sha sha; int32_t value; uint32_t run; } Stream;
static void flush_stream(Stream *s) {
  if (!s->run) return;
  uint8_t b[8]; uint32_t v=(uint32_t)s->value, n=s->run;
  for (int k=3;k>=0;k--) {b[k]=(uint8_t)v;v>>=8;b[k+4]=(uint8_t)n;n>>=8;}
  sha_update(&s->sha,b,8);s->run=0;
}
static void push_stream(Stream *s,int64_t value) {
  if (value < INT32_MIN || value > INT32_MAX) fatal_limit("AUDIT_VALUE_RANGE");
  if (s->run && (s->value!=value || s->run==UINT32_MAX)) flush_stream(s);
  s->value=(int32_t)value;s->run++;
}
static bool positional(Audit *a, uint32_t i, int64_t got, int column) {
  bool forward = column == 2 || column == 4, named = column >= 3;
  int link = forward ? 2 : 1, skipped = 0;
  uint32_t b = forward ? a->nodes[i].end : a->nodes[i].start;
  for (int32_t c = a->nodes[i].v[link]; c >= 0; c = a->nodes[c].v[link]) {
    AuditNode *x = &a->nodes[c];
    if (named && !x->named) continue;
    if (c == got) return skipped > 0;
    if (x->start != b || x->end != b) return false;
    skipped++;
  }
  return got == -1 && skipped > 0;
}

typedef struct {
  Stream expected, observed;
  uint64_t checks, differences, divergences, child_fields, field_lookups, points;
  uint32_t first_node, first_column;
  int64_t first_expected, first_observed;
 uint32_t divergence_node, divergence_column;
 int64_t divergence_expected, divergence_observed;
} Checks;
static void check(Checks *c, uint32_t node, uint32_t column, int64_t expected, int64_t observed) {
  push_stream(&c->expected, expected);
  push_stream(&c->observed, observed);
  c->checks++;
  if (expected != observed) {
    if (!c->differences) { c->first_node=node; c->first_column=column; c->first_expected=expected; c->first_observed=observed; }
    c->differences++;
  }
}

static void audit_tree(Req *r, TSTree *tree, Checks *checks, uint32_t *count, uint32_t *depth, uint32_t start, uint32_t *end) {
  const TSLanguage *lang = ts_tree_language(tree);
  TSNode root = ts_tree_root_node(tree);
  /* This count sizes storage; the independent cursor walk must visit exactly it. */
  uint32_t n = ts_node_descendant_count(root);
  if (!n || n > r->nodes) fatal_limit("NODE_LIMIT");
  Audit a = {0}; a.lang = lang;
  a.nodes = t_calloc(n, sizeof *a.nodes); idmap_init(&a.map, n);
  Walk w = {n, r->depth, collect, &a, 0, 0};
  const char *error = walk(root, &w);
  if (error) fatal_limit(error);
  if (w.count != n) fatal_limit("CURSOR_COUNT_MISMATCH");
  *count = n; *depth = w.max_seen;
  if (start >= n || *end <= start) fatal_limit("AUDIT_RANGE_INVALID");
  if (*end > n) *end = n;
  for (uint32_t j = n; j-- > 0;) {
    AuditNode *x = &a.nodes[j];
    if (x->v[0] >= 0) a.nodes[x->v[0]].v[9] += x->v[9];
    int32_t next_named = -1;
    for (int32_t k = x->last; k >= 0; k = a.nodes[k].v[1]) {
      a.nodes[k].v[4] = next_named;
      if (a.nodes[k].named) next_named = k;
    }
  }
  uint32_t nf = ts_language_field_count(lang);
  int32_t *first = t_calloc(nf + 1, sizeof *first);
  TSTreeCursor cursor = ts_tree_cursor_new(root);
  for (uint32_t i = start; i < *end; i++) {
    ts_tree_cursor_goto_descendant(&cursor, i);
    AuditNode *x = &a.nodes[i]; TSNode node = ts_tree_cursor_current_node(&cursor);
    int64_t got[13] = {
      idx_of(&a.map, ts_node_parent(node)), idx_of(&a.map, ts_node_prev_sibling(node)),
      idx_of(&a.map, ts_node_next_sibling(node)), idx_of(&a.map, ts_node_prev_named_sibling(node)),
      idx_of(&a.map, ts_node_next_named_sibling(node)), idx_of(&a.map, ts_node_child(node, 0)),
      idx_of(&a.map, ts_node_named_child(node, 0)), ts_node_child_count(node),
      ts_node_named_child_count(node), ts_node_descendant_count(node), 0, 0, 0
    };
    ts_tree_cursor_goto_descendant(&cursor, i);
    got[10] = ts_tree_cursor_current_depth(&cursor);
    got[11] = ts_tree_cursor_current_descendant_index(&cursor);
    got[12] = idx_of(&a.map, ts_tree_cursor_current_node(&cursor));
#ifdef TSGK_AUDIT_FAULT_PARENT
    if (i == 1) got[0] = -1;
#endif
#ifdef TSGK_AUDIT_FAULT_COUNT
    if (i == 0) got[9]++;
#endif
#ifdef TSGK_AUDIT_FAULT_SIBLING
    if (i == 1) got[2] = 1;
#endif
#ifdef TSGK_AUDIT_FAULT_POSITIVE_SKIP
    int32_t sibling=x->v[2];
    if (sibling>=0 && a.nodes[sibling].end>a.nodes[sibling].start) got[2]=a.nodes[sibling].v[2];
#endif
#ifdef TSGK_AUDIT_FAULT_ZERO_SKIP
    int32_t sibling=x->v[2];
    if (sibling>=0 && a.nodes[sibling].start==x->end && a.nodes[sibling].end==x->end) {
      while(sibling>=0 && a.nodes[sibling].start==x->end && a.nodes[sibling].end==x->end) sibling=a.nodes[sibling].v[2];
      got[2]=sibling;
    }
#endif
#ifdef TSGK_AUDIT_FAULT_CURSOR
    if (i == 1) got[11]++;
#endif
    for (int k = 0; k < 13; k++) {
      if (got[k] != x->v[k] && k >= 1 && k <= 4 && positional(&a, i, got[k], k)) {
        if (!checks->divergences) {checks->divergence_node=i;checks->divergence_column=k;checks->divergence_expected=x->v[k];checks->divergence_observed=got[k];}
        checks->divergences++; got[k] = x->v[k];
      }
      check(checks, i, k, x->v[k], got[k]);
    }
    for (uint32_t f = 0; f <= nf; f++) first[f] = -1;
    uint32_t ordinal = 0;
    for (int32_t child = x->v[5]; child >= 0; child = a.nodes[child].v[2], ordinal++) {
      TSFieldId expected = a.nodes[child].field;
      const char *name = ts_node_field_name_for_child(node, ordinal);
      int64_t observed = name ? ts_language_field_id_for_name(lang, name, (uint32_t)strlen(name)) : 0;
      if (name && !observed) observed = -2;
#ifdef TSGK_AUDIT_FAULT_FIELD_NAME
      if (expected) observed = 0;
#endif
      check(checks, i, 13 + ordinal, expected, observed); checks->child_fields++;
      if (expected && first[expected] < 0) first[expected] = child;
    }
    /* Include absent fields and leaves: an unexpected non-null lookup must fail. */
    for (uint32_t f = 1; f <= nf; f++) {
      const char *name = ts_language_field_name_for_id(lang, (TSFieldId)f);
      int64_t by_id = idx_of(&a.map, ts_node_child_by_field_id(node, (TSFieldId)f));
      int64_t by_name = idx_of(&a.map, ts_node_child_by_field_name(node, name, (uint32_t)strlen(name)));
#ifdef TSGK_AUDIT_FAULT_FIELD_LOOKUP
      if (first[f] >= 0) by_id = -1;
#endif
      check(checks, i, 1000000 + 2*f, first[f], by_id);
      check(checks, i, 1000001 + 2*f, first[f], by_name); checks->field_lookups += 2;
    }
  }
  ts_tree_cursor_delete(&cursor);
  uint32_t points[MAX_POINTS + 2], np = 0;
  points[np++] = 0; points[np++] = r->source_len;
  for (int i = 0; i < r->npoints; i++) points[np++] = r->points[i].byte;
  for (uint32_t k = 0; k < np; k++) {
    uint32_t b = points[k];
    TSNode found[] = {ts_node_descendant_for_byte_range(root,b,b), ts_node_named_descendant_for_byte_range(root,b,b)};
    for (uint32_t named = 0; named < 2; named++) {
      int64_t i = idx_of(&a.map, found[named]);
      bool outside_root = b < a.nodes[0].start || b > a.nodes[0].end;
      bool valid = outside_root ? i == 0 : (i >= 0 && i < n && a.nodes[i].start <= b && b <= a.nodes[i].end && (!named || a.nodes[i].named || i == 0));
#ifdef TSGK_AUDIT_FAULT_POINT
      valid = false;
#endif
      check(checks, k, 2000000 + named, 1, valid); checks->points++;
    }
    int32_t first_child = -1;
    for (int32_t i = a.nodes[0].v[5]; i >= 0; i = a.nodes[i].v[2]) {
      if (a.nodes[i].end > b) { first_child = i; break; }
    }
    check(checks, k, 2000002, first_child, idx_of(&a.map, ts_node_first_child_for_byte(root,b))); checks->points++;
  }
  t_free(first); idmap_free(&a.map); t_free(a.nodes);
}

int main(int argc, char **argv) {
  uint32_t start=0,end=MAX_NODES;
  if (argc!=3) return 2;
  char *tail; unsigned long begin=strtoul(argv[1],&tail,10);if (*tail || begin>MAX_NODES) return 2;
  unsigned long finish=strtoul(argv[2],&tail,10);if (*tail || finish>MAX_NODES || finish<=begin) return 2;
  start=(uint32_t)begin;end=(uint32_t)finish;
#ifdef _WIN32
  _setmode(_fileno(stdin), _O_BINARY); _setmode(_fileno(stdout), _O_BINARY);
#endif
  ts_set_allocator(t_malloc, t_calloc, t_realloc, t_free);
  const TSLanguage *language=tsgk_language();
  snprintf(producer_json[0],sizeof producer_json[0],"{\"language_version\":%u,\"runtime_language_version\":%u,\"runtime_min_compatible\":%u,\"query\":\"UNSUPPORTED\"}",ts_language_abi_version(language),TREE_SITTER_LANGUAGE_VERSION,TREE_SITTER_MIN_COMPATIBLE_LANGUAGE_VERSION);
  memcpy(producer_json[1],producer_json[0],sizeof producer_json[0]);
  uint8_t header[4];
  if (read_full(header,4) != 4) return 2;
  uint32_t len = (uint32_t)header[0]<<24 | (uint32_t)header[1]<<16 | (uint32_t)header[2]<<8 | header[3];
  if (len > MAX_FRAME) return 2;
  uint8_t *input = t_malloc((size_t)len+1);
  if (read_full(input,len) != len || fgetc(stdin) != EOF) return 2;
  Req r; const char *error = parse_request(input,len,&r);
  if (error || r.nedits || r.nqueries || r.nsteps_ranges || r.enc != ENC_UTF8 || !valid_source(r.enc,r.source,r.source_len)) return 2;
  mem_limit = r.memory_bytes; cur_source_bytes = r.source_len;
  TSParser *parser = ts_parser_new();
  if (!ts_parser_set_language(parser,tsgk_language())) return 4;
  Src source = {0}; source.s = r.source; source.n = r.source_len;
  uint64_t millis; TSTree *tree = parse(parser,NULL,&source,r.enc,r.parse_ms,&error,&millis);
  if (!tree) return 3;
  Checks checks = {0}; sha_init(&checks.expected.sha); sha_init(&checks.observed.sha);
  uint32_t count, depth; audit_tree(&r,tree,&checks,&count,&depth,start,&end);
  char expected[65], observed[65], source_sha[65];
  flush_stream(&checks.expected); flush_stream(&checks.observed);
  sha_hex(&checks.expected.sha,expected); sha_hex(&checks.observed.sha,observed);
  Sha source_hash; sha_init(&source_hash); sha_update(&source_hash,r.source,r.source_len); sha_hex(&source_hash,source_sha);
  Buf out = {0}; out.limit = r.output_bytes;
  puts_(&out,"{\"schema\":\"tsgk-large-api-audit/r1\",\"id\":"); putstr(&out,r.id);
  puts_(&out,",\"source_sha256\":"); putstr(&out,source_sha);
  puts_(&out,",\"source_bytes\":"); putu(&out,r.source_len);
  puts_(&out,",\"nodes\":"); putu(&out,count);
  puts_(&out,",\"range_start\":"); putu(&out,start);
  puts_(&out,",\"range_end\":"); putu(&out,end);
  puts_(&out,",\"depth\":"); putu(&out,depth);
  puts_(&out,",\"field_count\":"); putu(&out,ts_language_field_count(ts_tree_language(tree)));
  puts_(&out,",\"checks\":"); putu(&out,checks.checks);
  puts_(&out,",\"child_fields\":"); putu(&out,checks.child_fields);
  puts_(&out,",\"field_lookups\":"); putu(&out,checks.field_lookups);
  puts_(&out,",\"point_checks\":"); putu(&out,checks.points);
  puts_(&out,",\"differences\":"); putu(&out,checks.differences);
  puts_(&out,",\"position_navigation_divergences\":"); putu(&out,checks.divergences);
  puts_(&out,",\"first_position_divergence\":["); putu(&out,checks.divergence_node); put(&out,",",1); putu(&out,checks.divergence_column); put(&out,",",1); puti(&out,checks.divergence_expected); put(&out,",",1); puti(&out,checks.divergence_observed); puts_(&out,"]");
  puts_(&out,",\"first_difference\":["); putu(&out,checks.first_node); put(&out,",",1); putu(&out,checks.first_column); put(&out,",",1); puti(&out,checks.first_expected); put(&out,",",1); puti(&out,checks.first_observed); puts_(&out,"]");
  puts_(&out,",\"expected_sha256\":"); putstr(&out,expected);
  puts_(&out,",\"observed_sha256\":"); putstr(&out,observed);
  puts_(&out,",\"complete\":true}");
  if (out.over) return 3;
  write_frame(out.data,out.len);
  ts_tree_delete(tree); ts_parser_delete(parser); req_free(&r); t_free(input); buf_free(&out);
  return checks.differences ? 1 : 0;
}
