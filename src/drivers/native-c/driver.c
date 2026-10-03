/* tsgk-native generic driver: r1 (Session 05) and r2 (Session 06, queries and API facts).
 *
 * Contract: docs/specs/tree-and-adapter-protocol.md, "S05 구현" and "S06 구현". An r1
 * request gets exactly the r1 response; r2 adds queries, API observations and a capability
 * declaration on the same parse and serialization engine. The kit builds this file
 * as a separate executable together with the pinned Tree-sitter runtime, the grammar's
 * parser and scanner and a generated shim that defines tsgk_language(). It reads
 * length-prefixed canonical JSON requests from stdin and writes one length-prefixed JSON
 * response per request to stdout. It never reads files, environment or arguments other
 * than the mode word, and it uses only public runtime APIs.
 *
 * TSGK_FAULT_* macros build owned fault-control variants used only by the kit's tests to
 * prove that the incremental route checks detect a broken route; the product never sets
 * them (the build closure records every define).
 */
#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <tree_sitter/api.h>
#include "cp949_table.h"
#ifdef _WIN32
#include <fcntl.h>
#include <io.h>
#include <windows.h>
#else
#include <time.h>
#endif

const TSLanguage *tsgk_language(void);

#define PROTOCOL_R1 "tsgk-native/r1"
#define PROTOCOL_R2 "tsgk-native/r2"
#define MAX_FRAME 50331648u
#define MAX_INPUT 33554432u
#define MAX_OUTPUT 16777216u
#define MAX_NODES 25000000u
#define MAX_DEPTH 100000u
#define MAX_PARSE_MS 60000u
#define MAX_MEMORY 8589934592ull /* 8 GiB: real-world-source-r3 on windows/amd64 (C1-REAL-WORLD-SOURCE-WINDOWS-R3) */
#define MAX_ERRORS 1000u
#define MAX_PARTIAL 1000u
#define MAX_EDITS 4
#define MAX_DECLS 64
#define MAX_POINTS 64
#define MAX_RANGES 16
#define MAX_TEXT 128
#define MAX_QUERIES 16
#define MAX_QUERY_BYTES 65536u
#define MAX_MATCHES 1000000u
#define MAX_CAPTURES 1000000u
#define MAX_QUERY_MS 90000u

enum { ENC_UTF8, ENC_UTF16LE, ENC_UTF16BE, ENC_CP949 };
enum { OUT_TREE, OUT_AUTO, OUT_RECORD };

/* ---- allocator hook: every runtime, scanner and driver allocation is counted ---- */

typedef union { size_t n; max_align_t align; } Header;
static uint64_t mem_used, mem_limit = MAX_MEMORY;
static char req_id[MAX_TEXT + 1];
static uint32_t cur_source_bytes, cur_steps_done;
static int req_rev = 1; /* protocol revision of the current request; 1 until r2 is read */
static char producer_json[2][384];
static const char *protocol_name(void) { return req_rev == 2 ? PROTOCOL_R2 : PROTOCOL_R1; }
static const char *producer(void) { return producer_json[req_rev - 1]; }
static void fatal_allocation(void);
static void fatal_limit(const char *code);

static void *t_malloc(size_t n) {
  if (n > (size_t)-1 - sizeof(Header) || (uint64_t)n > mem_limit || mem_used > mem_limit - (uint64_t)n) fatal_allocation();
  Header *h = malloc(sizeof(Header) + n);
  if (!h) fatal_allocation();
  h->n = n;
  mem_used += n;
  return h + 1;
}

static void *t_calloc(size_t count, size_t size) {
  if (size && count > (size_t)-1 / size) fatal_allocation();
  void *p = t_malloc(count * size);
  memset(p, 0, count * size);
  return p;
}

static void *t_realloc(void *p, size_t n) {
  if (!p) return t_malloc(n);
  Header *h = (Header *)p - 1;
  size_t old = h->n;
  if (n > old) {
    uint64_t grow = (uint64_t)(n - old);
    if (n > (size_t)-1 - sizeof(Header) || grow > mem_limit || mem_used > mem_limit - grow) fatal_allocation();
  }
  Header *g = realloc(h, sizeof(Header) + n);
  if (!g) fatal_allocation();
  mem_used = mem_used - old + n;
  g->n = n;
  return g + 1;
}

static void t_free(void *p) {
  if (!p) return;
  Header *h = (Header *)p - 1;
  mem_used -= h->n;
  free(h);
}

static void write_frame(const char *payload, size_t n) {
  unsigned char hdr[4] = {(unsigned char)(n >> 24), (unsigned char)(n >> 16), (unsigned char)(n >> 8), (unsigned char)n};
  fwrite(hdr, 1, 4, stdout);
  fwrite(payload, 1, n, stdout);
  fflush(stdout);
}

/* A limit that leaves no consistent point to continue from ends the process with a typed
 * frame and no steps: an allocation over the request's memory_bytes (or a failed
 * allocation), where runtime state may be inconsistent, and a query the runtime kept
 * running after its cancellation. The frame is formatted in static storage. */
static void fatal_limit(const char *code) {
  static char buf[1024];
  int n = snprintf(buf, sizeof buf,
    "{\"protocol\":\"%s\",\"id\":\"%s\",\"status\":\"RESOURCE_LIMIT\",\"code\":\"%s\",\"producer\":%s,"
    "\"source_bytes\":%u,\"steps_completed\":%u,\"steps\":[],\"complete\":true}",
    protocol_name(), req_id, code, producer(), (unsigned)cur_source_bytes, (unsigned)cur_steps_done);
  if (n > 0 && (size_t)n < sizeof buf) write_frame(buf, (size_t)n);
  _Exit(3);
}
static void fatal_allocation(void) { fatal_limit("ALLOCATION_LIMIT"); }

/* ---- time ---- */

static uint64_t now_ms(void) {
#ifdef _WIN32
  static LARGE_INTEGER freq;
  LARGE_INTEGER c;
  if (!freq.QuadPart) QueryPerformanceFrequency(&freq);
  QueryPerformanceCounter(&c);
  return (uint64_t)(c.QuadPart / freq.QuadPart) * 1000u + (uint64_t)(c.QuadPart % freq.QuadPart) * 1000u / (uint64_t)freq.QuadPart;
#else
  struct timespec ts;
  clock_gettime(CLOCK_MONOTONIC, &ts);
  return (uint64_t)ts.tv_sec * 1000u + (uint64_t)ts.tv_nsec / 1000000u;
#endif
}

/* ---- SHA-256 (FIPS 180-4) ---- */

typedef struct { uint32_t h[8]; uint64_t len; uint8_t buf[64]; uint32_t n; } Sha;
static const uint32_t SHA_K[64] = {
  0x428a2f98, 0x71374491, 0xb5c0fbcf, 0xe9b5dba5, 0x3956c25b, 0x59f111f1, 0x923f82a4, 0xab1c5ed5,
  0xd807aa98, 0x12835b01, 0x243185be, 0x550c7dc3, 0x72be5d74, 0x80deb1fe, 0x9bdc06a7, 0xc19bf174,
  0xe49b69c1, 0xefbe4786, 0x0fc19dc6, 0x240ca1cc, 0x2de92c6f, 0x4a7484aa, 0x5cb0a9dc, 0x76f988da,
  0x983e5152, 0xa831c66d, 0xb00327c8, 0xbf597fc7, 0xc6e00bf3, 0xd5a79147, 0x06ca6351, 0x14292967,
  0x27b70a85, 0x2e1b2138, 0x4d2c6dfc, 0x53380d13, 0x650a7354, 0x766a0abb, 0x81c2c92e, 0x92722c85,
  0xa2bfe8a1, 0xa81a664b, 0xc24b8b70, 0xc76c51a3, 0xd192e819, 0xd6990624, 0xf40e3585, 0x106aa070,
  0x19a4c116, 0x1e376c08, 0x2748774c, 0x34b0bcb5, 0x391c0cb3, 0x4ed8aa4a, 0x5b9cca4f, 0x682e6ff3,
  0x748f82ee, 0x78a5636f, 0x84c87814, 0x8cc70208, 0x90befffa, 0xa4506ceb, 0xbef9a3f7, 0xc67178f2};
#define ROR(x, n) (((x) >> (n)) | ((x) << (32 - (n))))

static void sha_init(Sha *s) {
  static const uint32_t h0[8] = {0x6a09e667, 0xbb67ae85, 0x3c6ef372, 0xa54ff53a, 0x510e527f, 0x9b05688c, 0x1f83d9ab, 0x5be0cd19};
  memcpy(s->h, h0, sizeof h0);
  s->len = 0;
  s->n = 0;
}

static void sha_block(Sha *s, const uint8_t *p) {
  uint32_t w[64], a, b, c, d, e, f, g, h;
  for (int i = 0; i < 16; i++) w[i] = (uint32_t)p[4 * i] << 24 | (uint32_t)p[4 * i + 1] << 16 | (uint32_t)p[4 * i + 2] << 8 | p[4 * i + 3];
  for (int i = 16; i < 64; i++) {
    uint32_t s0 = ROR(w[i - 15], 7) ^ ROR(w[i - 15], 18) ^ (w[i - 15] >> 3);
    uint32_t s1 = ROR(w[i - 2], 17) ^ ROR(w[i - 2], 19) ^ (w[i - 2] >> 10);
    w[i] = w[i - 16] + s0 + w[i - 7] + s1;
  }
  a = s->h[0]; b = s->h[1]; c = s->h[2]; d = s->h[3]; e = s->h[4]; f = s->h[5]; g = s->h[6]; h = s->h[7];
  for (int i = 0; i < 64; i++) {
    uint32_t t1 = h + (ROR(e, 6) ^ ROR(e, 11) ^ ROR(e, 25)) + ((e & f) ^ (~e & g)) + SHA_K[i] + w[i];
    uint32_t t2 = (ROR(a, 2) ^ ROR(a, 13) ^ ROR(a, 22)) + ((a & b) ^ (a & c) ^ (b & c));
    h = g; g = f; f = e; e = d + t1; d = c; c = b; b = a; a = t1 + t2;
  }
  s->h[0] += a; s->h[1] += b; s->h[2] += c; s->h[3] += d; s->h[4] += e; s->h[5] += f; s->h[6] += g; s->h[7] += h;
}

static void sha_update(Sha *s, const void *data, size_t n) {
  const uint8_t *p = data;
  s->len += n;
  while (n) {
    if (s->n == 0 && n >= 64) { sha_block(s, p); p += 64; n -= 64; continue; }
    uint32_t k = 64 - s->n;
    if (k > n) k = (uint32_t)n;
    memcpy(s->buf + s->n, p, k);
    s->n += k; p += k; n -= k;
    if (s->n == 64) { sha_block(s, s->buf); s->n = 0; }
  }
}

static void sha_hex(Sha *s, char out[65]) {
  uint64_t bits = s->len * 8;
  uint8_t pad = 0x80, zero = 0, lenbe[8];
  sha_update(s, &pad, 1);
  while (s->n != 56) sha_update(s, &zero, 1);
  for (int i = 0; i < 8; i++) lenbe[i] = (uint8_t)(bits >> (56 - 8 * i));
  sha_update(s, lenbe, 8);
  for (int i = 0; i < 8; i++) snprintf(out + 8 * i, 9, "%08x", (unsigned)s->h[i]);
}

/* ---- bounded output buffer ---- */

typedef struct { char *data; size_t len, cap, limit; bool over; } Buf;

