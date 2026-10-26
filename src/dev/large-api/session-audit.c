//go:build ignore

/* Evaluation-only session; the required per-page producer remains the reference. */
#define TSGK_AUDIT_LIBRARY
#include "audit.c"

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
  if (argc != 4 || strlen(argv[1]) != 32 ||
      !session_number(argv[2], 1000000, &page_size) ||
      !session_number(argv[3], MAX_NODES, &expected_nodes)) return 2;
  if ((expected_nodes + page_size - 1) / page_size > 25) return 2;
  for (const char *p = argv[1]; *p; p++)
    if (!(*p >= '0' && *p <= '9') && !(*p >= 'a' && *p <= 'f')) return 2;
#ifdef _WIN32
  _setmode(_fileno(stdin), _O_BINARY); _setmode(_fileno(stdout), _O_BINARY);
#endif
  ts_set_allocator(t_malloc, t_calloc, t_realloc, t_free);
  const TSLanguage *language = tsgk_language();
  snprintf(producer_json[0],sizeof producer_json[0],"{\"language_version\":%u,\"runtime_language_version\":%u,\"runtime_min_compatible\":%u,\"query\":\"UNSUPPORTED\"}",ts_language_abi_version(language),TREE_SITTER_LANGUAGE_VERSION,TREE_SITTER_MIN_COMPATIBLE_LANGUAGE_VERSION);
  memcpy(producer_json[1],producer_json[0],sizeof producer_json[0]);
  uint8_t header[4];
  if (read_full(header,4) != 4) return 2;
  uint32_t len = (uint32_t)header[0]<<24 | (uint32_t)header[1]<<16 | (uint32_t)header[2]<<8 | header[3];
  if (len > MAX_FRAME) return 2;
  uint8_t *input = t_malloc((size_t)len+1);
  if (read_full(input,len) != len) return 2;
  Req r; const char *error = parse_request(input,len,&r);
  if (error || r.rev == 3 || r.enc != ENC_UTF8 || r.nedits || r.nqueries || r.nsteps_ranges || !valid_source(r.enc,r.source,r.source_len)) return 2;
  mem_limit = r.memory_bytes; cur_source_bytes = r.source_len;
  TSParser *parser = ts_parser_new();
  if (!ts_parser_set_language(parser,language)) return 4;
  Src source = {0}; source.s = r.source; source.n = r.source_len;
  uint64_t millis;
  uint32_t parse_count = 0;
  parse_count++;
  TSTree *tree = parse(parser,NULL,NULL,&source,r.enc,r.parse_ms,&error,&millis,NULL);
  if (!tree) return 3;
  if (ts_node_descendant_count(ts_tree_root_node(tree)) != expected_nodes) return 2;
  /* The expected model belongs to this immutable tree; every page still calls the APIs. */
  Audit audit = {0};
  uint32_t start = 0, index = 0;
  while (start < expected_nodes) {
    uint32_t end = start + page_size;
    if (end > expected_nodes) end = expected_nodes;
    if (index) {
      if (read_full(header,4) != 4 || header[0] || header[1] || header[2] || header[3] != 8) return 2;
      uint8_t command[8];
      if (read_full(command,8) != 8) return 2;
      uint32_t got_start = (uint32_t)command[0]<<24 | (uint32_t)command[1]<<16 | (uint32_t)command[2]<<8 | command[3];
      uint32_t got_end = (uint32_t)command[4]<<24 | (uint32_t)command[5]<<16 | (uint32_t)command[6]<<8 | command[7];
      if (got_start != start || got_end != end) return 2;
    }
    uint64_t differences;
    Buf receipt = render_audit(&r,tree,start,end,&differences,&audit);
    Buf out = {0}; out.limit = 4096;
    puts_(&out,"{\"schema\":\"tsgk-large-api-session-page/r1\",\"session\":"); putstr(&out,argv[1]);
    puts_(&out,",\"index\":"); putu(&out,index);
    puts_(&out,",\"parse_count\":"); putu(&out,parse_count);
    puts_(&out,",\"session_complete\":"); puts_(&out,end == expected_nodes ? "true" : "false");
    puts_(&out,",\"receipt\":"); put(&out,receipt.data,receipt.len);
    puts_(&out,",\"complete\":true}");
    if (receipt.over || out.over) return 3;
    buf_free(&receipt);
    if (end == expected_nodes) {
      /* out owns the copied receipt; keep bulk teardown within the final page deadline. */
      idmap_free(&audit.map); t_free(audit.nodes);
      ts_tree_delete(tree); ts_parser_delete(parser); req_free(&r); t_free(input);
    }
    write_frame(out.data,out.len);
    buf_free(&out);
    if (differences) return 1;
    start = end; index++;
  }
  if (fgetc(stdin) != EOF || ferror(stdin)) return 2;
  return 0;
}
