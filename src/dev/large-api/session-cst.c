//go:build ignore

/* Development-only large full-CST page session (#362 scope 6). One process parses once and
 * answers consecutive page commands with a digest of the canonical CST rows of that page; the
 * last page carries the complete footer. The row layout is the contract in
 * docs/specs/instrumented-query-reference.md; the Go side recomputes the same rows from the
 * fixture template without reading this output. */
#define main protocol_main
#include "driver/driver.c"
#undef main

typedef struct {
  TSTreeCursor cursor;
  uint32_t *path;
  uint32_t cap, depth, next, max_depth;
  bool has_next;
} RowWalk;

static void be32(uint8_t *p, uint32_t v) { p[0] = (uint8_t)(v >> 24); p[1] = (uint8_t)(v >> 16); p[2] = (uint8_t)(v >> 8); p[3] = (uint8_t)v; }

/* Encodes the current node as one canonical row and advances the cursor to the next preorder node. */
static uint32_t next_row(RowWalk *w, uint8_t *row, size_t room) {
  TSNode n = ts_tree_cursor_current_node(&w->cursor);
  if (w->depth > w->cap) { w->cap = w->cap ? w->cap * 2 : 64; w->path = t_realloc(w->path, w->cap * sizeof *w->path); }
  w->path[w->depth - 1] = w->next;
  if (w->depth > w->max_depth) w->max_depth = w->depth;
  const char *type = ts_node_type(n), *field = ts_tree_cursor_current_field_name(&w->cursor);
  size_t tl = strlen(type), fl = field ? strlen(field) : 0;
  if (tl > 0xffff || fl > 0xffff || 4 * 8 + 1 + 2 + tl + 2 + fl > room) fatal_limit("CST_ROW_TOO_LARGE");
  TSPoint a = ts_node_start_point(n), e = ts_node_end_point(n);
  uint8_t *p = row;
  be32(p, w->next); p += 4;
  be32(p, w->depth > 1 ? w->path[w->depth - 2] : 0xffffffffu); p += 4;
  be32(p, ts_node_start_byte(n)); p += 4;
  be32(p, ts_node_end_byte(n)); p += 4;
  be32(p, a.row); p += 4; be32(p, a.column); p += 4; be32(p, e.row); p += 4; be32(p, e.column); p += 4;
  *p++ = (uint8_t)((ts_node_is_named(n) ? 1 : 0) | (ts_node_is_extra(n) ? 2 : 0) | (ts_node_is_missing(n) ? 4 : 0) | (ts_node_is_error(n) ? 8 : 0) | (ts_node_has_error(n) ? 16 : 0));
  *p++ = (uint8_t)(tl >> 8); *p++ = (uint8_t)tl; memcpy(p, type, tl); p += tl;
  *p++ = (uint8_t)(fl >> 8); *p++ = (uint8_t)fl; if (fl) memcpy(p, field, fl); p += fl;
  w->next++;
  if (ts_tree_cursor_goto_first_child(&w->cursor)) { w->depth++; }
  else {
    w->has_next = false;
    for (;;) {
      if (ts_tree_cursor_goto_next_sibling(&w->cursor)) { w->has_next = true; break; }
      if (!ts_tree_cursor_goto_parent(&w->cursor)) break;
      w->depth--;
    }
  }
  return (uint32_t)(p - row);
}

static bool session_number(const char *text, uint32_t max, uint32_t *value) {
  if (!*text) return false;
  uint64_t n = 0;
  for (; *text; text++) {
    if (*text < '0' || *text > '9') return false;
    n = n * 10 + (unsigned)(*text - '0');
    if (n > max) return false;
  }
  *value = (uint32_t)n;
  return n > 0;
}

