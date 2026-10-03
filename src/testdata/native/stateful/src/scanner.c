/* Owned stateful external scanner for the tsgk_stateful fixture (Session 05).
 *
 * A heredoc `<<TAG` stores TAG; each following line is a heredoc_line until a line equal
 * to TAG, which is heredoc_end. The delimiter is the scanner state and must survive
 * serialize/deserialize, which the runtime calls around every external scan.
 *
 * TSGK_SCANNER_FAULT builds the deliberate serialization defect used by the kit's tests:
 * serialize writes nothing and deserialize of zero bytes keeps the in-memory delimiter.
 * A sequential (fresh) parse still works, but an incremental reparse that resumes inside a
 * heredoc sees the delimiter left over from the previous parse.
 */
#include <string.h>
#include "tree_sitter/alloc.h"
#include "tree_sitter/parser.h"

enum { HEREDOC_START, HEREDOC_LINE, HEREDOC_END };
#define DELIM_MAX 32

typedef struct {
  unsigned len;
  char delim[DELIM_MAX];
} State;

void *tree_sitter_tsgk_stateful_external_scanner_create(void) { return ts_calloc(1, sizeof(State)); }

void tree_sitter_tsgk_stateful_external_scanner_destroy(void *payload) { ts_free(payload); }

unsigned tree_sitter_tsgk_stateful_external_scanner_serialize(void *payload, char *buffer) {
#ifdef TSGK_SCANNER_FAULT
  (void)payload;
  (void)buffer;
  return 0;
#else
  State *s = payload;
  memcpy(buffer, s->delim, s->len);
  return s->len;
#endif
}

void tree_sitter_tsgk_stateful_external_scanner_deserialize(void *payload, const char *buffer, unsigned length) {
  State *s = payload;
#ifdef TSGK_SCANNER_FAULT
  if (length == 0) return;
#endif
  s->len = length <= DELIM_MAX ? length : 0;
  if (s->len) memcpy(s->delim, buffer, s->len);
}

bool tree_sitter_tsgk_stateful_external_scanner_scan(void *payload, TSLexer *lexer, const bool *valid) {
  State *s = payload;
  if (s->len && (valid[HEREDOC_LINE] || valid[HEREDOC_END])) {
    while (lexer->lookahead == '\n' || lexer->lookahead == '\r') lexer->advance(lexer, true);
    if (lexer->eof(lexer)) return false;
    char line[DELIM_MAX];
    unsigned n = 0;
    bool fits = true;
    while (lexer->lookahead != '\n' && lexer->lookahead != '\r' && !lexer->eof(lexer)) {
      if (n < DELIM_MAX) line[n++] = (char)lexer->lookahead;
      else fits = false;
      lexer->advance(lexer, false);
    }
    lexer->mark_end(lexer);
    if (fits && n == s->len && !memcmp(line, s->delim, n) && valid[HEREDOC_END]) {
      s->len = 0;
      lexer->result_symbol = HEREDOC_END;
      return true;
    }
    if (!valid[HEREDOC_LINE]) return false;
    lexer->result_symbol = HEREDOC_LINE;
    return true;
  }
  if (!s->len && valid[HEREDOC_START]) {
    while (lexer->lookahead == ' ' || lexer->lookahead == '\t' || lexer->lookahead == '\n' || lexer->lookahead == '\r') {
      lexer->advance(lexer, true);
    }
    if (lexer->lookahead != '<') return false;
    lexer->advance(lexer, false);
    if (lexer->lookahead != '<') return false;
    lexer->advance(lexer, false);
    char delim[DELIM_MAX];
    unsigned n = 0;
    while (lexer->lookahead >= 'A' && lexer->lookahead <= 'Z' && n < DELIM_MAX) {
      delim[n++] = (char)lexer->lookahead;
      lexer->advance(lexer, false);
    }
    if (!n) return false;
    memcpy(s->delim, delim, n);
    s->len = n;
    lexer->mark_end(lexer);
    lexer->result_symbol = HEREDOC_START;
    return true;
  }
  return false;
}