static void put(Buf *b, const char *s, size_t n) {
  if (b->over || n == 0) return;
  if (n > b->limit - b->len) { b->over = true; return; }
  if (b->len + n > b->cap) {
    size_t c = b->cap ? b->cap : 4096;
    while (c < b->len + n) c *= 2;
    if (c > b->limit) c = b->limit;
    b->data = t_realloc(b->data, c);
    b->cap = c;
  }
  memcpy(b->data + b->len, s, n);
  b->len += n;
}
static void puts_(Buf *b, const char *s) { put(b, s, strlen(s)); }
static void putu(Buf *b, uint64_t v) {
  char t[24];
  int i = 24;
  do { t[--i] = (char)('0' + v % 10); v /= 10; } while (v);
  put(b, t + i, (size_t)(24 - i));
}
static void puti(Buf *b, int64_t v) {
  if (v < 0) { put(b, "-", 1); putu(b, (uint64_t)(-v)); } else putu(b, (uint64_t)v);
}
static void putb(Buf *b, bool v) { puts_(b, v ? "true" : "false"); }
static void putstrn(Buf *b, const char *s, size_t n) {
  static const char hex[] = "0123456789abcdef";
  put(b, "\"", 1);
  const char *run = s;
  for (const unsigned char *p = (const unsigned char *)s; p < (const unsigned char *)s + n; p++) {
    if (*p == '"' || *p == '\\' || *p < 0x20) {
      put(b, run, (size_t)((const char *)p - run));
      if (*p == '"') puts_(b, "\\\"");
      else if (*p == '\\') puts_(b, "\\\\");
      else { char e[6] = {'\\', 'u', '0', '0', hex[*p >> 4], hex[*p & 15]}; put(b, e, 6); }
      run = (const char *)p + 1;
    }
  }
  put(b, run, (size_t)(s + n - run));
  put(b, "\"", 1);
}
static void putstr(Buf *b, const char *s) { putstrn(b, s, strlen(s)); }
static void putpoint(Buf *b, TSPoint p) { put(b, "[", 1); putu(b, p.row); put(b, ",", 1); putu(b, p.column); put(b, "]", 1); }
static void buf_free(Buf *b) { t_free(b->data); memset(b, 0, sizeof *b); }

/* ---- canonical request ---- */

typedef struct { char fact[MAX_TEXT + 1], node[MAX_TEXT + 1], name[MAX_TEXT + 1]; } Decl;
typedef struct { char id[MAX_TEXT + 1]; uint32_t byte; } Point;
typedef struct { uint32_t start, old_end, new_end, old_len, new_len; uint8_t *old, *neu; } Edit;
/* r2 query: source bytes, and after compilation the query or its error */
typedef struct { char id[MAX_TEXT + 1]; uint8_t *src; uint32_t len; TSQuery *q; uint32_t err_offset; TSQueryError err; } Query;
typedef struct {
  char id[MAX_TEXT + 1];
  int rev, enc, out;
  uint64_t input_bytes, nodes, full_nodes, depth, output_bytes, parse_ms, memory_bytes, errors, partial_nodes;
  uint64_t matches, captures, query_ms; /* r2 */
  Query queries[MAX_QUERIES];
  int nqueries;
  bool api;
  Decl decls[MAX_DECLS];
  int ndecls;
  Point points[MAX_POINTS];
  int npoints;
  TSRange ranges[MAX_EDITS + 1][MAX_RANGES];
  int nranges[MAX_EDITS + 1];
  int nsteps_ranges;
  uint8_t *source;
  uint32_t source_len;
  Edit edits[MAX_EDITS];
  int nedits;
} Req;

typedef struct { const uint8_t *p, *end; const char *err; } Cur;

static bool fail(Cur *c, const char *code) {
  if (!c->err) c->err = code;
  return false;
}
static bool lit(Cur *c, const char *s) {
  size_t n = strlen(s);
  if (c->err) return false;
  if ((size_t)(c->end - c->p) < n || memcmp(c->p, s, n)) return fail(c, "REQUEST_MALFORMED");
  c->p += n;
  return true;
}
static bool peek(Cur *c, char ch) { return !c->err && c->p < c->end && *c->p == (uint8_t)ch; }
static uint64_t num(Cur *c) {
  uint64_t v = 0;
  if (c->err) return 0;
  if (c->p >= c->end || *c->p < '0' || *c->p > '9') { fail(c, "REQUEST_MALFORMED"); return 0; }
  if (*c->p == '0' && c->p + 1 < c->end && c->p[1] >= '0' && c->p[1] <= '9') { fail(c, "REQUEST_MALFORMED"); return 0; }
  while (c->p < c->end && *c->p >= '0' && *c->p <= '9') {
    if (v > (UINT64_C(1) << 53) / 10) { fail(c, "REQUEST_MALFORMED"); return 0; }
    v = v * 10 + (uint64_t)(*c->p++ - '0');
  }
  return v;
}
static bool id_char(uint8_t ch) {
  return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '.' || ch == '_' || ch == ':' || ch == '-';
}
/* A quoted ASCII string without escapes; idset restricts it to the id alphabet. */
static bool text(Cur *c, char *out, bool idset) {
  size_t n = 0;
  if (!lit(c, "\"")) return false;
  while (c->p < c->end && *c->p != '"') {
    uint8_t ch = *c->p++;
    if (ch < 0x20 || ch > 0x7e || ch == '\\' || (idset && !id_char(ch)) || n == MAX_TEXT) return fail(c, "REQUEST_MALFORMED");
    out[n++] = (char)ch;
  }
  out[n] = 0;
  if (n == 0) return fail(c, "REQUEST_MALFORMED");
  return lit(c, "\"");
}
static int b64v(uint8_t ch) {
  if (ch >= 'A' && ch <= 'Z') return ch - 'A';
  if (ch >= 'a' && ch <= 'z') return ch - 'a' + 26;
  if (ch >= '0' && ch <= '9') return ch - '0' + 52;
  if (ch == '+') return 62;
  if (ch == '/') return 63;
  return -1;
}
/* Strict padded standard base64 between quotes, decoded into a fresh buffer. */
static uint8_t *b64(Cur *c, uint64_t max, uint32_t *len) {
  if (!lit(c, "\"")) return NULL;
  const uint8_t *s = c->p;
  while (c->p < c->end && *c->p != '"') c->p++;
  size_t n = (size_t)(c->p - s);
  if (!lit(c, "\"")) return NULL;
  if (n % 4) { fail(c, "BASE64_INVALID"); return NULL; }
  size_t pad = n && s[n - 1] == '=' ? (s[n - 2] == '=' ? 2 : 1) : 0;
  size_t out = n / 4 * 3 - pad;
  if (out > max) { fail(c, "INPUT_TOO_LARGE"); return NULL; }
  uint8_t *d = t_malloc(out + 1);
  size_t k = 0;
  for (size_t i = 0; i < n; i += 4) {
    int v[4];
    for (int j = 0; j < 4; j++) {
      v[j] = b64v(s[i + j]);
      bool tail = i + 4 == n && ((j == 3 && pad >= 1) || (j == 2 && pad == 2));
      if (tail) { if (s[i + j] != '=') v[j] = -1; else v[j] = 0; }
      if (v[j] < 0) { t_free(d); fail(c, "BASE64_INVALID"); return NULL; }
    }
    uint32_t w = (uint32_t)v[0] << 18 | (uint32_t)v[1] << 12 | (uint32_t)v[2] << 6 | (uint32_t)v[3];
    /* Non-canonical trailing bits are rejected so each byte string has one encoding. */
    if (i + 4 == n && ((pad == 1 && (w & 0xff)) || (pad == 2 && (w & 0xffff)))) { t_free(d); fail(c, "BASE64_INVALID"); return NULL; }
    uint8_t bytes[3] = {(uint8_t)(w >> 16), (uint8_t)(w >> 8), (uint8_t)w};
    size_t take = i + 4 == n ? 3 - pad : 3;
    memcpy(d + k, bytes, take);
    k += take;
  }
  *len = (uint32_t)out;
  return d;
}

static const char *bounded(uint64_t v, uint64_t max) { return v == 0 || v > max ? "LIMIT_INVALID" : NULL; }

static void req_free(Req *r) {
  t_free(r->source);
  for (int i = 0; i < r->nedits; i++) { t_free(r->edits[i].old); t_free(r->edits[i].neu); }
  for (int i = 0; i < r->nqueries; i++) {
    t_free(r->queries[i].src);
    if (r->queries[i].q) ts_query_delete(r->queries[i].q);
  }
  r->source = NULL;
  r->nedits = 0;
  r->nqueries = 0;
}

