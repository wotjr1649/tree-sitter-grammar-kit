/* Fresh-parse baseline linked with the kit build's exact runtime/parser objects. */
#include <tree_sitter/api.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#ifdef _WIN32
#include <windows.h>
#include <fcntl.h>
#include <io.h>
#else
#include <time.h>
#endif

const TSLanguage *tree_sitter_json(void);

static uint64_t now_ns(void) {
#ifdef _WIN32
  LARGE_INTEGER value, frequency;
  if (!QueryPerformanceCounter(&value) || !QueryPerformanceFrequency(&frequency)) exit(2);
  return (uint64_t)((long double)value.QuadPart * 1000000000.0L / frequency.QuadPart);
#else
  struct timespec value;
  if (clock_gettime(CLOCK_MONOTONIC, &value)) exit(2);
  return (uint64_t)value.tv_sec * 1000000000 + (uint64_t)value.tv_nsec;
#endif
}

int main(void) {
#ifdef _WIN32
  if (_setmode(_fileno(stdin), _O_BINARY) == -1) return 2;
#endif
  char source[65537];
  size_t bytes = fread(source, 1, sizeof(source), stdin);
  if (ferror(stdin) || bytes == 0 || bytes > 65536) return 2;
  TSParser *parser = ts_parser_new();
  if (!parser) return 2;
  if (!ts_parser_set_language(parser, tree_sitter_json())) { ts_parser_delete(parser); return 2; }
  uint64_t start = now_ns();
  TSTree *tree = ts_parser_parse_string(parser, NULL, source, (uint32_t)bytes);
  uint64_t elapsed = now_ns() - start;
  if (!tree) { ts_parser_delete(parser); return 2; }
  TSNode root = ts_tree_root_node(tree);
  bool error = ts_node_has_error(root);
  printf("{\"parse_ns\":%llu,\"nodes\":%u,\"end_byte\":%u,\"has_error\":%s}\n",
    (unsigned long long)elapsed, ts_node_descendant_count(root), ts_node_end_byte(root), error ? "true" : "false");
  ts_tree_delete(tree);
  ts_parser_delete(parser);
  return error ? 1 : 0;
}