int main(int argc, char **argv) {
  uint32_t page_size, expected_nodes;
  if (argc != 4 || strlen(argv[1]) != 32 || !session_number(argv[2], 1000000, &page_size) || !session_number(argv[3], MAX_NODES, &expected_nodes)) return 2;
  if ((expected_nodes + page_size - 1) / page_size > 25) return 2;
  for (const char *p = argv[1]; *p; p++)
    if (!(*p >= '0' && *p <= '9') && !(*p >= 'a' && *p <= 'f')) return 2;
#ifdef _WIN32
  _setmode(_fileno(stdin), _O_BINARY); _setmode(_fileno(stdout), _O_BINARY);
#endif
  ts_set_allocator(t_malloc, t_calloc, t_realloc, t_free);
  const TSLanguage *language = tsgk_language();
  snprintf(producer_json[0], sizeof producer_json[0], "{\"language_version\":%u,\"runtime_language_version\":%u,\"runtime_min_compatible\":%u,\"query\":\"UNSUPPORTED\"}", ts_language_abi_version(language), TREE_SITTER_LANGUAGE_VERSION, TREE_SITTER_MIN_COMPATIBLE_LANGUAGE_VERSION);
  memcpy(producer_json[1], producer_json[0], sizeof producer_json[0]);
  uint8_t header[4];
  if (read_full(header, 4) != 4) return 2;
  uint32_t len = (uint32_t)header[0] << 24 | (uint32_t)header[1] << 16 | (uint32_t)header[2] << 8 | header[3];
  if (len > MAX_FRAME) return 2;
  uint8_t *input = t_malloc((size_t)len + 1);
  if (read_full(input, len) != len) return 2;
  Req r; const char *error = parse_request(input, len, &r);
  if (error || r.rev == 3 || r.enc != ENC_UTF8 || r.nedits || r.nqueries || r.nsteps_ranges || !valid_source(r.enc, r.source, r.source_len)) return 2;
  mem_limit = r.memory_bytes; cur_source_bytes = r.source_len;
  TSParser *parser = ts_parser_new();
  if (!ts_parser_set_language(parser, language)) return 4;
  Src source = {0}; source.s = r.source; source.n = r.source_len;
  uint64_t millis;
  uint32_t parse_count = 1;
  TSTree *tree = parse(parser, NULL, NULL, &source, r.enc, r.parse_ms, &error, &millis, NULL);
  if (!tree) return 3;
  TSNode root = ts_tree_root_node(tree);
  uint32_t actual = ts_node_descendant_count(root);
  RowWalk walk = {ts_tree_cursor_new(root), NULL, 0, 1, 0, 0, true};
  Sha all; sha_init(&all);
  Buf chain = {0}; chain.limit = 25 * 65 + 1;
  uint64_t row_bytes = 0;
  uint32_t start = 0, index = 0;
  uint8_t row[2048];
  while (start < expected_nodes) {
    uint32_t end = start + page_size;
    if (end > expected_nodes) end = expected_nodes;
    if (index) {
      if (read_full(header, 4) != 4 || header[0] || header[1] || header[2] || header[3] != 8) return 2;
      uint8_t command[8];
      if (read_full(command, 8) != 8) return 2;
      uint32_t got_start = (uint32_t)command[0] << 24 | (uint32_t)command[1] << 16 | (uint32_t)command[2] << 8 | command[3];
      uint32_t got_end = (uint32_t)command[4] << 24 | (uint32_t)command[5] << 16 | (uint32_t)command[6] << 8 | command[7];
      if (got_start != start || got_end != end) return 2;
    }
    Buf out = {0}; out.limit = 4096;
    puts_(&out, "{\"schema\":\"tsgk-large-cst-page/r1\",\"session\":"); putstr(&out, argv[1]);
    puts_(&out, ",\"index\":"); putu(&out, index);
    puts_(&out, ",\"parse_count\":"); putu(&out, parse_count);
    if (actual != expected_nodes) {
      /* the plan came from the independent expectation: report the actual count and stop */
      puts_(&out, ",\"nodes\":"); putu(&out, actual);
      puts_(&out, ",\"error\":\"NODE_COUNT_MISMATCH\",\"complete\":false}");
      write_frame(out.data, out.len); buf_free(&out);
      return 1;
    }
    Sha page; sha_init(&page);
    uint64_t page_bytes = 0;
    for (uint32_t i = start; i < end; i++) {
      if (!walk.has_next) return 3;
      uint32_t n = next_row(&walk, row, sizeof row);
      sha_update(&page, row, n); sha_update(&all, row, n);
      page_bytes += n;
    }
    char digest[65];
    sha_hex(&page, digest);
    put(&chain, digest, 64); put(&chain, "\n", 1);
    row_bytes += page_bytes;
    puts_(&out, ",\"start\":"); putu(&out, start);
    puts_(&out, ",\"end\":"); putu(&out, end);
    puts_(&out, ",\"rows\":"); putu(&out, end - start);
    puts_(&out, ",\"row_bytes\":"); putu(&out, page_bytes);
    puts_(&out, ",\"digest\":"); putstr(&out, digest);
    bool last = end == expected_nodes;
    puts_(&out, ",\"session_complete\":"); puts_(&out, last ? "true" : "false");
    if (last) {
      char all_hex[65], chain_hex[65];
      Sha chain_sha; sha_init(&chain_sha); sha_update(&chain_sha, chain.data, chain.len);
      sha_hex(&all, all_hex); sha_hex(&chain_sha, chain_hex);
      puts_(&out, ",\"footer\":{\"schema\":\"tsgk-large-cst-footer/r1\",\"nodes\":"); putu(&out, actual);
      puts_(&out, ",\"pages\":"); putu(&out, index + 1);
      puts_(&out, ",\"row_bytes\":"); putu(&out, row_bytes);
      puts_(&out, ",\"max_depth\":"); putu(&out, walk.max_depth);
      puts_(&out, ",\"all_rows_sha256\":"); putstr(&out, all_hex);
      puts_(&out, ",\"pages_sha256\":"); putstr(&out, chain_hex);
      puts_(&out, ",\"has_error\":"); puts_(&out, ts_node_has_error(root) ? "true" : "false");
      puts_(&out, "}");
    }
    puts_(&out, ",\"complete\":true}");
    if (out.over || chain.over) return 3;
    if (last) {
      ts_tree_cursor_delete(&walk.cursor); t_free(walk.path);
      ts_tree_delete(tree); ts_parser_delete(parser); req_free(&r); t_free(input);
    }
    write_frame(out.data, out.len);
    buf_free(&out);
    start = end; index++;
  }
  if (fgetc(stdin) != EOF || ferror(stdin)) return 2;
  buf_free(&chain);
  return 0;
}