static const char *parse_request(const uint8_t *data, size_t n, Req *r) {
  Cur c = {data, data + n, NULL};
  char word[MAX_TEXT + 1];
  memset(r, 0, sizeof *r);
  lit(&c, "{\"protocol\":");
  if (!text(&c, word, false)) return c.err;
  if (!strcmp(word, PROTOCOL_R1)) r->rev = 1;
  else if (!strcmp(word, PROTOCOL_R2)) r->rev = 2;
  else return "PROTOCOL_MISMATCH";
  req_rev = r->rev;
  lit(&c, ",\"id\":");
  if (!text(&c, r->id, true)) return c.err;
  memcpy(req_id, r->id, sizeof req_id);
  lit(&c, ",\"encoding\":");
  if (!text(&c, word, false)) return c.err;
  if (!strcmp(word, "UTF-8")) r->enc = ENC_UTF8;
  else if (!strcmp(word, "UTF-16LE")) r->enc = ENC_UTF16LE;
  else if (!strcmp(word, "UTF-16BE")) r->enc = ENC_UTF16BE;
  else if (!strcmp(word, "CP949")) r->enc = ENC_CP949;
  else return "ENCODING_UNSUPPORTED";
  lit(&c, ",\"output\":");
  if (!text(&c, word, false)) return c.err;
  if (!strcmp(word, "tree")) r->out = OUT_TREE;
  else if (!strcmp(word, "auto")) r->out = OUT_AUTO;
  else if (!strcmp(word, "record")) r->out = OUT_RECORD;
  else return "REQUEST_MALFORMED";
  lit(&c, ",\"limits\":{\"input_bytes\":"); r->input_bytes = num(&c);
  lit(&c, ",\"nodes\":"); r->nodes = num(&c);
  lit(&c, ",\"full_nodes\":"); r->full_nodes = num(&c);
  lit(&c, ",\"depth\":"); r->depth = num(&c);
  lit(&c, ",\"output_bytes\":"); r->output_bytes = num(&c);
  lit(&c, ",\"parse_ms\":"); r->parse_ms = num(&c);
  lit(&c, ",\"memory_bytes\":"); r->memory_bytes = num(&c);
  lit(&c, ",\"errors\":"); r->errors = num(&c);
  lit(&c, ",\"partial_nodes\":"); r->partial_nodes = num(&c);
  if (r->rev == 2) {
    lit(&c, ",\"matches\":"); r->matches = num(&c);
    lit(&c, ",\"captures\":"); r->captures = num(&c);
    lit(&c, ",\"query_ms\":"); r->query_ms = num(&c);
  }
  lit(&c, "}");
  if (c.err) return c.err;
  const char *e = bounded(r->input_bytes, MAX_INPUT);
  if (!e) e = bounded(r->nodes, MAX_NODES);
  if (!e) e = bounded(r->full_nodes, r->nodes);
  if (!e) e = bounded(r->depth, MAX_DEPTH);
  if (!e) e = bounded(r->output_bytes, MAX_OUTPUT);
  if (!e) e = bounded(r->parse_ms, MAX_PARSE_MS);
  if (!e) e = bounded(r->memory_bytes, MAX_MEMORY);
  if (!e) e = bounded(r->errors, MAX_ERRORS);
  if (!e) e = bounded(r->partial_nodes, MAX_PARTIAL);
  if (!e && r->rev == 2) e = bounded(r->matches, MAX_MATCHES);
  if (!e && r->rev == 2) e = bounded(r->captures, MAX_CAPTURES);
  if (!e && r->rev == 2) e = bounded(r->query_ms, MAX_QUERY_MS);
  if (e) return e;
  lit(&c, ",\"declarations\":[");
  while (!c.err && !peek(&c, ']')) {
    if (r->ndecls && !lit(&c, ",")) break;
    if (r->ndecls == MAX_DECLS) return "LIMIT_INVALID";
    Decl *d = &r->decls[r->ndecls++];
    lit(&c, "{\"fact\":"); text(&c, d->fact, false);
    lit(&c, ",\"node\":"); text(&c, d->node, false);
    lit(&c, ",\"name\":"); text(&c, d->name, false);
    lit(&c, "}");
  }
  lit(&c, "],\"points\":[");
  while (!c.err && !peek(&c, ']')) {
    if (r->npoints && !lit(&c, ",")) break;
    if (r->npoints == MAX_POINTS) return "LIMIT_INVALID";
    Point *p = &r->points[r->npoints++];
    lit(&c, "{\"id\":"); text(&c, p->id, true);
    lit(&c, ",\"byte\":");
    uint64_t v = num(&c);
    if (v > UINT32_MAX) return "REQUEST_MALFORMED";
    p->byte = (uint32_t)v;
    lit(&c, "}");
  }
  /* Included ranges per step (SVC-SERVICEHOST-r1 inline code): [] or one list per step. */
  lit(&c, "],\"ranges\":[");
  while (!c.err && !peek(&c, ']')) {
    if (r->nsteps_ranges && !lit(&c, ",")) break;
    if (r->nsteps_ranges == MAX_EDITS + 1) return "RANGES_INVALID";
    int k = r->nsteps_ranges++;
    lit(&c, "[");
    while (!c.err && !peek(&c, ']')) {
      if (r->nranges[k] && !lit(&c, ",")) break;
      if (r->nranges[k] == MAX_RANGES) return "RANGES_INVALID";
      TSRange *g = &r->ranges[k][r->nranges[k]++];
      uint64_t v[6];
      lit(&c, "{\"start_byte\":"); v[0] = num(&c);
      lit(&c, ",\"end_byte\":"); v[1] = num(&c);
      lit(&c, ",\"start_point\":["); v[2] = num(&c);
      lit(&c, ","); v[3] = num(&c);
      lit(&c, "],\"end_point\":["); v[4] = num(&c);
      lit(&c, ","); v[5] = num(&c);
      lit(&c, "]}");
      for (int i = 0; i < 6; i++) if (v[i] > UINT32_MAX) return "RANGES_INVALID";
      *g = (TSRange){{(uint32_t)v[2], (uint32_t)v[3]}, {(uint32_t)v[4], (uint32_t)v[5]}, (uint32_t)v[0], (uint32_t)v[1]};
    }
    lit(&c, "]");
  }
  lit(&c, "],\"source\":");
  if (c.err) return c.err;
  r->source = b64(&c, r->input_bytes, &r->source_len);
  if (c.err) return c.err;
  lit(&c, ",\"edits\":[");
  while (!c.err && !peek(&c, ']')) {
    if (r->nedits && !lit(&c, ",")) break;
    if (r->nedits == MAX_EDITS) return "EDIT_COUNT_LIMIT";
    Edit *ed = &r->edits[r->nedits++];
    uint64_t a, b, d;
    lit(&c, "{\"start_byte\":"); a = num(&c);
    lit(&c, ",\"old_end_byte\":"); b = num(&c);
    lit(&c, ",\"new_end_byte\":"); d = num(&c);
    if (!c.err && (a > UINT32_MAX || b > UINT32_MAX || d > UINT32_MAX)) return "EDIT_RANGE";
    ed->start = (uint32_t)a; ed->old_end = (uint32_t)b; ed->new_end = (uint32_t)d;
    lit(&c, ",\"old\":");
    if (c.err) break;
    ed->old = b64(&c, r->input_bytes, &ed->old_len);
    lit(&c, ",\"new\":");
    if (c.err) break;
    ed->neu = b64(&c, r->input_bytes, &ed->new_len);
    lit(&c, "}");
  }
  lit(&c, "]");
  if (r->rev == 2) {
    /* r2: queries (id, base64 UTF-8 source) and the API-observation switch */
    lit(&c, ",\"queries\":[");
    while (!c.err && !peek(&c, ']')) {
      if (r->nqueries && !lit(&c, ",")) break;
      if (r->nqueries == MAX_QUERIES) return "LIMIT_INVALID";
      Query *q = &r->queries[r->nqueries++];
      lit(&c, "{\"id\":"); text(&c, q->id, true);
      lit(&c, ",\"source\":");
      if (c.err) break;
      q->src = b64(&c, MAX_INPUT, &q->len);
      if (c.err) break;
      if (q->len == 0 || q->len > MAX_QUERY_BYTES) return "QUERY_SOURCE_INVALID";
      for (int i = 0; i + 1 < r->nqueries; i++) if (!strcmp(r->queries[i].id, q->id)) return "QUERY_ID_DUPLICATE";
      lit(&c, "}");
    }
    lit(&c, "],\"api\":");
    if (peek(&c, 't')) { lit(&c, "true"); r->api = true; } else lit(&c, "false");
  }
  lit(&c, "}");
  if (c.err) return c.err;
  if (c.p != c.end) return "REQUEST_MALFORMED";
  for (int i = 0; i < r->ndecls; i++) {
    /* Locator grammar: segments node | field:F | child:T | children:T joined by '/'. */
    const char *s = r->decls[i].name;
    if (!strcmp(s, "node")) continue;
    while (*s) {
      const char *slash = strchr(s, '/');
      size_t len = slash ? (size_t)(slash - s) : strlen(s);
      size_t k = !strncmp(s, "field:", 6) ? 6 : !strncmp(s, "children:", 9) ? 9 : !strncmp(s, "child:", 6) ? 6 : 0;
      if (!k || len <= k || memchr(s, ':', len) != s + k - 1 || memchr(s + k, ':', len - k)) return "LOCATOR_INVALID";
      s += len;
      if (*s == '/') { s++; if (!*s) return "LOCATOR_INVALID"; }
    }
  }
  if (r->nedits && r->out != OUT_TREE) return "EDITS_NOT_ALLOWED";
  return NULL;
}

/* ---- encodings, edit boundaries and points ---- */

static uint32_t cp949_pointer(uint8_t lead, uint8_t trail) { return (uint32_t)(lead - 0x81) * 190u + (uint32_t)(trail - 0x41); }
static bool cp949_pair(uint8_t lead, uint8_t trail) {
  return lead >= 0x81 && lead <= 0xfe && trail >= 0x41 && trail <= 0xfe && tsgk_euc_kr[cp949_pointer(lead, trail)] != 0;
}
static uint32_t unit16(int enc, const uint8_t *s) { return enc == ENC_UTF16LE ? (uint32_t)s[0] | (uint32_t)s[1] << 8 : (uint32_t)s[0] << 8 | s[1]; }

static bool valid_source(int enc, const uint8_t *s, uint32_t n) {
  if (enc == ENC_UTF8) return true;
  if (enc == ENC_CP949) {
    for (uint32_t i = 0; i < n; i++) {
      if (s[i] < 0x80) continue;
      if (i + 1 >= n || !cp949_pair(s[i], s[i + 1])) return false;
      i++;
    }
    return true;
  }
  if (n % 2) return false;
  bool high = false;
  for (uint32_t i = 0; i < n; i += 2) {
    uint32_t u = unit16(enc, s + i);
    if (u == 0) return false;
    if (u >= 0xd800 && u <= 0xdbff) { if (high) return false; high = true; }
    else if (u >= 0xdc00 && u <= 0xdfff) { if (!high) return false; high = false; }
    else if (high) return false;
  }
  return !high;
}

/* Length of the well-formed UTF-8 character at s, or 1 for ASCII and invalid bytes. */
static uint32_t utf8_unit(const uint8_t *s, uint32_t n) {
  uint8_t b = s[0];
  if (b < 0xc2 || b > 0xf4) return 1;
  uint32_t need = b < 0xe0 ? 2 : b < 0xf0 ? 3 : 4;
  if (n < need) return 1;
  uint8_t lo = 0x80, hi = 0xbf;
  if (b == 0xe0) lo = 0xa0;
  else if (b == 0xed) hi = 0x9f;
  else if (b == 0xf0) lo = 0x90;
  else if (b == 0xf4) hi = 0x8f;
  if (s[1] < lo || s[1] > hi) return 1;
  for (uint32_t i = 2; i < need; i++) if (s[i] < 0x80 || s[i] > 0xbf) return 1;
  return need;
}

static const char *boundary(int enc, const uint8_t *s, uint32_t n, uint32_t off) {
  if (enc == ENC_UTF16LE || enc == ENC_UTF16BE) {
    if (off % 2) return "EDIT_ODD_UTF16";
    if (off >= 2 && off + 2 <= n) {
      uint32_t a = unit16(enc, s + off - 2), b = unit16(enc, s + off);
      if (a >= 0xd800 && a <= 0xdbff && b >= 0xdc00 && b <= 0xdfff) return "EDIT_SPLITS_CHARACTER";
    }
    return NULL;
  }
  uint32_t i = 0;
  while (i < off) {
    uint32_t len = enc == ENC_CP949 ? (s[i] >= 0x80 ? 2 : 1) : utf8_unit(s + i, n - i);
    if (i + len > off) return "EDIT_SPLITS_CHARACTER";
    i += len;
  }
  return NULL;
}

static TSPoint point_at(int enc, const uint8_t *s, uint32_t off) {
  uint32_t row = 0, line = 0;
  if (enc == ENC_UTF16LE || enc == ENC_UTF16BE) {
    for (uint32_t i = 0; i + 2 <= off; i += 2) if (unit16(enc, s + i) == 0x0a) { row++; line = i + 2; }
  } else {
    for (uint32_t i = 0; i < off; i++) if (s[i] == '\n') { row++; line = i + 1; }
  }
  return (TSPoint){row, off - line};
}

typedef struct { uint8_t *s; uint32_t n; } Src;

/* Applies every edit to a copy before any parse; returns the first violation. */
static const char *apply_edits(Req *r, Src *v) {
  v[0].s = r->source;
  v[0].n = r->source_len;
  for (int k = 0; k < r->nedits; k++) {
    Edit *e = &r->edits[k];
    Src *cur = &v[k];
    if (e->start > e->old_end || e->old_end > cur->n) return "EDIT_RANGE";
    if (e->old_len != e->old_end - e->start || memcmp(cur->s + e->start, e->old ? e->old : (uint8_t *)"", e->old_len)) return "EDIT_OLD_MISMATCH";
    if ((uint64_t)e->new_end != (uint64_t)e->start + e->new_len) return "EDIT_LENGTH_INCONSISTENT";
    if (!e->old_len && !e->new_len) return "EDIT_EMPTY";
    uint64_t size = (uint64_t)cur->n - e->old_len + e->new_len;
    if (size > r->input_bytes) return "INPUT_TOO_LARGE";
    const char *b = boundary(r->enc, cur->s, cur->n, e->start);
    if (!b) b = boundary(r->enc, cur->s, cur->n, e->old_end);
    if (b) return b;
    Src *nx = &v[k + 1];
    nx->n = (uint32_t)size;
    nx->s = t_malloc(size + 1);
    memcpy(nx->s, cur->s, e->start);
    memcpy(nx->s + e->start, e->neu, e->new_len);
    memcpy(nx->s + e->new_end, cur->s + e->old_end, cur->n - e->old_end);
    if (!valid_source(r->enc, nx->s, nx->n)) return "EDIT_ENCODING_INVALID";
    b = boundary(r->enc, nx->s, nx->n, e->start);
    if (!b) b = boundary(r->enc, nx->s, nx->n, e->new_end);
    if (b) return b;
  }
  return NULL;
}

