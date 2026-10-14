# #208 Large native tree digest cost investigation

[Issue #208](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/208) tracks the Windows 32 MiB C# oracle timeout in PR #207. The [failed CI run](https://github.com/wotjr1649/tree-sitter-grammar-kit/actions/runs/38598435159) retained its original 90-second wall, 60-second parse and 8 GiB limits. The failure was preserved without reclassification or an unchanged rerun.

The input is `cs-large-32mib-errors`: 33,554,432 bytes, SHA-256 `6409672e6b17cdf655cd975cc908992aabe8ad4ba4537d95323956de4bc7fa3e`. The CI oracle reached WALL_LIMIT at 90,398 ms with peak memory 5,168,136,192 bytes. The ordinary parse in that CI run completed with parse 52,292 ms and process 88,334 ms, traversing 17,478,169 nodes. The ordinary 26 routes and oracle 26 routes also passed. These results do not establish large-case stability or causation by the Go grammar change. The driver was identical in the earlier main and PR #207.

`digest_node` called `snprintf` 9 times per node for numeric formatting: 157,303,521 calls on this input. The change reuses existing `puti`/`putu` with a 24-byte stack buffer. The largest segment is 22 bytes: NUL, a 20-digit integer and colon, requiring no heap allocation. Parent is -1 for the root or a node index; coordinates are uint32. NUL separators, decimal lengths, flags, strings, coordinate order and LF retain the [digest contract](../specs/tree-and-adapter-protocol.md). This does not claim safety for every input to a general int64 formatter.

The following local observations used Windows/amd64, Go 1.27.2, GCC 16.2.0 on 2026-10-15, with the same pinned runtime, C# grammar and input as CI.

| Observation | Original | Candidate |
|---|---:|---:|
| Same 32 MiB oracle native process wall (ms) | 81,801 | 61,089 |
| Parse in that execution (ms) | 44,470 | 37,714 |
| Peak job committed memory (bytes) | 5,201,711,104 | 5,201,707,008 |
| Real runtime, owned plain grammar, 1,000,000-node digest kernel, 3 trials (ms) | 750 / 550 / 532 | 341 / 290 / 268 |

Both large executions were COMPLETED/PASS under the original limits, with fact reproduction PASS. Node counts and digest `4fb32d84e29573dd76f84511ff0552173b3ab5f8a95b455e3ff601d6d8eed38e` matched. The kernel used actual Tree-sitter nodes. NUL-inclusive byte comparisons at numeric boundaries, root parents, empty/UTF-8 fields, 32 flag combinations and all three paired digests matched. This kernel measurement is distinct from whole-parser performance.

The whole-process wall difference includes parse-time differences and environmental variation. A single large-case pair establishes neither a general speedup nor permanent resolution of the CI timeout. Compiler bytes also differ between CI and the local host. These are local investigation results, not a new three-platform support verdict.

`TestSummaryGate` adds independent Go `TreeDigest` validation of the full response to the existing full/summary equality check and passes the actual process exit code. The full Go suite, native owned suite, vet, build, two STATIC context reviews, fresh mandatory CI and actual main verification remain checks before final integration. The right-pane review is STATIC and provides neither independent execution nor formal approval. Original limits, inputs, queries, coverage and failure propagation remain in place. Release/tag publication and restarting the paused real consumer are outside this change.
