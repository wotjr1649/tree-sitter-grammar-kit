#include <assert.h>
#include <stdio.h>
#include "tree_sitter/api.h"
#include "subtree.h"
#include "tree.h"

extern const TSLanguage *tree_sitter_tsgk_plain(void);

int main(void) {
  const TSLanguage *language = tree_sitter_tsgk_plain();
  TSSymbol root_symbol = ts_language_symbol_for_name(language, "source_file", 11, true);
  TSSymbol named = ts_language_symbol_for_name(language, "identifier", 10, true);
  TSSymbol unnamed = ts_language_symbol_for_name(language, ";", 1, false);
  TSSymbol hidden = 0;
  for (TSSymbol i = 1; i < ts_language_symbol_count(language); i++) {
    if (ts_language_symbol_type(language, i) == TSSymbolTypeAuxiliary) { hidden = i; break; }
  }
  assert(root_symbol && named && unnamed && hidden);
  for (unsigned wrapped = 0; wrapped < 4; wrapped++) {
    for (unsigned named_only = 0; named_only < 2; named_only++) {
    for (unsigned positive_tail = 0; positive_tail < 2; positive_tail++) {
      SubtreePool pool = ts_subtree_pool_new(8);
      SubtreeArray children = {0};
      unsigned groups = wrapped >= 2 ? 2 : 1;
      unsigned per_group = named_only ? 2 : 3;
      for (unsigned group = 0; group < groups; group++) {
        SubtreeArray inner = {0};
        for (unsigned i = 0; i < per_group; i++) {
          array_push(&inner, ts_subtree_new_missing_leaf(&pool, !named_only && i == 1 ? unnamed : named, 0, length_zero(), 0, language));
        }
        if (wrapped) {
          Subtree wrapper = ts_subtree_from_mut(ts_subtree_new_node(hidden, &inner, 0, language));
          if (wrapped == 3) {
            inner = (SubtreeArray){0};
            array_push(&inner, wrapper);
            wrapper = ts_subtree_from_mut(ts_subtree_new_node(hidden, &inner, 0, language));
          }
          array_push(&children, wrapper);
        } else {
          for (unsigned i = 0; i < per_group; i++) array_push(&children, inner.contents[i]);
          array_delete(&inner);
        }
      }
      if (positive_tail) {
        Length one = {1, {0, 1}};
        array_push(&children, ts_subtree_new_leaf(&pool, named, length_zero(), one, 0, 0, false, false, false, language));
      }
      TSRange range = {{0, 0}, {0, positive_tail}, 0, positive_tail};
      TSTree *tree = ts_tree_new(ts_subtree_from_mut(ts_subtree_new_node(root_symbol, &children, 0, language)), language, &range, 1);
      TSNode root = ts_tree_root_node(tree), nodes[7];
      unsigned count = 0;
      TSTreeCursor cursor = ts_tree_cursor_new(root);
      assert(ts_tree_cursor_goto_first_child(&cursor));
      do { assert(count < 7); nodes[count++] = ts_tree_cursor_current_node(&cursor); }
      while (ts_tree_cursor_goto_next_sibling(&cursor));
      assert(count == groups * per_group + positive_tail);
      for (unsigned i = 0; i < count; i++) {
        assert(ts_node_eq(ts_node_parent(nodes[i]), root));
        TSNode next = ts_node_next_sibling(nodes[i]);
        assert(i + 1 < count ? ts_node_eq(next, nodes[i + 1]) : ts_node_is_null(next));
        unsigned j = i + 1;
        while (j < count && !ts_node_is_named(nodes[j])) j++;
        next = ts_node_next_named_sibling(nodes[i]);
        assert(j < count ? ts_node_eq(next, nodes[j]) : ts_node_is_null(next));
        TSNode prev = ts_node_prev_sibling(nodes[i]);
        assert(i > 0 ? ts_node_eq(prev, nodes[i - 1]) : ts_node_is_null(prev));
        int previous = (int)i - 1;
        while (previous >= 0 && !ts_node_is_named(nodes[previous])) previous--;
        prev = ts_node_prev_named_sibling(nodes[i]);
        assert(previous >= 0 ? ts_node_eq(prev, nodes[previous]) : ts_node_is_null(prev));
        if (i < groups * per_group) assert(ts_node_start_byte(nodes[i]) == 0 && ts_node_end_byte(nodes[i]) == 0);
        for (unsigned k = 0; k < i; k++) assert(!ts_node_eq(nodes[i], nodes[k]));
      }
      assert(ts_node_is_null(ts_node_next_sibling(root)));
      assert(ts_node_is_null(ts_node_prev_sibling(root)));
      assert(ts_node_is_null(ts_node_next_named_sibling(root)));
      assert(ts_node_is_null(ts_node_prev_named_sibling(root)));
      TSTree *copy = ts_tree_copy(tree);
      assert(!ts_node_eq(ts_tree_root_node(copy), root));
      ts_tree_delete(copy);
      ts_tree_cursor_delete(&cursor);
      ts_tree_delete(tree);
      ts_subtree_pool_delete(&pool);
    }
  }
    }
  puts("zero-width identities, named/anonymous, multiple/nested hidden wrappers, both directions, EOF and cross-tree: PASS");
  return 0;
}