/* Included ranges: none, or one non-empty ordered list per step inside that step's source
 * with points equal to the driver's own computation. */
static const char *check_ranges(Req *r, Src *v) {
  if (!r->nsteps_ranges) return NULL;
  if (r->nsteps_ranges != r->nedits + 1) return "RANGES_INVALID";
  for (int k = 0; k < r->nsteps_ranges; k++) {
    if (!r->nranges[k]) return "RANGES_INVALID";
    uint32_t prev = 0;
    for (int i = 0; i < r->nranges[k]; i++) {
      TSRange *g = &r->ranges[k][i];
      if (g->start_byte < prev || g->start_byte > g->end_byte || g->end_byte > v[k].n) return "RANGES_INVALID";
      TSPoint a = point_at(r->enc, v[k].s, g->start_byte), b = point_at(r->enc, v[k].s, g->end_byte);
      if (a.row != g->start_point.row || a.column != g->start_point.column || b.row != g->end_point.row || b.column != g->end_point.column)
        return "RANGES_INVALID";
      prev = g->end_byte;
    }
  }
  return NULL;
}

static void set_ranges(TSParser *p, Req *r, int k) {
  if (r->nsteps_ranges) ts_parser_set_included_ranges(p, r->ranges[k], (uint32_t)r->nranges[k]);
}

/* ---- parsing ---- */

static const char *read_input(void *payload, uint32_t byte, TSPoint position, uint32_t *got) {
  (void)position;
  Src *x = payload;
  if (byte >= x->n) { *got = 0; return ""; }
  *got = x->n - byte; /* the whole remaining buffer: a decode never sees a split character */
  return (const char *)x->s + byte;
}

static uint32_t cp949_decode(const uint8_t *s, uint32_t len, int32_t *cp) {
  if (len == 0) { *cp = -1; return 0; }
  if (s[0] < 0x80) { *cp = s[0]; return 1; }
  if (len < 2 || !cp949_pair(s[0], s[1])) { *cp = -1; return 1; }
  *cp = tsgk_euc_kr[cp949_pointer(s[0], s[1])];
  return 2;
}

typedef struct { uint64_t start, limit; bool timed_out; } Clock;
static bool progress(TSParseState *state) {
  Clock *c = state->payload;
  if (now_ms() - c->start > c->limit) { c->timed_out = true; return true; }
  return false;
}

static TSTree *parse(TSParser *p, const TSTree *old, Src *src, int enc, uint64_t limit, const char **code, uint64_t *ms) {
  static const TSInputEncoding map[] = {TSInputEncodingUTF8, TSInputEncodingUTF16LE, TSInputEncodingUTF16BE, TSInputEncodingCustom};
  TSInput in = {src, read_input, map[enc], enc == ENC_CP949 ? cp949_decode : NULL};
  Clock clock = {now_ms(), limit, false};
  TSParseOptions opt = {&clock, progress};
  TSTree *t = ts_parser_parse_with_options(p, old, in, opt);
#ifdef TSGK_FAULT_NULL_TREE
  if (t && old) { ts_tree_delete(t); t = NULL; } /* owned fault: a null tree without cancellation */
#endif
  *ms = now_ms() - clock.start;
  *code = NULL;
  if (!t) {
    *code = clock.timed_out ? "PARSE_TIME_LIMIT" : "PARSE_NULL";
    ts_parser_reset(p);
  }
  return t;
}

/* ---- preorder walk ---- */

typedef struct Walk Walk;
typedef void (*Visit)(Walk *w, TSNode n, uint32_t idx, int64_t parent, const char *field);
struct Walk { uint64_t max_nodes, max_depth; Visit visit; void *ctx; uint32_t count, max_seen; };

static const char *walk(TSNode root, Walk *w) {
  TSTreeCursor c = ts_tree_cursor_new(root);
  uint32_t *path = NULL, cap = 0, depth = 1, idx = 0;
  const char *err = NULL;
  for (;;) {
    if (depth > w->max_depth) { err = "DEPTH_LIMIT"; break; }
    if (idx >= w->max_nodes) { err = "NODE_LIMIT"; break; }
    if (depth > cap) {
      cap = cap ? cap * 2 : 64;
      path = t_realloc(path, cap * sizeof *path);
    }
    path[depth - 1] = idx;
    if (depth > w->max_seen) w->max_seen = depth;
    w->visit(w, ts_tree_cursor_current_node(&c), idx, depth > 1 ? (int64_t)path[depth - 2] : -1, ts_tree_cursor_current_field_name(&c));
    idx++;
    if (ts_tree_cursor_goto_first_child(&c)) { depth++; continue; }
    bool more = false;
    for (;;) {
      if (ts_tree_cursor_goto_next_sibling(&c)) { more = true; break; }
      if (!ts_tree_cursor_goto_parent(&c)) break;
      depth--;
    }
    if (!more) break;
  }
  w->count = idx;
  t_free(path);
  ts_tree_cursor_delete(&c);
  return err;
}

/* ---- name tables ---- */

typedef struct { const char **names; uint32_t n, cap, *slots, nslots; } Names;

static uint32_t fnv(const char *s) {
  uint32_t h = 2166136261u;
  for (; *s; s++) h = (h ^ (uint8_t)*s) * 16777619u;
  return h;
}
static uint32_t intern(Names *t, const char *s) {
  if (2 * (t->n + 1) > t->nslots) {
    uint32_t ns = t->nslots ? t->nslots * 2 : 64;
    uint32_t *slots = t_malloc(ns * sizeof *slots);
    for (uint32_t i = 0; i < ns; i++) slots[i] = UINT32_MAX;
    for (uint32_t i = 0; i < t->n; i++) {
      uint32_t h = fnv(t->names[i]) & (ns - 1);
      while (slots[h] != UINT32_MAX) h = (h + 1) & (ns - 1);
      slots[h] = i;
    }
    t_free(t->slots);
    t->slots = slots;
    t->nslots = ns;
  }
  uint32_t h = fnv(s) & (t->nslots - 1);
  while (t->slots[h] != UINT32_MAX) {
    if (!strcmp(t->names[t->slots[h]], s)) return t->slots[h];
    h = (h + 1) & (t->nslots - 1);
  }
  if (t->n == t->cap) {
    t->cap = t->cap ? t->cap * 2 : 32;
    t->names = t_realloc(t->names, t->cap * sizeof *t->names);
  }
  t->names[t->n] = s;
  t->slots[h] = t->n;
  return t->n++;
}
static void names_put(Buf *b, Names *t) {
  put(b, "[", 1);
  for (uint32_t i = 0; i < t->n; i++) { if (i) put(b, ",", 1); putstr(b, t->names[i]); }
  put(b, "]", 1);
}
static void names_free(Names *t) { t_free(t->names); t_free(t->slots); memset(t, 0, sizeof *t); }

/* ---- per-tree serialization context ---- */

typedef struct { int kind; const char *arg; size_t len; } Seg; /* 0 field, 1 child, 2 children */
typedef struct {
  Req *r;
  Sha sha;
  Names types, fields;
  Buf nodes, errs, decls;
  bool full, fault_flag;
  uint32_t err_total, err_items, decl_items;
  uint64_t *symdecl; /* per symbol: bit i set when decls[i].node matches */
  uint8_t *symdone;  /* per symbol: symdecl computed */
  uint32_t nsym;
} TreeCtx;

static void digest_node(TreeCtx *x, TSNode n, int64_t parent, const char *field, uint32_t flags) {
  char t[24];
  const char *type = ts_node_type(n);
  TSPoint a = ts_node_start_point(n), b = ts_node_end_point(n);
  uint64_t v[6] = {ts_node_start_byte(n), ts_node_end_byte(n), a.row, a.column, b.row, b.column};
  int k = snprintf(t, sizeof t, "%lld", (long long)parent);
  sha_update(&x->sha, t, (size_t)k);
  const char *strs[2] = {type, field ? field : ""};
  for (int i = 0; i < 2; i++) {
    k = snprintf(t, sizeof t, "%c%zu:", 0, strlen(strs[i]));
    sha_update(&x->sha, t, (size_t)k);
    sha_update(&x->sha, strs[i], strlen(strs[i]));
  }
  for (int i = 0; i < 5; i++) {
    char f[2] = {0, (char)('0' + ((flags >> i) & 1))};
    sha_update(&x->sha, f, 2);
  }
  for (int i = 0; i < 6; i++) {
    k = snprintf(t, sizeof t, "%c%llu", 0, (unsigned long long)v[i]);
    sha_update(&x->sha, t, (size_t)k);
  }
  sha_update(&x->sha, "\n", 1);
}

static uint32_t node_flags(TSNode n) {
  return (ts_node_is_named(n) ? 1u : 0) | (ts_node_is_extra(n) ? 2u : 0) | (ts_node_is_error(n) ? 4u : 0) |
         (ts_node_has_error(n) ? 8u : 0) | (ts_node_is_missing(n) ? 16u : 0);
}

static void node_array(Buf *b, TreeCtx *x, TSNode n, int64_t parent, const char *field, uint32_t flags) {
  TSPoint a = ts_node_start_point(n), e = ts_node_end_point(n);
  put(b, "[", 1); puti(b, parent);
  put(b, ",", 1); putu(b, intern(&x->types, ts_node_type(n)));
  put(b, ",", 1); puti(b, field ? (int64_t)intern(&x->fields, field) : -1);
  put(b, ",", 1); putu(b, flags);
  put(b, ",", 1); putu(b, ts_node_start_byte(n));
  put(b, ",", 1); putu(b, ts_node_end_byte(n));
  put(b, ",", 1); putu(b, a.row); put(b, ",", 1); putu(b, a.column);
  put(b, ",", 1); putu(b, e.row); put(b, ",", 1); putu(b, e.column);
  put(b, "]", 1);
}

typedef struct { TSNode *v; uint32_t n, cap; } NodeList;
static void list_push(NodeList *l, TSNode n) {
  if (l->n == l->cap) { l->cap = l->cap ? l->cap * 2 : 8; l->v = t_realloc(l->v, l->cap * sizeof *l->v); }
  l->v[l->n++] = n;
}

