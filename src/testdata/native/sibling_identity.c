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
  for (unsigned wrapped = 0; wrapped < 2; wrapped++) {
    for (unsigned positive_tail = 0; positive_tail < 2; positive_tail++) {
      SubtreePool pool = ts_subtree_pool_new(8);
      SubtreeArray children = {0};
      for (unsigned i = 0; i < 3; i++) {
        array_push(&children, ts_subtree_new_missing_leaf(&pool, i == 1 ? unnamed : named, 0, length_zero(), 0, language));
      }
      if (positive_tail) {
        Length one = {1, {0, 1}};
        array_push(&children, ts_subtree_new_leaf(&pool, named, length_zero(), one, 0, 0, false, false, false, language));
      }
      if (wrapped) {
        Subtree wrapper = ts_subtree_from_mut(ts_subtree_new_node(hidden, &children, 0, language));
        children = (SubtreeArray){0};
        array_push(&children, wrapper);
      }
      TSTree *tree = ts_tree_new(ts_subtree_from_mut(ts_subtree_new_node(root_symbol, &children, 0, language)), language, NULL, 0);
      TSNode root = ts_tree_root_node(tree), nodes[4];
      unsigned count = 0;
      TSTreeCursor cursor = ts_tree_cursor_new(root);
      assert(ts_tree_cursor_goto_first_child(&cursor));
      do { assert(count < 4); nodes[count++] = ts_tree_cursor_current_node(&cursor); }
      while (ts_tree_cursor_goto_next_sibling(&cursor));
      assert(count == 3 + positive_tail);
      for (unsigned i = 0; i < count; i++) {
        assert(ts_node_eq(ts_node_parent(nodes[i]), root));
        TSNode next = ts_node_next_sibling(nodes[i]);
        assert(i + 1 < count ? ts_node_eq(next, nodes[i + 1]) : ts_node_is_null(next));
        unsigned j = i + 1;
        while (j < count && !ts_node_is_named(nodes[j])) j++;
        next = ts_node_next_named_sibling(nodes[i]);
        assert(j < count ? ts_node_eq(next, nodes[j]) : ts_node_is_null(next));
        if (i < 3) assert(ts_node_start_byte(nodes[i]) == 0 && ts_node_end_byte(nodes[i]) == 0);
        for (unsigned k = 0; k < i; k++) assert(!ts_node_eq(nodes[i], nodes[k]));
      }
      assert(ts_node_is_null(ts_node_next_sibling(root)));
      TSTree *copy = ts_tree_copy(tree);
      assert(!ts_node_eq(ts_tree_root_node(copy), root));
      ts_tree_delete(copy);
      ts_tree_cursor_delete(&cursor);
      ts_tree_delete(tree);
      ts_subtree_pool_delete(&pool);
    }
  }
  puts("three zero-width identities, mixed named/anonymous, hidden wrapper, EOF and cross-tree: PASS");
  return 0;
}
