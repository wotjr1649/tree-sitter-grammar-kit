# Ordinary JSON grammar fixture

`parser.c` is the generated tree-sitter-json 0.24.8 parser (23,126 bytes, SHA-256
`86971e7cb4e6dae9c5395bedf51abf525b2b662c6bcb632d33b72e4519aaab83`, MIT licensed upstream). It is byte-identical to the
`src/parser.c` member the P0 registry pins for the `json` route. The three runtime headers the route also pins
(`tree_sitter/alloc.h`, `array.h`, `parser.h`) are byte-identical to `src/testdata/native/plain/src/tree_sitter/`, so they are
not copied again. The ordinary full-positive regression assembles a grammar root from these files and compares every member
with the registry pin before it runs the owned native producer.