static void locate(TSNode decl, const char *loc, NodeList *out) {
  NodeList cur = {0}, next = {0};
  list_push(&cur, decl);
  const char *s = loc;
  while (*s && cur.n) {
    const char *slash = strchr(s, '/');
    size_t len = slash ? (size_t)(slash - s) : strlen(s);
    int kind = !strncmp(s, "field:", 6) ? 0 : !strncmp(s, "children:", 9) ? 2 : 1;
    const char *arg = s + (kind == 2 ? 9 : 6);
    size_t alen = len - (size_t)(arg - s);
    next.n = 0;
    for (uint32_t i = 0; i < cur.n; i++) {
      if (kind == 0) {
        TSNode c = ts_node_child_by_field_name(cur.v[i], arg, (uint32_t)alen);
        if (!ts_node_is_null(c)) list_push(&next, c);
        continue;
      }
      TSTreeCursor c = ts_tree_cursor_new(cur.v[i]);
      if (ts_tree_cursor_goto_first_child(&c)) {
        do {
          TSNode m = ts_tree_cursor_current_node(&c);
          const char *t = ts_node_type(m);
          if (ts_node_is_named(m) && strlen(t) == alen && !memcmp(t, arg, alen)) {
            list_push(&next, m);
            if (kind == 1) break;
          }
        } while (ts_tree_cursor_goto_next_sibling(&c));
      }
      ts_tree_cursor_delete(&c);
    }
    NodeList tmp = cur; cur = next; next = tmp;
    s += len;
    if (*s == '/') s++;
  }
  for (uint32_t i = 0; i < cur.n; i++) list_push(out, cur.v[i]);
  t_free(cur.v);
  t_free(next.v);
}

static void decl_item(TreeCtx *x, const Decl *d, TSNode n, const TSNode *name) {
  Buf *b = &x->decls;
  if (x->decl_items++) put(b, ",", 1);
  put(b, "{\"fact\":", 8); putstr(b, d->fact);
  puts_(b, ",\"node_type\":"); putstr(b, d->node);
  puts_(b, ",\"start_byte\":"); putu(b, ts_node_start_byte(n));
  puts_(b, ",\"end_byte\":"); putu(b, ts_node_end_byte(n));
  puts_(b, ",\"name\":");
  if (name) {
    puts_(b, "{\"start_byte\":"); putu(b, ts_node_start_byte(*name));
    puts_(b, ",\"end_byte\":"); putu(b, ts_node_end_byte(*name)); put(b, "}", 1);
  } else puts_(b, "null");
  const char *status = !name && strcmp(d->name, "node") ? "NAME_MISSING" : ts_node_has_error(n) ? "HAS_ERROR" : "PASS";
  puts_(b, ",\"status\":"); putstr(b, status);
  put(b, "}", 1);
}

static void declarations(TreeCtx *x, TSNode n) {
  if (!x->r->ndecls || !ts_node_is_named(n)) return;
  TSSymbol sym = ts_node_symbol(n);
  if (sym >= x->nsym) return;
  uint64_t mask = x->symdecl[sym];
  if (!x->symdone[sym]) {
    mask = 0;
    const char *t = ts_node_type(n);
    for (int i = 0; i < x->r->ndecls; i++) if (!strcmp(t, x->r->decls[i].node)) mask |= UINT64_C(1) << i;
    x->symdecl[sym] = mask;
    x->symdone[sym] = 1;
  }
  for (int i = 0; i < x->r->ndecls; i++) {
    if (!(mask & (UINT64_C(1) << i))) continue;
    const Decl *d = &x->r->decls[i];
    if (!strcmp(d->name, "node")) { decl_item(x, d, n, NULL); continue; }
    NodeList names = {0};
    locate(n, d->name, &names);
    if (!names.n) decl_item(x, d, n, NULL);
    for (uint32_t k = 0; k < names.n; k++) decl_item(x, d, n, &names.v[k]);
    t_free(names.v);
  }
}

static void visit_tree(Walk *w, TSNode n, uint32_t idx, int64_t parent, const char *field) {
  TreeCtx *x = w->ctx;
  uint32_t flags = node_flags(n);
  if (x->fault_flag && idx == 0) flags ^= 2u; /* owned fault: a wrong but well-formed tree */
  digest_node(x, n, parent, field, flags);
  if (x->full) {
    if (idx) put(&x->nodes, ",", 1);
    node_array(&x->nodes, x, n, parent, field, flags);
  } else if (flags & (4u | 16u)) {
    x->err_total++;
    if (x->err_items < x->r->errors) {
      Buf *b = &x->errs;
      if (x->err_items++) put(b, ",", 1);
      puts_(b, flags & 4u ? "{\"kind\":\"ERROR\",\"type\":" : "{\"kind\":\"MISSING\",\"type\":");
      putstr(b, ts_node_type(n));
      puts_(b, ",\"start_byte\":"); putu(b, ts_node_start_byte(n));
      puts_(b, ",\"end_byte\":"); putu(b, ts_node_end_byte(n));
      puts_(b, ",\"start_point\":"); putpoint(b, ts_node_start_point(n));
      puts_(b, ",\"end_point\":"); putpoint(b, ts_node_end_point(n));
      put(b, "}", 1);
    }
  }
  declarations(x, n);
}

/* Partial tree of one registered point: the ancestor chain of the deepest node that covers
 * the byte, then that node's subtree, at most partial_nodes nodes in preorder. */
typedef struct { TreeCtx *x; Buf *b; uint32_t base, written; const char *top_field; } Part;
static void visit_part(Walk *w, TSNode n, uint32_t idx, int64_t parent, const char *field) {
  Part *p = w->ctx;
  if (p->written++) put(p->b, ",", 1);
  node_array(p->b, p->x, n, parent < 0 ? (int64_t)p->base - 1 : parent + p->base, parent < 0 ? p->top_field : field, node_flags(n));
  (void)idx;
}

static bool covers(TSNode n, uint32_t b) { return ts_node_start_byte(n) <= b && b < ts_node_end_byte(n); }

static void partial_tree(TreeCtx *x, Buf *b, TSNode root, const Point *pt) {
  puts_(b, "{\"point\":"); putstr(b, pt->id);
  puts_(b, ",\"byte\":"); putu(b, pt->byte);
  NodeList chain = {0};
  const char **fields = NULL;
  uint32_t fcap = 0;
  if (covers(root, pt->byte)) {
    TSTreeCursor c = ts_tree_cursor_new(root);
    list_push(&chain, root);
    fields = t_realloc(NULL, (fcap = 8) * sizeof *fields);
    fields[0] = NULL;
    while (ts_tree_cursor_goto_first_child(&c)) {
      bool found = false;
      do {
        if (covers(ts_tree_cursor_current_node(&c), pt->byte)) { found = true; break; }
      } while (ts_tree_cursor_goto_next_sibling(&c));
      if (!found) break;
      if (chain.n == fcap) fields = t_realloc(fields, (fcap *= 2) * sizeof *fields);
      fields[chain.n] = ts_tree_cursor_current_field_name(&c);
      list_push(&chain, ts_tree_cursor_current_node(&c));
    }
    ts_tree_cursor_delete(&c);
  }
  bool truncated = false;
  Buf nodes = {0};
  nodes.limit = MAX_OUTPUT;
  if (chain.n) {
    /* ancestors first; the deepest node starts its own subtree walk */
    uint32_t anc = chain.n - 1, budget = (uint32_t)x->r->partial_nodes;
    uint32_t keep = anc < budget ? anc : budget;
    for (uint32_t i = 0; i < keep; i++) {
      if (i) put(&nodes, ",", 1);
      node_array(&nodes, x, chain.v[i], (int64_t)i - 1, fields[i], node_flags(chain.v[i]));
    }
    if (anc >= budget) truncated = true;
    else {
      Part part = {x, &nodes, anc, anc, fields[anc]};
      Walk w = {budget - anc, x->r->depth, visit_part, &part, 0, 0};
      if (walk(chain.v[anc], &w)) truncated = true;
    }
  }
  puts_(b, ",\"truncated\":"); putb(b, truncated);
  puts_(b, ",\"nodes\":[");
  if (nodes.len) put(b, nodes.data, nodes.len);
  if (nodes.over) b->over = true;
  puts_(b, "]}");
  buf_free(&nodes);
  t_free(chain.v);
  t_free(fields);
}

static const char *emit_queries(Buf *out, Req *r, TSTree *t);
static void emit_api(Buf *out, Req *r, TSTree *t, uint32_t count);

/* Serializes one tree object. Returns NULL for a completed tree, else its code; *status is
 * set to the tree status. fault_flag applies the owned STEP1 fault. An r2 tree also carries
 * its query results and API observations; a query that did not complete returns its code. */
static const char *emit_tree(Buf *out, Req *r, TSTree *t, const char *code, uint64_t ms, int form, bool fault_flag) {
  size_t mark = out->len;
  const char *status = "COMPLETED";
  if (!t) status = !strcmp(code, "PARSE_TIME_LIMIT") ? "RESOURCE_LIMIT" : "FAILED";
  TreeCtx x;
  memset(&x, 0, sizeof x);
  x.r = r;
  x.fault_flag = fault_flag;
  uint32_t count = 0, depth = 0;
  bool has_error = false;
  char digest[65] = {0};
  const char *reason = NULL;
  if (t) {
    TSNode root = ts_tree_root_node(t);
    count = ts_node_descendant_count(root);
    has_error = ts_node_has_error(root);
    if (count > r->nodes) { status = "RESOURCE_LIMIT"; code = "NODE_LIMIT"; }
    else {
      x.nsym = ts_language_symbol_count(ts_tree_language(t));
      x.symdecl = t_calloc(x.nsym ? x.nsym : 1, sizeof *x.symdecl);
      x.symdone = t_calloc(x.nsym ? x.nsym : 1, sizeof *x.symdone);
      x.full = form == OUT_TREE || (form == OUT_AUTO && count <= r->full_nodes);
      if (form == OUT_AUTO && !x.full) reason = "DESCENDANT_LIMIT";
      for (int attempt = 0; attempt < 2; attempt++) {
        sha_init(&x.sha);
        sha_update(&x.sha, "tsgk-tree-digest/r1\n", 20);
        names_free(&x.types);
        names_free(&x.fields);
        x.nodes = (Buf){0}; x.nodes.limit = out->limit;
        x.errs = (Buf){0}; x.errs.limit = out->limit;
        x.decls = (Buf){0}; x.decls.limit = out->limit;
        x.err_total = x.err_items = x.decl_items = 0;
        Walk w = {r->nodes, r->depth, visit_tree, &x, 0, 0};
        const char *e = walk(root, &w);
        depth = w.max_seen;
        if (!e && w.count != count) e = "DESCENDANT_COUNT_MISMATCH";
        if (e) { status = !strcmp(e, "DESCENDANT_COUNT_MISMATCH") ? "FAILED" : "RESOURCE_LIMIT"; code = e; break; }
        sha_hex(&x.sha, digest);
        if (x.full && form == OUT_AUTO && (x.nodes.over || x.nodes.len > out->limit - out->len)) {
          buf_free(&x.nodes); buf_free(&x.errs); buf_free(&x.decls);
          x.full = false;
          reason = "OUTPUT_LIMIT";
          continue;
        }
        break;
      }
    }
  }
  put(out, "{\"status\":", 10); putstr(out, status);
  puts_(out, ",\"code\":"); putstr(out, strcmp(status, "COMPLETED") ? code : "");
  puts_(out, ",\"parse_ms\":"); putu(out, ms);
  const char *qcode = NULL;
  if (strcmp(status, "COMPLETED")) {
    puts_(out, r->rev == 2 ? ",\"form\":null,\"queries\":null,\"api\":null}" : ",\"form\":null}");
  } else {
    puts_(out, ",\"form\":"); putstr(out, x.full ? "full" : form == OUT_RECORD ? "record" : "summary");
    if (!x.full && form == OUT_AUTO) { puts_(out, ",\"reason\":"); putstr(out, reason); }
    puts_(out, ",\"descendant_count\":"); putu(out, count);
    puts_(out, ",\"max_depth\":"); putu(out, depth);
    puts_(out, ",\"has_error\":"); putb(out, has_error);
    puts_(out, ",\"digest\":"); putstr(out, digest);
    if (x.full) {
      Buf nodes = x.nodes;
      puts_(out, ",\"types\":"); names_put(out, &x.types);
      puts_(out, ",\"fields\":"); names_put(out, &x.fields);
      puts_(out, ",\"nodes\":[");
      if (nodes.over) out->over = true; else put(out, nodes.data, nodes.len);
      put(out, "]", 1);
    } else {
      puts_(out, ",\"errors\":{\"limit\":"); putu(out, r->errors);
      puts_(out, ",\"total\":"); putu(out, x.err_total);
      puts_(out, ",\"truncated\":"); putb(out, x.err_total > x.err_items);
      puts_(out, ",\"items\":[");
      if (x.errs.over) out->over = true; else put(out, x.errs.data, x.errs.len);
      puts_(out, "]}");
    }
    puts_(out, ",\"declarations\":");
    if (!r->ndecls) puts_(out, "null");
    else {
      put(out, "[", 1);
      if (x.decls.over) out->over = true; else put(out, x.decls.data, x.decls.len);
      put(out, "]", 1);
    }
    if (!x.full && form == OUT_AUTO) {
      Buf parts = {0};
      parts.limit = out->limit;
      puts_(&parts, "[");
      for (int i = 0; i < r->npoints; i++) {
        if (i) put(&parts, ",", 1);
        partial_tree(&x, &parts, ts_tree_root_node(t), &r->points[i]);
      }
      puts_(&parts, "]");
      /* the partial nodes intern into the same tables, which are written after them */
      puts_(out, ",\"types\":"); names_put(out, &x.types);
      puts_(out, ",\"fields\":"); names_put(out, &x.fields);
      puts_(out, ",\"partial_trees\":");
      if (parts.over) out->over = true; else put(out, parts.data, parts.len);
      buf_free(&parts);
    }
    if (r->rev == 2) {
      /* the serialization buffers are released first: the queries may need the memory */
      buf_free(&x.nodes); buf_free(&x.errs); buf_free(&x.decls);
      qcode = emit_queries(out, r, t);
      puts_(out, ",\"api\":");
      if (r->api && x.full) emit_api(out, r, t, count); else puts_(out, "null");
    }
    put(out, "}", 1);
  }
  buf_free(&x.nodes); buf_free(&x.errs); buf_free(&x.decls);
  names_free(&x.types); names_free(&x.fields);
  t_free(x.symdecl);
  t_free(x.symdone);
  (void)mark;
  return strcmp(status, "COMPLETED") ? code : qcode;
}

/* ---- incremental route instrumentation: runtime node identities of the edited old tree ---- */

typedef struct { const void **slots; uint32_t n; uint64_t hits; bool count; } IdSet;
static uint32_t ptr_hash(const void *p) { uint64_t v = (uint64_t)(uintptr_t)p; v ^= v >> 33; v *= 0xff51afd7ed558ccdULL; v ^= v >> 33; return (uint32_t)v; }
static void visit_ids(Walk *w, TSNode n, uint32_t idx, int64_t parent, const char *field) {
  (void)idx; (void)parent; (void)field;
  IdSet *s = w->ctx;
  uint32_t h = ptr_hash(n.id) & (s->n - 1);
  while (s->slots[h]) {
    if (s->slots[h] == n.id) { if (s->count) s->hits++; return; }
    h = (h + 1) & (s->n - 1);
  }
  if (!s->count) s->slots[h] = n.id;
}
static void route_counts(Req *r, TSTree *old, TSTree *inc, TSTree *fresh, uint64_t *reused, uint64_t *fresh_reused) {
  uint32_t count = ts_node_descendant_count(ts_tree_root_node(old));
  IdSet s = {0};
  s.n = 64;
  while (s.n < 2 * (uint64_t)count + 2) s.n *= 2;
  s.slots = t_calloc(s.n, sizeof *s.slots);
  Walk w = {r->nodes + 1, r->depth, visit_ids, &s, 0, 0};
  walk(ts_tree_root_node(old), &w);
  s.count = true;
  walk(ts_tree_root_node(inc), &w);
  *reused = s.hits;
  s.hits = 0;
  walk(ts_tree_root_node(fresh), &w);
  *fresh_reused = s.hits;
  t_free(s.slots);
}

/* ---- r2: node index map ----
 * Maps a runtime node identity (TSNode.id, the node's subtree slot; ts_node_eq compares it)
 * to its preorder index in the serialized tree. Identities are used only inside this
 * process and never written; (type, start, end) is not assumed to be unique. */

typedef struct { const void **keys; uint32_t *vals; uint32_t n; } IdMap;
static void idmap_init(IdMap *m, uint64_t count) {
  m->n = 64;
  while (m->n < 2 * count + 2) m->n *= 2;
  m->keys = t_calloc(m->n, sizeof *m->keys);
  m->vals = t_calloc(m->n, sizeof *m->vals);
}
static uint32_t *idmap_slot(IdMap *m, const void *id, bool add) {
  uint32_t h = ptr_hash(id) & (m->n - 1);
  while (m->keys[h]) {
    if (m->keys[h] == id) return &m->vals[h];
    h = (h + 1) & (m->n - 1);
  }
  if (!add) return NULL;
  m->keys[h] = id;
  m->vals[h] = UINT32_MAX;
  return &m->vals[h];
}
static void idmap_free(IdMap *m) { t_free(m->keys); t_free(m->vals); memset(m, 0, sizeof *m); }
/* -1 for a null node, -2 for a node the map does not know (never expected) */
static int64_t idx_of(IdMap *m, TSNode n) {
  if (ts_node_is_null(n)) return -1;
  uint32_t *v = idmap_slot(m, n.id, false);
  return v && *v != UINT32_MAX ? (int64_t)*v : -2;
}
typedef struct { IdMap *m; bool all; } MapWalk;
static void visit_map(Walk *w, TSNode n, uint32_t idx, int64_t parent, const char *field) {
  (void)parent; (void)field;
  MapWalk *mw = w->ctx;
  uint32_t *v = idmap_slot(mw->m, n.id, mw->all);
  if (v) *v = idx;
}
static void map_tree(Req *r, TSTree *t, IdMap *m, bool all) {
  MapWalk mw = {m, all};
  Walk w = {r->nodes, r->depth, visit_map, &mw, 0, 0};
  walk(ts_tree_root_node(t), &w);
}

/* ---- r2: queries ----
 * Each compiled query runs on the step tree through the pinned runtime's capture stream
 * (ts_query_cursor_next_capture) and every capture is written in the order the runtime
 * returned it: no sorting, no deduplication. Predicates and directives are not evaluated
 * here; their steps are reported for the host's declared policy. A query cut by a limit
 * keeps its captures but is marked partial and RESOURCE_LIMIT. */

typedef struct { uint32_t match, pattern, capture; TSNode node; } CapRow;
typedef struct { CapRow *rows; uint32_t n, cap, matches; const char *code; uint64_t ms; } QRun;
typedef struct { uint64_t start, limit; bool timed_out; } QClock;
/* The pinned runtime does not keep a cancellation: next_capture calls the cursor again
 * while an earlier match is unfinished, so a cancelled query can run on. The first expiry
 * cancels (the loop below then stops and keeps its captures as partial); a callback after
 * that means the runtime continued, and the process ends with a typed QUERY_TIME_LIMIT. */
static bool qprogress(TSQueryCursorState *s) {
  QClock *c = s->payload;
  if (now_ms() - c->start <= c->limit) return false;
  if (c->timed_out) fatal_limit("QUERY_TIME_LIMIT");
  c->timed_out = true;
  return true;
}
static const char *quant_name(TSQuantifier q) {
  switch (q) {
    case TSQuantifierZero: return "ZERO";
    case TSQuantifierZeroOrOne: return "ZERO_OR_ONE";
    case TSQuantifierZeroOrMore: return "ZERO_OR_MORE";
    case TSQuantifierOne: return "ONE";
    default: return "ONE_OR_MORE";
  }
}
static const char *qerr_name(TSQueryError e) {
  static const char *names[] = {"NONE", "SYNTAX", "NODE_TYPE", "FIELD", "CAPTURE", "STRUCTURE", "LANGUAGE"};
  return (unsigned)e < sizeof names / sizeof *names ? names[e] : "UNKNOWN";
}

static void run_query(Req *r, const TSQuery *q, TSNode root, QRun *run) {
  TSQueryCursor *qc = ts_query_cursor_new();
  QClock clock = {now_ms(), r->query_ms, false};
  TSQueryCursorOptions opt = {&clock, qprogress};
  ts_query_cursor_exec_with_options(qc, q, root, &opt);
  TSQueryMatch m;
  uint32_t ci;
  for (;;) {
    /* the runtime does not keep a cancellation: the loop stops on the flag itself */
    if (clock.timed_out) { run->code = "QUERY_TIME_LIMIT"; break; }
    if (!ts_query_cursor_next_capture(qc, &m, &ci)) {
      if (clock.timed_out) run->code = "QUERY_TIME_LIMIT";
      break;
    }
    if ((uint64_t)m.id >= r->matches) { run->code = "MATCH_LIMIT"; break; }
    if (run->n == r->captures) { run->code = "CAPTURE_LIMIT"; break; }
    if (run->n == run->cap) {
      run->cap = run->cap ? run->cap * 2 : 64;
      run->rows = t_realloc(run->rows, run->cap * sizeof *run->rows);
    }
    run->rows[run->n++] = (CapRow){m.id, m.pattern_index, m.captures[ci].index, m.captures[ci].node};
    if (m.id + 1 > run->matches) run->matches = m.id + 1;
  }
  /* an in-progress match dropped for capacity makes the stream incomplete */
  if (!run->code && ts_query_cursor_did_exceed_match_limit(qc)) run->code = "QUERY_MATCH_OVERFLOW";
  run->ms = now_ms() - clock.start;
  ts_query_cursor_delete(qc);
}

/* Predicates as the runtime lists them: per pattern, each predicate's steps in order (the
 * first step is the operator string); capture steps carry the capture's quantifier. */
static void emit_predicates(Buf *b, const TSQuery *q) {
  put(b, "[", 1);
  bool firstp = true;
  for (uint32_t p = 0; p < ts_query_pattern_count(q); p++) {
    uint32_t ns;
    const TSQueryPredicateStep *st = ts_query_predicates_for_pattern(q, p, &ns);
    for (uint32_t k = 0; k < ns;) {
      if (!firstp) put(b, ",", 1);
      firstp = false;
      puts_(b, "{\"pattern\":"); putu(b, p); puts_(b, ",\"steps\":[");
      for (bool firsts = true; k < ns && st[k].type != TSQueryPredicateStepTypeDone; k++, firsts = false) {
        uint32_t len;
        if (!firsts) put(b, ",", 1);
        if (st[k].type == TSQueryPredicateStepTypeCapture) {
          const char *name = ts_query_capture_name_for_id(q, st[k].value_id, &len);
          puts_(b, "{\"kind\":\"capture\",\"value\":"); putstrn(b, name, len);
          puts_(b, ",\"quantifier\":"); putstr(b, quant_name(ts_query_capture_quantifier_for_id(q, p, st[k].value_id)));
          put(b, "}", 1);
        } else {
          const char *s = ts_query_string_value_for_id(q, st[k].value_id, &len);
          puts_(b, "{\"kind\":\"string\",\"value\":"); putstrn(b, s, len); puts_(b, ",\"quantifier\":null}");
        }
      }
      puts_(b, "]}");
      k++; /* the Done sentinel */
    }
  }
  put(b, "]", 1);
}

/* Writes ,"queries":[...] for one completed tree; returns the code of the first query that
 * did not complete, else NULL. */
static const char *emit_queries(Buf *out, Req *r, TSTree *t) {
  puts_(out, ",\"queries\":");
  if (!r->nqueries) { puts_(out, "null"); return NULL; }
  TSNode root = ts_tree_root_node(t);
  QRun runs[MAX_QUERIES];
  memset(runs, 0, sizeof runs);
  uint64_t total = 0;
  for (int i = 0; i < r->nqueries; i++) {
    if (!r->queries[i].q) continue;
    run_query(r, r->queries[i].q, root, &runs[i]);
    total += runs[i].n;
  }
  /* one preorder walk resolves every captured node to its index */
  IdMap m;
  idmap_init(&m, total);
  for (int i = 0; i < r->nqueries; i++)
    for (uint32_t k = 0; k < runs[i].n; k++) idmap_slot(&m, runs[i].rows[k].node.id, true);
  if (total) map_tree(r, t, &m, false);
  const char *first = NULL;
  put(out, "[", 1);
  for (int i = 0; i < r->nqueries; i++) {
    Query *qq = &r->queries[i];
    QRun *run = &runs[i];
    if (i) put(out, ",", 1);
    puts_(out, "{\"id\":"); putstr(out, qq->id);
    if (!qq->q) {
      char code[32];
      snprintf(code, sizeof code, "QUERY_%s", qerr_name(qq->err));
      TSPoint at = point_at(ENC_UTF8, qq->src, qq->err_offset <= qq->len ? qq->err_offset : qq->len);
      puts_(out, ",\"status\":\"INVALID_QUERY\",\"code\":"); putstr(out, code);
      puts_(out, ",\"query_ms\":0,\"error\":{\"type\":"); putstr(out, qerr_name(qq->err));
      puts_(out, ",\"offset\":"); putu(out, qq->err_offset);
      puts_(out, ",\"point\":"); putpoint(out, at);
      puts_(out, "},\"patterns\":0,\"capture_names\":[],\"predicates\":[],\"matches\":0,\"partial\":false,\"types\":[],\"captures\":null}");
      continue;
    }
    const TSQuery *q = qq->q;
    /* a capture that the walk did not reach would break the record linkage */
    const char *code = run->code;
    for (uint32_t k = 0; !code && k < run->n; k++) {
      uint32_t *v = idmap_slot(&m, run->rows[k].node.id, false);
      if (!v || *v == UINT32_MAX) code = "CAPTURE_NODE_UNMAPPED";
    }
    bool unmapped = code && !strcmp(code, "CAPTURE_NODE_UNMAPPED");
    if (code && !first) first = code;
    puts_(out, ",\"status\":"); putstr(out, !code ? "COMPLETED" : unmapped ? "FAILED" : "RESOURCE_LIMIT");
    puts_(out, ",\"code\":"); putstr(out, code ? code : "");
    puts_(out, ",\"query_ms\":"); putu(out, run->ms);
    puts_(out, ",\"error\":null,\"patterns\":"); putu(out, ts_query_pattern_count(q));
    puts_(out, ",\"capture_names\":[");
    for (uint32_t k = 0; k < ts_query_capture_count(q); k++) {
      uint32_t len;
      const char *name = ts_query_capture_name_for_id(q, k, &len);
      if (k) put(out, ",", 1);
      putstrn(out, name, len);
    }
    puts_(out, "],\"predicates\":"); emit_predicates(out, q);
    puts_(out, ",\"matches\":"); putu(out, run->matches);
    puts_(out, ",\"partial\":"); putb(out, code && !unmapped);
    Names types = {0};
    Buf rows = {0};
    rows.limit = out->limit;
    for (uint32_t k = 0; !unmapped && k < run->n; k++) {
      CapRow *c = &run->rows[k];
      TSNode n = c->node;
      TSPoint a = ts_node_start_point(n), e = ts_node_end_point(n);
      if (k) put(&rows, ",", 1);
      put(&rows, "[", 1); putu(&rows, c->match);
      put(&rows, ",", 1); putu(&rows, c->pattern);
      put(&rows, ",", 1); putu(&rows, c->capture);
      put(&rows, ",", 1); putu(&rows, *idmap_slot(&m, n.id, false));
      put(&rows, ",", 1); putu(&rows, intern(&types, ts_node_type(n)));
      put(&rows, ",", 1); putu(&rows, node_flags(n));
      put(&rows, ",", 1); putu(&rows, ts_node_start_byte(n));
      put(&rows, ",", 1); putu(&rows, ts_node_end_byte(n));
      put(&rows, ",", 1); putu(&rows, a.row); put(&rows, ",", 1); putu(&rows, a.column);
      put(&rows, ",", 1); putu(&rows, e.row); put(&rows, ",", 1); putu(&rows, e.column);
      put(&rows, "]", 1);
    }
    puts_(out, ",\"types\":"); names_put(out, &types);
    puts_(out, ",\"captures\":");
    if (unmapped) puts_(out, "null");
    else {
      put(out, "[", 1);
      if (rows.over) out->over = true; else put(out, rows.data, rows.len);
      put(out, "]", 1);
    }
    put(out, "}", 1);
    buf_free(&rows);
    names_free(&types);
  }
  put(out, "]", 1);
  for (int i = 0; i < r->nqueries; i++) t_free(runs[i].rows);
  idmap_free(&m);
  return first;
}

/* ---- r2: public API observations (tsgk-api/r1) ----
 * For a full tree: per preorder node the node API's parent, siblings, first children,
 * counts and a cursor positioned by descendant index; every field lookup by field id; the
 * field name the node API gives each child; and the byte lookups of the request points.
 * The kit compares them with the cursor serialization of the same tree. */

typedef struct { Req *r; IdMap *m; TSTreeCursor cur; Buf nodes, fields, looks; Names names; const TSLanguage *lang; } ApiCtx;

static void visit_api(Walk *w, TSNode n, uint32_t idx, int64_t parent, const char *field) {
  (void)parent; (void)field;
  ApiCtx *x = w->ctx;
  Buf *b = &x->nodes;
  uint32_t children = ts_node_child_count(n);
  int64_t v[13];
  v[0] = idx_of(x->m, ts_node_parent(n));
  v[1] = idx_of(x->m, ts_node_prev_sibling(n));
  v[2] = idx_of(x->m, ts_node_next_sibling(n));
  v[3] = idx_of(x->m, ts_node_prev_named_sibling(n));
  v[4] = idx_of(x->m, ts_node_next_named_sibling(n));
  v[5] = children ? idx_of(x->m, ts_node_child(n, 0)) : -1;
  v[6] = ts_node_named_child_count(n) ? idx_of(x->m, ts_node_named_child(n, 0)) : -1;
  v[7] = children;
  v[8] = ts_node_named_child_count(n);
  v[9] = ts_node_descendant_count(n);
  ts_tree_cursor_goto_descendant(&x->cur, idx);
  v[10] = ts_tree_cursor_current_depth(&x->cur);
  v[11] = ts_tree_cursor_current_descendant_index(&x->cur);
  v[12] = idx_of(x->m, ts_tree_cursor_current_node(&x->cur));
  if (idx) put(b, ",", 1);
  put(b, "[", 1);
  for (int i = 0; i < 13; i++) { if (i) put(b, ",", 1); puti(b, v[i]); }
  put(b, "]", 1);
  for (uint32_t k = 0; k < children; k++) {
    const char *f = ts_node_field_name_for_child(n, k);
    if (!f) continue;
    if (x->fields.len) put(&x->fields, ",", 1);
    put(&x->fields, "[", 1); putu(&x->fields, idx);
    put(&x->fields, ",", 1); putu(&x->fields, k);
    put(&x->fields, ",", 1); putu(&x->fields, intern(&x->names, f));
    put(&x->fields, "]", 1);
  }
  if (!children) return;
  uint32_t nf = ts_language_field_count(x->lang);
  for (TSFieldId f = 1; f <= nf; f++) {
    TSNode c = ts_node_child_by_field_id(n, f);
    if (ts_node_is_null(c)) continue;
    if (x->looks.len) put(&x->looks, ",", 1);
    put(&x->looks, "[", 1); putu(&x->looks, idx);
    put(&x->looks, ",", 1); putu(&x->looks, intern(&x->names, ts_language_field_name_for_id(x->lang, f)));
    put(&x->looks, ",", 1); puti(&x->looks, idx_of(x->m, c));
    put(&x->looks, "]", 1);
  }
}

static void emit_api(Buf *out, Req *r, TSTree *t, uint32_t count) {
  TSNode root = ts_tree_root_node(t);
  IdMap m;
  idmap_init(&m, count);
  map_tree(r, t, &m, true);
  ApiCtx x;
  memset(&x, 0, sizeof x);
  x.r = r;
  x.m = &m;
  x.lang = ts_tree_language(t);
  x.cur = ts_tree_cursor_new(root);
  x.nodes.limit = x.fields.limit = x.looks.limit = out->limit;
  Walk w = {r->nodes, r->depth, visit_api, &x, 0, 0};
  walk(root, &w);
  ts_tree_cursor_delete(&x.cur);
  puts_(out, "{\"revision\":\"tsgk-api/r1\",\"nodes\":[");
  if (x.nodes.over) out->over = true; else put(out, x.nodes.data, x.nodes.len);
  puts_(out, "],\"child_fields\":[");
  if (x.fields.over) out->over = true; else put(out, x.fields.data, x.fields.len);
  puts_(out, "],\"field_lookups\":[");
  if (x.looks.over) out->over = true; else put(out, x.looks.data, x.looks.len);
  puts_(out, "],\"points\":[");
  for (int i = 0; i < r->npoints; i++) {
    uint32_t b = r->points[i].byte;
    if (i) put(out, ",", 1);
    put(out, "[", 1); putu(out, b);
    put(out, ",", 1); puti(out, idx_of(&m, ts_node_descendant_for_byte_range(root, b, b)));
    put(out, ",", 1); puti(out, idx_of(&m, ts_node_named_descendant_for_byte_range(root, b, b)));
    put(out, ",", 1); puti(out, idx_of(&m, ts_node_first_child_for_byte(root, b)));
    put(out, "]", 1);
  }
  puts_(out, "],\"names\":"); names_put(out, &x.names);
  put(out, "}", 1);
  buf_free(&x.nodes); buf_free(&x.fields); buf_free(&x.looks);
  names_free(&x.names);
  idmap_free(&m);
}

/* ---- request handling ---- */

static int exit_for(const char *status) {
  if (!strcmp(status, "COMPLETED")) return 0;
  if (!strcmp(status, "INVALID_REQUEST")) return 2;
  if (!strcmp(status, "RESOURCE_LIMIT")) return 3;
  return 4;
}

static void respond(const char *status, const char *code, uint32_t source_bytes, uint32_t done, Buf *steps, uint64_t limit) {
  Buf out = {0};
  out.limit = limit;
  puts_(&out, "{\"protocol\":"); putstr(&out, protocol_name());
  puts_(&out, ",\"id\":"); putstr(&out, req_id);
  puts_(&out, ",\"status\":"); putstr(&out, status);
  puts_(&out, ",\"code\":"); putstr(&out, code ? code : "");
  puts_(&out, ",\"producer\":"); puts_(&out, producer());
  puts_(&out, ",\"source_bytes\":"); putu(&out, source_bytes);
  puts_(&out, ",\"steps_completed\":"); putu(&out, done);
  puts_(&out, ",\"steps\":[");
  if (steps && steps->len) put(&out, steps->data, steps->len);
  puts_(&out, "],\"complete\":true}");
  if (out.over) {
    /* the steps do not fit: report the limit without them (the small frame always fits) */
    buf_free(&out);
    out.limit = MAX_OUTPUT;
    puts_(&out, "{\"protocol\":"); putstr(&out, protocol_name());
    puts_(&out, ",\"id\":"); putstr(&out, req_id);
    puts_(&out, ",\"status\":\"RESOURCE_LIMIT\",\"code\":\"OUTPUT_LIMIT\",\"producer\":"); puts_(&out, producer());
    puts_(&out, ",\"source_bytes\":"); putu(&out, source_bytes);
    puts_(&out, ",\"steps_completed\":"); putu(&out, done);
    puts_(&out, ",\"steps\":[],\"complete\":true}");
  }
  write_frame(out.data, out.len);
  buf_free(&out);
}

static void step_header(Buf *b, int k, Src *src, const Edit *e, const Src *prev, int enc) {
  Sha s;
  char hex[65];
  sha_init(&s);
  sha_update(&s, src->s, src->n);
  sha_hex(&s, hex);
  if (k) put(b, ",", 1);
  puts_(b, "{\"step\":"); putu(b, (uint64_t)k);
  puts_(b, ",\"source_bytes\":"); putu(b, src->n);
  puts_(b, ",\"source_sha256\":"); putstr(b, hex);
  puts_(b, ",\"edit\":");
  if (!e) { puts_(b, "null"); return; }
  puts_(b, "{\"start_byte\":"); putu(b, e->start);
  puts_(b, ",\"old_end_byte\":"); putu(b, e->old_end);
  puts_(b, ",\"new_end_byte\":"); putu(b, e->new_end);
  puts_(b, ",\"start_point\":"); putpoint(b, point_at(enc, prev->s, e->start));
  puts_(b, ",\"old_end_point\":"); putpoint(b, point_at(enc, prev->s, e->old_end));
  puts_(b, ",\"new_end_point\":"); putpoint(b, point_at(enc, src->s, e->new_end));
  put(b, "}", 1);
}

static int handle(const uint8_t *data, size_t n) {
  Req r;
  req_id[0] = 0;
  req_rev = 1;
  cur_source_bytes = cur_steps_done = 0;
  const char *code = parse_request(data, n, &r);
  if (code) { respond("INVALID_REQUEST", code, 0, 0, NULL, MAX_OUTPUT); req_free(&r); return 2; }
  mem_limit = r.memory_bytes;
  cur_source_bytes = r.source_len;
  Src v[MAX_EDITS + 1];
  memset(v, 0, sizeof v);
  code = valid_source(r.enc, r.source, r.source_len) ? apply_edits(&r, v) : "SOURCE_ENCODING_INVALID";
  if (!code) code = check_ranges(&r, v);
  if (code) {
    for (int k = 1; k <= r.nedits; k++) t_free(v[k].s);
    respond("INVALID_REQUEST", code, r.source_len, 0, NULL, MAX_OUTPUT);
    req_free(&r);
    mem_limit = MAX_MEMORY;
    return 2;
  }
  const char *status = "COMPLETED";
  Buf steps = {0};
  steps.limit = r.output_bytes;
  TSParser *a = ts_parser_new(), *b = ts_parser_new();
  if (!ts_parser_set_language(a, tsgk_language()) || !ts_parser_set_language(b, tsgk_language())) {
    status = "FAILED";
    code = "LANGUAGE_INCOMPATIBLE";
  } else {
    /* queries compile once, before the first parse; an invalid one is reported per tree */
    for (int i = 0; i < r.nqueries; i++) {
      Query *q = &r.queries[i];
      q->q = ts_query_new(tsgk_language(), (const char *)q->src, q->len, &q->err_offset, &q->err);
    }
    uint64_t ms;
    const char *pcode;
    set_ranges(a, &r, 0);
    TSTree *prev = parse(a, NULL, &v[0], r.enc, r.parse_ms, &pcode, &ms);
    step_header(&steps, 0, &v[0], NULL, NULL, r.enc);
    puts_(&steps, ",\"route\":null,\"incremental\":");
    code = emit_tree(&steps, &r, prev, pcode, ms, r.out, false);
    puts_(&steps, ",\"fresh\":null}");
    if (!code) cur_steps_done = 1;
    for (int k = 0; !code && k < r.nedits; k++) {
      Edit *e = &r.edits[k];
      TSInputEdit edit = {e->start, e->old_end, e->new_end, point_at(r.enc, v[k].s, e->start),
                          point_at(r.enc, v[k].s, e->old_end), point_at(r.enc, v[k + 1].s, e->new_end)};
#ifndef TSGK_FAULT_OMIT_EDIT
      ts_tree_edit(prev, &edit);
#endif
      TSNode oldroot = ts_tree_root_node(prev);
      bool changed = ts_node_has_changes(oldroot);
      uint32_t oldend = ts_node_end_byte(oldroot);
      set_ranges(a, &r, k + 1);
#ifdef TSGK_FAULT_OMIT_OLD_TREE
      TSTree *inc = parse(a, NULL, &v[k + 1], r.enc, r.parse_ms, &pcode, &ms);
#else
      TSTree *inc = parse(a, prev, &v[k + 1], r.enc, r.parse_ms, &pcode, &ms);
#endif
      step_header(&steps, k + 1, &v[k + 1], e, &v[k], r.enc);
      if (!inc) {
        puts_(&steps, ",\"route\":null,\"incremental\":");
        code = emit_tree(&steps, &r, NULL, pcode, ms, r.out, false);
        puts_(&steps, ",\"fresh\":null}");
        break;
      }
      uint64_t fms;
      const char *fcode;
      set_ranges(b, &r, k + 1);
#ifdef TSGK_FAULT_FRESH_WITH_OLD
      TSTree *fresh = parse(b, prev, &v[k + 1], r.enc, r.parse_ms, &fcode, &fms);
#else
      TSTree *fresh = parse(b, NULL, &v[k + 1], r.enc, r.parse_ms, &fcode, &fms);
#endif
      if (fresh) {
        uint64_t reused, fresh_reused;
        route_counts(&r, prev, inc, fresh, &reused, &fresh_reused);
        puts_(&steps, ",\"route\":{\"edit_has_changes\":"); putb(&steps, changed);
        puts_(&steps, ",\"edited_root_end_byte\":"); putu(&steps, oldend);
        puts_(&steps, ",\"reused_nodes\":"); putu(&steps, reused);
        puts_(&steps, ",\"fresh_reused_nodes\":"); putu(&steps, fresh_reused);
        put(&steps, "}", 1);
      } else puts_(&steps, ",\"route\":null");
      puts_(&steps, ",\"incremental\":");
#ifdef TSGK_FAULT_STEP1_FLAG
      bool fault = k == 0;
#else
      bool fault = false;
#endif
      code = emit_tree(&steps, &r, inc, NULL, ms, r.out, fault);
      puts_(&steps, ",\"fresh\":");
      if (!code) code = emit_tree(&steps, &r, fresh, fcode, fms, r.out, false);
      else puts_(&steps, "null");
      put(&steps, "}", 1);
      if (fresh) ts_tree_delete(fresh);
      ts_tree_delete(prev);
      prev = inc;
      if (!code) cur_steps_done = (uint32_t)k + 2;
    }
    if (code) {
      status = !strcmp(code, "PARSE_NULL") || !strcmp(code, "DESCENDANT_COUNT_MISMATCH") || !strcmp(code, "CAPTURE_NODE_UNMAPPED") ? "FAILED" : "RESOURCE_LIMIT";
    }
    if (prev) ts_tree_delete(prev);
  }
  ts_parser_delete(a);
  ts_parser_delete(b);
  if (steps.over) { status = "RESOURCE_LIMIT"; code = "OUTPUT_LIMIT"; steps.len = 0; }
  respond(status, code, r.source_len, cur_steps_done, &steps, r.output_bytes);
  buf_free(&steps);
  for (int k = 1; k <= r.nedits; k++) t_free(v[k].s);
  req_free(&r);
  mem_limit = MAX_MEMORY;
  return exit_for(status);
}

/* ---- framing ---- */

static size_t read_full(void *buf, size_t n) {
  size_t got = 0;
  while (got < n) {
    size_t k = fread((char *)buf + got, 1, n - got, stdin);
    if (!k) break;
    got += k;
  }
  return got;
}

static int protocol_error(const char *code) {
  respond("INVALID_REQUEST", code, 0, 0, NULL, MAX_OUTPUT);
  return 2;
}

int main(int argc, char **argv) {
#ifdef _WIN32
  _setmode(_fileno(stdin), _O_BINARY);
  _setmode(_fileno(stdout), _O_BINARY);
#endif
  if (argc != 2 || (strcmp(argv[1], "single") && strcmp(argv[1], "batch"))) {
    fputs("usage: tsgk-native-driver single|batch\n", stderr);
    return 2;
  }
  bool batch = !strcmp(argv[1], "batch");
  ts_set_allocator(t_malloc, t_calloc, t_realloc, t_free);
  const TSLanguage *lang = tsgk_language();
  unsigned abi = ts_language_abi_version(lang);
  snprintf(producer_json[0], sizeof producer_json[0],
           "{\"language_version\":%u,\"runtime_language_version\":%u,\"runtime_min_compatible\":%u,\"query\":\"UNSUPPORTED\"}",
           abi, (unsigned)TREE_SITTER_LANGUAGE_VERSION, (unsigned)TREE_SITTER_MIN_COMPATIBLE_LANGUAGE_VERSION);
  /* r2 capability declaration: what this producer can observe, and the language shape */
#ifdef TSGK_FAULT_NO_QUERY
  const char *query_cap = "UNSUPPORTED"; /* owned fault: a producer without the query capability */
#else
  const char *query_cap = "tsgk-query/r1";
#endif
  snprintf(producer_json[1], sizeof producer_json[1],
           "{\"language_version\":%u,\"runtime_language_version\":%u,\"runtime_min_compatible\":%u,\"query\":\"%s\",\"api\":\"tsgk-api/r1\","
           "\"predicates\":\"NOT_EVALUATED\",\"symbol_count\":%u,\"field_count\":%u}",
           abi, (unsigned)TREE_SITTER_LANGUAGE_VERSION, (unsigned)TREE_SITTER_MIN_COMPATIBLE_LANGUAGE_VERSION, query_cap,
           (unsigned)ts_language_symbol_count(lang), (unsigned)ts_language_field_count(lang));
  for (;;) {
    uint8_t hdr[4];
    size_t got = read_full(hdr, 4);
    if (got == 0 && batch) {
#ifdef TSGK_FAULT_TRAILING
      fwrite("junk", 1, 4, stdout); /* owned fault: bytes after the last response */
#endif
      return 0;
    }
    req_id[0] = 0;
    req_rev = 1;
    if (got < 4) return protocol_error("FRAME_TRUNCATED");
    uint32_t len = (uint32_t)hdr[0] << 24 | (uint32_t)hdr[1] << 16 | (uint32_t)hdr[2] << 8 | hdr[3];
    if (len > MAX_FRAME) return protocol_error("FRAME_TOO_LARGE");
    uint8_t *data = t_malloc((size_t)len + 1);
    const char *perr = read_full(data, len) < len ? "FRAME_TRUNCATED" : (!batch && fgetc(stdin) != EOF) ? "FRAME_EXTRA" : NULL;
    if (perr) {
      t_free(data);
      return protocol_error(perr);
    }
    int code = handle(data, len);
    t_free(data);
    if (!batch) return code;
  }
}
