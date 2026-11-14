# 저장소 종료 인계 B — grammar 결함·patch와 26-route 기능 범위

작성 기준: tree-sitter-grammar-kit `origin/main`의 `8f2ed6f0c9563364b5e0fdc77bc8b0f8cb1d5a3d`. 아래 수치는 이 revision의 추적 파일을 정적으로 해독·집계한 값이다.

[사용자의 저장소 종료 결정](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/195#issuecomment-6875080857)과 [go-treesitter ADR-0020](https://github.com/wotjr1649/go-treesitter/blob/main/docs/design/decisions/ADR-0020-cgo-free-library-on-unmodified-tree-sitter.md)에 따라 grammar-kit의 지식·사례를 인계한다. go-treesitter는 upstream C를 수정하지 않고 사용하므로 아래 patch를 그대로 이식하는 것이 아니라 upstream 기여 후보와 측정할 known gap을 고르는 자료로 쓴다. `#427` 작업은 재개하지 않았다. 두 저장소의 제품·계약·코드는 수정하지 않는다.

## B1. Grammar 결함 및 patch 인벤토리

### 기준·집계 단위

정본은 [native-routes.json](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/contracts/native-routes.json)의 각 `routes[].regeneration.patch_chain`이다. 각 entry의 `subject` 파일을 읽고 `pointer`로 찾아 `field`가 지정한 배열 길이를 계산했다. `operations`만 세지 않고 `replacements`도 포함한다. `occurrences`는 한 operation의 적용 횟수이므로 operation 수에 곱하지 않았다. 같은 target을 여러 단계에서 바꾸면 단계별로 센다. 생성된 `parser.c`·`grammar.json`·`node-types.json`·headers는 이 chain의 출력으로, 별도의 수동 grammar patch로 세지 않는다.

- 26 routes / 24 upstream repositories / 25 parsers. JavaScript와 JSX는 동일 parser이며 TypeScript/TSX는 같은 repository의 서로 다른 parser다.
- **97개 chain entry / 1,423개 literal operations / C2 defect label 503개**. 모두 route별 합계로서 독립 root cause 수가 아니다.
- 모든 route에 C2 patch가 있다. `native-routes.json`의 “나머지 20개는 무패치” 문구는 C2 추가 전 출발 상태이며 실제 현재 chain이 우선한다.
- JavaScript/JSX의 C2 `files` 배열과 TypeScript/TSX의 C2 `files` 배열은 각각 동일하지만 route별 label·case 수는 다르다. 중복 route의 operations도 각 route에서 센다.
- C2 `defects`는 라벨 목록이며 operation↔defect 일대일 대응은 기록하지 않는다. 아래 설명은 파일별 변경·대표 construct를 묶은 것이고, 정확한 전후 bytes와 모든 operation은 링크의 JSON pointer 및 배열 순서가 소유한다. 명시적인 개별 원인 기록이 없는 라벨에는 원인을 새로 꾸며 붙이지 않는다.
- 아래의 모든 grammar/scanner patch에 대해 **현재 upstream 현존 여부는 `unknown`**이다. 지정된 종료 결정 댓글은 읽었지만 upstream 이슈·PR·최신 branch를 온라인 조사하지 않았다. 고정 pin에 patch가 필요했다는 기록은 현재 upstream 잔존 증명이 아니다.
- 처분 기본값은 **upstream 기여 후보**이며, unmodified upstream으로 다시 재현하기 전에는 **측정할 known gap 후보**다. C/C++ 미전개 macro heuristic, C# branch 선택·fragment 정책, version 합집합·SQLCMD·CST 모양과 관련된 정책 변경은 upstream 범위 확인이 먼저다.
- ★는 wave 1이다. TypeScript/TSX를 각각 세어 **wave 1은 7 routes**, wave 2는 19 routes다.

초기 remedy의 출처는 [#20](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/20), [P05 source closure](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/docs/validation/p05-source-closure.md), [source feasibility](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/docs/validation/source-feature-feasibility.md)다. 공통 C2 기능 대안 보완은 [#76](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/76)과 `src/dev/c2-patches/<route>.json`에 기록된다. 재현 가능한 source/edit/expectation은 `src/testdata/native/routes/<route>.json` 및 존재하는 `gaps/<route>.json`에 있다. 파일별 설명의 구문 예는 public patch/fixture의 construct이며 비공개 corpus 원문을 포함하지 않는다.

### route별 upstream pin 및 patch 수

아래 commit은 기반 upstream source identity다. kit 결과는 해당 commit에 chain을 순서대로 적용하고 재생성한 별도 identity다.

| Route | upstream repository / pinned commit | chain entries | operations | C2 labels |
|---|---|---:|---:|---:|
| ★ `csharp` | [tree-sitter/tree-sitter-c-sharp](https://github.com/tree-sitter/tree-sitter-c-sharp/tree/9150f7d56bb47f1a809fa23623f1ba1413e93fa9) / `9150f7d56bb47f1a809fa23623f1ba1413e93fa9` | 8 | 122 | 24 |
| ★ `go` | [tree-sitter/tree-sitter-go](https://github.com/tree-sitter/tree-sitter-go/tree/2346a3ab1bb3857b48b29d779a1ef9799a248cd7) / `2346a3ab1bb3857b48b29d779a1ef9799a248cd7` | 2 | 14 | 5 |
| ★ `python` | [tree-sitter/tree-sitter-python](https://github.com/tree-sitter/tree-sitter-python/tree/26855eabccb19c6abf499fbc5b8dc7cc9ab8bc64) / `26855eabccb19c6abf499fbc5b8dc7cc9ab8bc64` | 2 | 20 | 10 |
| `javascript` | [tree-sitter/tree-sitter-javascript](https://github.com/tree-sitter/tree-sitter-javascript/tree/58404d8cf191d69f2674a8fd507bd5776f46cb11) / `58404d8cf191d69f2674a8fd507bd5776f46cb11` | 2 | 57 | 9 |
| `jsx` | [tree-sitter/tree-sitter-javascript](https://github.com/tree-sitter/tree-sitter-javascript/tree/58404d8cf191d69f2674a8fd507bd5776f46cb11) / `58404d8cf191d69f2674a8fd507bd5776f46cb11` | 2 | 57 | 6 |
| ★ `typescript` | [tree-sitter/tree-sitter-typescript](https://github.com/tree-sitter/tree-sitter-typescript/tree/75b3874edb2dc714fb1fd77a32013d0f8699989f) / `75b3874edb2dc714fb1fd77a32013d0f8699989f` | 4 | 17 | 9 |
| ★ `tsx` | [tree-sitter/tree-sitter-typescript](https://github.com/tree-sitter/tree-sitter-typescript/tree/75b3874edb2dc714fb1fd77a32013d0f8699989f) / `75b3874edb2dc714fb1fd77a32013d0f8699989f` | 4 | 17 | 10 |
| `java` | [tree-sitter/tree-sitter-java](https://github.com/tree-sitter/tree-sitter-java/tree/e10607b45ff745f5f876bfa3e94fbcc6b44bdc11) / `e10607b45ff745f5f876bfa3e94fbcc6b44bdc11` | 1 | 9 | 7 |
| `kotlin` | [tree-sitter-grammars/tree-sitter-kotlin](https://github.com/tree-sitter-grammars/tree-sitter-kotlin/tree/3dea6dfa9c0129deb7c4315afbda806c85c41667) / `3dea6dfa9c0129deb7c4315afbda806c85c41667` | 2 | 53 | 22 |
| `c` | [tree-sitter/tree-sitter-c](https://github.com/tree-sitter/tree-sitter-c/tree/b780e47fc780ddc8da13afa35a3f4ed5c157823d) / `b780e47fc780ddc8da13afa35a3f4ed5c157823d` | 1 | 78 | 27 |
| `cpp` | [tree-sitter/tree-sitter-cpp](https://github.com/tree-sitter/tree-sitter-cpp/tree/c009222808634c1014f82438d4883753516a2c24) / `c009222808634c1014f82438d4883753516a2c24` | 3 | 150 | 50 |
| `rust` | [tree-sitter/tree-sitter-rust](https://github.com/tree-sitter/tree-sitter-rust/tree/77a3747266f4d621d0757825e6b11edcbf991ca5) / `77a3747266f4d621d0757825e6b11edcbf991ca5` | 2 | 26 | 17 |
| `swift` | [alex-pinkus/tree-sitter-swift](https://github.com/alex-pinkus/tree-sitter-swift/tree/35245fbfee2fccf16273c6f4299438fb76875970) / `35245fbfee2fccf16273c6f4299438fb76875970` | 3 | 83 | 38 |
| `dart` | [UserNobody14/tree-sitter-dart](https://github.com/UserNobody14/tree-sitter-dart/tree/be07cf7118d3dba06236a3f19541685a68209934) / `be07cf7118d3dba06236a3f19541685a68209934` | 1 | 30 | 14 |
| `php` | [tree-sitter/tree-sitter-php](https://github.com/tree-sitter/tree-sitter-php/tree/92b5271b60bec77fb65b5e5bc41561e8dac81299) / `92b5271b60bec77fb65b5e5bc41561e8dac81299` | 2 | 22 | 11 |
| `ruby` | [tree-sitter/tree-sitter-ruby](https://github.com/tree-sitter/tree-sitter-ruby/tree/ad907a69da0c8a4f7a943a7fe012712208da6dee) / `ad907a69da0c8a4f7a943a7fe012712208da6dee` | 2 | 6 | 6 |
| `r` | [r-lib/tree-sitter-r](https://github.com/r-lib/tree-sitter-r/tree/58a22794466c0fc15b0d3b40531db751593721e8) / `58a22794466c0fc15b0d3b40531db751593721e8` | 2 | 8 | 3 |
| `bash` | [tree-sitter/tree-sitter-bash](https://github.com/tree-sitter/tree-sitter-bash/tree/a06c2e4415e9bc0346c6b86d401879ffb44058f7) / `a06c2e4415e9bc0346c6b86d401879ffb44058f7` | 2 | 25 | 15 |
| `powershell` | [airbus-cert/tree-sitter-powershell](https://github.com/airbus-cert/tree-sitter-powershell/tree/e7bd348c49fdfd5c853a146a670965ba516a6239) / `e7bd348c49fdfd5c853a146a670965ba516a6239` | 2 | 46 | 23 |
| `html` | [tree-sitter/tree-sitter-html](https://github.com/tree-sitter/tree-sitter-html/tree/73a3947324f6efddf9e17c0ea58d454843590cc0) / `73a3947324f6efddf9e17c0ea58d454843590cc0` | 3 | 17 | 12 |
| `css` | [tree-sitter/tree-sitter-css](https://github.com/tree-sitter/tree-sitter-css/tree/dda5cfc5722c429eaba1c910ca32c2c0c5bb1a3f) / `dda5cfc5722c429eaba1c910ca32c2c0c5bb1a3f` | 2 | 32 | 28 |
| ★ `json` | [tree-sitter/tree-sitter-json](https://github.com/tree-sitter/tree-sitter-json/tree/254c42a6476413b776221e03982ac8ae159eeb72) / `254c42a6476413b776221e03982ac8ae159eeb72` | 1 | 5 | 4 |
| `yaml` | [tree-sitter-grammars/tree-sitter-yaml](https://github.com/tree-sitter-grammars/tree-sitter-yaml/tree/a1c4812a73ec5e089de8e441fdea3a921e8d5079) / `a1c4812a73ec5e089de8e441fdea3a921e8d5079` | 1 | 1 | 1 |
| `xml` | [tree-sitter-grammars/tree-sitter-xml](https://github.com/tree-sitter-grammars/tree-sitter-xml/tree/5000ae8f22d11fbe93939b05c1e37cf21117162d) / `5000ae8f22d11fbe93939b05c1e37cf21117162d` | 4 | 13 | 7 |
| ★ `tsql` | [meloncholera/tree-sitter-mssql](https://github.com/meloncholera/tree-sitter-mssql/tree/8620fbcfca9438e1ff7104835bcb8305aba3bdfa) / `8620fbcfca9438e1ff7104835bcb8305aba3bdfa` | 36 | 494 | 136 |
| `postgresql-sql` | [gmr/tree-sitter-postgres](https://github.com/gmr/tree-sitter-postgres/tree/59d0d8cd7506d68de1229fb4bbce838c83b60c8a) / `59d0d8cd7506d68de1229fb4bbce838c83b60c8a` | 3 | 21 | 9 |
| **합계** | **26 routes** | **97** | **1,423** | **503** |

TypeScript/TSX의 `node_modules/tree-sitter-javascript/grammar.js`와 C++의 `node_modules/tree-sitter-c/grammar.js`는 parent grammar git commit과 다른 dependency다. [reproduction-routes.json](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/contracts/reproduction-routes.json)의 npm pin을 적용해야 한다.

| 소비 route | dependency upstream | 고정 artifact·git commit | integrity |
|---|---|---|---|
| typescript / tsx | tree-sitter/tree-sitter-javascript | npm tree-sitter-javascript@0.23.1; 대응 git commit은 이 registry에 미기록(unknown) | `sha512-/bnhbrTD9frUYHQTiYnPcxyHORIw157ERBa6dqzaKxvR/x3PC4Yzd+D1pZIMS6zNg2v3a8BZ0oK7jHqsQo9fWA==` |
| cpp | tree-sitter/tree-sitter-c | npm tree-sitter-c@0.24.1; 대응 git commit은 이 registry에 미기록(unknown) | `sha512-lkYwWN3SRecpvaeqmFKkuPNR3ZbtnvHU+4XAEEkJdrp3JfSp2pBrhXOtvfsENUneye76g889Y0ddF2DM0gEDpA==` |

### 적용 순서대로 전수 열거한 patch 항목

번호는 각 route의 chain 순서다. Operation 주소는 표의 pointer 뒤 `/<field>/<0..N-1>`이며 before/after(또는 old/new)가 정확한 변경 정본이다. Source 링크와 target은 kit/해당 upstream 기준 경로로 구분한다. 모든 행의 upstream pin은 직전 route별 표를 상속하며 현재 upstream 상태 `unknown`, 기본 처분 upstream 기여/known-gap 재측정 후보를 상속한다.

`src/dev/prepare-p05/`의 proposal·case·expectation amendment 전체를 적용 patch로 세지 않는다. 실제 chain에서 선택된 subject/pointer만 아래에 있다. `PROPOSED`·`applied=false` 같은 원 proposal의 역사 metadata를 현재 chain 채택 여부로 대신하지 않는다.

#### ★ csharp: 적용 chain

| 순서 | patch subject 및 JSON pointer | target | field / 수 | 변경·실패 construct·근거 |
|---:|---|---|---|---|
| 1 | [`prepare-p05/remedy-patches.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/prepare-p05/remedy-patches.json) `/patches/0/files/0` | `grammar.js` | `operations` / 5 | async modifier/identifier와 file-based directive를 교정한다. 아래 C# 상세 표의 r1 grammar. |
| 2 | [`prepare-p05/remedy-patches.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/prepare-p05/remedy-patches.json) `/patches/0/files/1` | `src/scanner.c` | `operations` / 2 | 행 시작 #:만 인식하며 string/interpolation 내부는 배제한다. 아래 C# r1 scanner. |
| 3 | [`prepare-p05/remedy-r2.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/prepare-p05/remedy-r2.json) `/patches/0/file` | `grammar.js` | `operations` / 3 | await contextual identifier와 반환형/new type의 var를 보존한다. 아래 C# r2. |
| 4 | [`prepare-p05/remedy-exact-r1.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/prepare-p05/remedy-exact-r1.json) `/patches/0/change` | `grammar.js` | `operations` / 1 | identifier/implicit_type/object creation conflict를 추가한다. 아래 C# r3. |
| 5 | [`prepare-p05/remedy-followup-r2.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/prepare-p05/remedy-followup-r2.json) `/patches/0/files/0` | `grammar.js` | `operations` / 1 | 위 conflict에 method return type을 추가한다. 아래 C# r4. |
| 6 | [`prepare-p05/remedy-csharp-r5.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/prepare-p05/remedy-csharp-r5.json) `/patch` | `grammar.js` | `replacements` / 2 | var 반환형/new var()의 잘못된 implicit_type 선택을 dynamic precedence로 교정한다. 아래 C# r5. |
| 7 | [`c2-patches/csharp.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/c2-patches/csharp.json) `/files/0` | `grammar.js` | `operations` / 66 | lexical/identifier/directive·conditional fragment·string recovery를 교정한다. 아래 C# 상세 표의 C2 grammar와 #157–#184·#233 근거. |
| 8 | [`c2-patches/csharp.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/c2-patches/csharp.json) `/files/1` | `src/scanner.c` | `operations` / 42 | interpolation·directive/inactive state·line/pragma/string/comment recovery를 교정한다. 아래 C# 상세 표의 C2 scanner와 #233 최종 회귀. |

C2 등록 defect labels (24): `csharp-P2-04`, `csharp-P2-02`, `csharp-P2-06`, `csharp-P2-07`, `csharp-P2-08`, `csharp-P2-09`, `csharp-P2-10`, `csharp-P2-05`, `csharp-P2-PP`, `csharp-P2-01`, `csharp-P2-03`, `csharp-P2-11`, `issue152-identifier-categories`, `issue153-directive-eof`, `issue157-directive-message-boundary`, `issue160-preprocessor-expression`, `issue161-conditional-symbol`, `issue162-line-lexical-form`, `issue163-general-pragma`, `issue179-inactive-directives`, `issue182-conditional-syntax-fragments`, `issue184-endif-delimited-comment`, `issue233-producer-lexical-boundaries`, `issue233-line-filename-comment-adjacency`.

#### ★ go: 적용 chain

| 순서 | patch subject 및 JSON pointer | target | field / 수 | 변경·실패 construct·근거 |
|---:|---|---|---|---|
| 1 | [`c2-patches/go.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/c2-patches/go.json) `/files/0` | `grammar.js` | `operations` / 13 | method type_parameters·다중 index·trailing comma를 추가하고 top-level을 declaration으로 제한한다. automatic semicolon을 external token으로 옮겨 if/for/switch header·이름·receiver·type parameter·composite type 뒤 행끝을 검사한다. identifier를 XID에서 Unicode17 L/Nd/_로 교정한다. 예: type T\nint, f(a\n), U+200C 거부·U+037A 허용. #76·#98·#205. |
| 2 | [`c2-patches/go.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/c2-patches/go.json) `/files/1` | `src/scanner.c` | `operations` / 1 | 신규 scanner(create=true). LF/EOF/개행 포함 block comment의 automatic semicolon과 \|\|·선언 이름 경계를 검사한다. #98. 타입 뒤 괄호/대괄호 안 newline 허용은 기록된 한계다. |

C2 등록 defect labels (5): `go-P2-01`, `go-P2-02`, `go-P2-03`, `go-P2-04`, `go-P2-05`.

#### ★ python: 적용 chain

| 순서 | patch subject 및 JSON pointer | target | field / 수 | 변경·실패 construct·근거 |
|---:|---|---|---|---|
| 1 | [`c2-patches/python.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/c2-patches/python.json) `/files/0` | `grammar.js` | `operations` / 14 | bitwise &amp;/xor·await/power 우선순위, positional-only / 및 bare/star/typed-star/** parameter 순서, except/except* 혼합·대상·괄호 없는 expression list, PEP696 type parameter default/type alias 이름, f/t-string format_spec를 교정한다. 예: power 결합, except*: 및 except 혼합. |
| 2 | [`c2-patches/python.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/c2-patches/python.json) `/files/1` | `src/scanner.c` | `operations` / 6 | lone CR을 newline으로 처리하고 stack에 없는 inconsistent dedent를 ERROR로 만든다. format-spec가 outer closing quote·single-line terminator를 넘지 않게 한다. |

C2 등록 defect labels (10): `python-P2-01`, `python-P2-02`, `python-P2-03`, `python-P2-04`, `python-P2-05`, `python-P2-06`, `python-P2-07`, `python-P2-08`, `python-P2-09`, `python-P2-10`.

#### javascript: 적용 chain

| 순서 | patch subject 및 JSON pointer | target | field / 수 | 변경·실패 construct·근거 |
|---:|---|---|---|---|
| 1 | [`c2-patches/javascript.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/c2-patches/javascript.json) `/files/0` | `grammar.js` | `operations` / 38 | ??와 &amp;&amp;/\|\| 혼합·우선순위, re-export attributes, export default anonymous function/class, Annex B if/else plain function 허용과 loop/with 선언 거부, JSX spread-only attribute, optional-chain new constructor 거부, unary/await ** base 거부, template NotEscapeSequence, function/generator/arrow concise body의 yield 문맥과 class field initializer [?Yield]를 교정한다. 예: a??b\|\|c, while(x) function f(){}, new a?.b(). #76·#95. |
| 2 | [`c2-patches/javascript.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/c2-patches/javascript.json) `/files/1` | `src/scanner.c` | `operations` / 19 | forbidden clause-start의 ERROR lookahead, function/generator/concise-body frame 직렬화와 YIELD_KEYWORD, arrow body 종료·return/yield ASI를 처리한다. 예: function* g(){()=&gt;yield}. #76·#95. |

C2 등록 defect labels (9): `javascript-P2-01`, `javascript-P2-02`, `javascript-P2-03`, `javascript-P2-04`, `javascript-P2-05`, `javascript-P2-06`, `javascript-P2-07`, `javascript-P2-09`, `javascript-P2-10`.

#### jsx: 적용 chain

| 순서 | patch subject 및 JSON pointer | target | field / 수 | 변경·실패 construct·근거 |
|---:|---|---|---|---|
| 1 | [`c2-patches/jsx.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/c2-patches/jsx.json) `/files/0` | `grammar.js` | `operations` / 38 | ??와 &amp;&amp;/\|\| 혼합·우선순위, re-export attributes, export default anonymous function/class, Annex B if/else plain function 허용과 loop/with 선언 거부, JSX spread-only attribute, optional-chain new constructor 거부, unary/await ** base 거부, template NotEscapeSequence, function/generator/arrow concise body의 yield 문맥과 class field initializer [?Yield]를 교정한다. 예: a??b\|\|c, while(x) function f(){}, new a?.b(). #76·#95. |
| 2 | [`c2-patches/jsx.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/c2-patches/jsx.json) `/files/1` | `src/scanner.c` | `operations` / 19 | forbidden clause-start의 ERROR lookahead, function/generator/concise-body frame 직렬화와 YIELD_KEYWORD, arrow body 종료·return/yield ASI를 처리한다. 예: function* g(){()=&gt;yield}. #76·#95. |

C2 등록 defect labels (6): `jsx-P2-01`, `jsx-P2-02`, `jsx-P2-03`, `jsx-P2-04`, `jsx-P2-05`, `jsx-P2-06`.

#### ★ typescript: 적용 chain

| 순서 | patch subject 및 JSON pointer | target | field / 수 | 변경·실패 construct·근거 |
|---:|---|---|---|---|
| 1 | [`prepare-p05/remedy-patches.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/prepare-p05/remedy-patches.json) `/patches/1/files/0` | `common/define-grammar.js` | `operations` / 1 | import defer * as ns from… branch를 추가하고 기존 import는 보존한다. P05-TS-TSX-REMEDY-r1, #20. |
| 2 | [`prepare-p05/remedy-r2.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/prepare-p05/remedy-r2.json) `/patches/1/file` | `common/define-grammar.js` | `operations` / 1 | default/named/alias import에서 defer를 identifier로 허용한다. P05-TS-TSX-REMEDY-r2, #20. |
| 3 | [`c2-patches/typescript.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/c2-patches/typescript.json) `/files/0` | `common/define-grammar.js` | `operations` / 11 | generic arrow/JSX ambiguity, accessor/abstract/readonly modifier, generic tagged template, using 위치, type re-export·import attributes·decorator expression·static index signature·bigint·in/out variance를 교정한다. 예: const f=&lt;T,&gt;(x:T)=&gt;x, export type * from..., accessor x, tag&lt;T&gt; 뒤 template. |
| 4 | [`c2-patches/typescript.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/c2-patches/typescript.json) `/files/1` | `node_modules/tree-sitter-javascript/grammar.js` | `operations` / 4 | re-export import attributes와 using/await using declaration·for-in/of left binding을 추가·분리한다. 대상은 npm tree-sitter-javascript@0.23.1 dependency이며 현재 JavaScript route pin으로 바꾸지 않는다. |

C2 등록 defect labels (9): `typescript-P2-01`, `typescript-P2-02`, `typescript-P2-03`, `typescript-P2-04`, `typescript-P2-05`, `typescript-P2-06`, `typescript-P2-07`, `typescript-P2-08`, `typescript-P2-09`.

#### ★ tsx: 적용 chain

| 순서 | patch subject 및 JSON pointer | target | field / 수 | 변경·실패 construct·근거 |
|---:|---|---|---|---|
| 1 | [`prepare-p05/remedy-patches.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/prepare-p05/remedy-patches.json) `/patches/1/files/0` | `common/define-grammar.js` | `operations` / 1 | import defer * as ns from… branch를 추가하고 기존 import는 보존한다. P05-TS-TSX-REMEDY-r1, #20. |
| 2 | [`prepare-p05/remedy-r2.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/prepare-p05/remedy-r2.json) `/patches/1/file` | `common/define-grammar.js` | `operations` / 1 | default/named/alias import에서 defer를 identifier로 허용한다. P05-TS-TSX-REMEDY-r2, #20. |
| 3 | [`c2-patches/tsx.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/c2-patches/tsx.json) `/files/0` | `common/define-grammar.js` | `operations` / 11 | generic arrow/JSX ambiguity, accessor/abstract/readonly modifier, generic tagged template, using 위치, type re-export·import attributes·decorator expression·static index signature·bigint·in/out variance를 교정한다. 예: const f=&lt;T,&gt;(x:T)=&gt;x, export type * from..., accessor x, tag&lt;T&gt; 뒤 template. |
| 4 | [`c2-patches/tsx.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/c2-patches/tsx.json) `/files/1` | `node_modules/tree-sitter-javascript/grammar.js` | `operations` / 4 | re-export import attributes와 using/await using declaration·for-in/of left binding을 추가·분리한다. 대상은 npm tree-sitter-javascript@0.23.1 dependency이며 현재 JavaScript route pin으로 바꾸지 않는다. |

C2 등록 defect labels (10): `tsx-P2-01`, `tsx-P2-02`, `tsx-P2-03`, `tsx-P2-04`, `tsx-P2-05`, `tsx-P2-06`, `tsx-P2-07`, `tsx-P2-08`, `tsx-P2-09`, `tsx-P2-10`.

#### java: 적용 chain

| 순서 | patch subject 및 JSON pointer | target | field / 수 | 변경·실패 construct·근거 |
|---:|---|---|---|---|
| 1 | [`c2-patches/java.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/c2-patches/java.json) `/files/0` | `grammar.js` | `operations` / 9 | 숫자 연속 underscore(0b/0x/octal), EOF SUB U+001A, switch comma-separated patterns, import module, explicit constructor invocation 앞 statement, vararg ... 앞 annotation, identifier Unicode escape를 추가한다. 예: 1__2, import module java.base;, C(){stmt();super();}, \u0061. |

C2 등록 defect labels (7): `java-P2-01`, `java-P2-02`, `java-P2-03`, `java-P2-04`, `java-P2-05`, `java-P2-06`, `java-P2-07`.

#### kotlin: 적용 chain

| 순서 | patch subject 및 JSON pointer | target | field / 수 | 변경·실패 construct·근거 |
|---:|---|---|---|---|
| 1 | [`c2-patches/kotlin.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/c2-patches/kotlin.json) `/files/0` | `src/scanner.c` | `operations` / 17 | class } zero-width terminator, raw quote run, else/in/get/set/constructor/by/as/where/field 연속성, accessor annotation, loop body newline hint, backing field marker, multi-dollar state/직렬화·중첩·quote 종료를 처리한다. |
| 2 | [`c2-patches/kotlin.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/c2-patches/kotlin.json) `/files/1` | `grammar.js` | `operations` / 36 | 숫자 underscore/exponent·CR comment/shebang·invalid Unicode escape, annotation priority·variance·suspend anonymous function·@all, trailing comma·typed destructured lambda·optional type parameters, Unicode/backtick interpolation·raw quote, when guard, loop newline·generic assignment lhs, explicit backing field·context parameter/type·contextual dynamic/suspend·multi-dollar string·do-while ambiguity를 교정한다. 예: context(x:T), val p=1 field:Int=2, multi-dollar interpolation. |

C2 등록 defect labels (22): `kotlin-P2-01`, `kotlin-P2-03`, `kotlin-P2-04`, `kotlin-P2-16`, `kotlin-P2-05`, `kotlin-P2-06`, `kotlin-P2-08`, `kotlin-P2-11`, `kotlin-P2-22`, `kotlin-P2-19`, `kotlin-P2-20`, `kotlin-P2-13`, `kotlin-P2-15`, `kotlin-P2-09`, `kotlin-P2-07`, `kotlin-P2-14`, `kotlin-P2-18`, `kotlin-P2-21`, `kotlin-P2-12`, `kotlin-P2-10`, `kotlin-P2-02`, `kotlin-P2-17`.

#### c: 적용 chain

| 순서 | patch subject 및 JSON pointer | target | field / 수 | 변경·실패 construct·근거 |
|---:|---|---|---|---|
| 1 | [`c2-patches/c.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/c2-patches/c.json) `/files/0` | `grammar.js` | `operations` / 78 | C99–C23 _Bool/Decimal/Complex/Imaginary·_BitInt·_Thread_local·constexpr/_Noreturn, _Atomic/typeof/auto, attribute slot·위치·balanced token, compound literal storage·_Generic default, enum underlying type·label/bitfield/static_assert, _Pragma/null directive·조건부 directive, __has_include/next/embed/c_attribute, initializer preproc·#if 0 skipped source·digraph, attributed case/field 경계를 교정한다. 예: _Atomic(int), auto x=1, enum E:unsigned int{A}. #101 macro type/preproc attribute는 미전개 source heuristic이다. |

C2 등록 defect labels (27): `c-P2-01`, `c-P2-02`, `c-P2-03`, `c-P2-04`, `c-P2-05`, `c-P2-06`, `c-P2-07`, `c-P2-08`, `c-P2-09`, `c-P2-10`, `c-P2-11`, `c-P2-12`, `c-P2-13`, `c-P2-14`, `c-P2-15`, `c-P2-16`, `c-P2-17`, `c-P2-18`, `c-P2-19`, `c-P2-20`, `c-P2-21`, `c-P2-22`, `c-P2-23`, `c-P2-24`, `c-P2-25`, `c-P3-01`, `c-P3-02`.

#### cpp: 적용 chain

| 순서 | patch subject 및 JSON pointer | target | field / 수 | 변경·실패 construct·근거 |
|---:|---|---|---|---|
| 1 | [`c2-patches/cpp.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/c2-patches/cpp.json) `/files/0` | `node_modules/tree-sitter-c/grammar.js` | `operations` / 32 | C dependency 전용 patch. attribute slot/enumerator/pointer·balanced args, _Pragma/null directive, __has_include/next/embed/c_attribute/cpp_attribute, #if0 skipped source, digraph/trigraph directive/escape 및 C++23 delimited/named escape·wchar_t를 추가한다. c route의 전체 patch와 다르다. dependency pin은 tree-sitter-c@0.24.1. |
| 2 | [`c2-patches/cpp.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/c2-patches/cpp.json) `/files/1` | `grammar.js` | `operations` / 116 | digraph/trigraph·&lt;:: 예외, attribute using/pack·enum/reference, deduction guide scope, if consteval, template operator·typeid/noexcept·destructor·member-pointer, explicit instantiation·designated init/coroutine·explicit(expr)/conversion·dependent type/auto, mutable·requires·parenthesized new/array attribute·typename using·parameter pack·bitfield·pure virtual·decltype base·constrained parameter·exception attr·legacy import/header-name/&lt;=를 교정한다. 예: requires{typename T::x; requires C&lt;T&gt;;}, new (T), template class X;. #101 macro_invocation/specifier/constructor 및 type-pack heuristic을 포함한다. |
| 3 | [`c2-patches/cpp.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/c2-patches/cpp.json) `/files/2` | `src/scanner.c` | `operations` / 2 | DIGRAPH_OPEN_BRACKET/LEGACY_LESS_EQUAL로 &lt;:: lexical 예외를 지키며 &lt;:와 legacy &lt;= 경계를 구분한다. |

C2 등록 defect labels (50): `cpp-P2-01`, `cpp-P2-02`, `cpp-P2-03`, `cpp-P2-04`, `cpp-P2-05`, `cpp-P2-06`, `cpp-P2-07`, `cpp-P2-08`, `cpp-P2-09`, `cpp-P2-10`, `cpp-P2-11`, `cpp-P2-12`, `cpp-P2-13`, `cpp-P2-14`, `cpp-P2-15`, `cpp-P2-16`, `cpp-P2-17`, `cpp-P2-18`, `cpp-P2-19`, `cpp-P2-20`, `cpp-P2-21`, `cpp-P2-22`, `cpp-P2-23`, `cpp-P2-24`, `cpp-P2-25`, `cpp-P2-26`, `cpp-P2-27`, `cpp-P2-28`, `cpp-P2-29`, `cpp-P2-30`, `cpp-P2-31`, `cpp-P2-32`, `cpp-P2-33`, `cpp-P2-34`, `cpp-P2-35`, `cpp-P2-36`, `cpp-P2-37`, `cpp-P2-38`, `cpp-P2-39`, `cpp-P2-40`, `cpp-P2-41`, `cpp-P2-42`, `cpp-P2-43`, `cpp-P2-44`, `cpp-P2-45`, `cpp-P3-01`, `cpp-P3-02`, `cpp-P3-03`, `cpp-P3-04`, `cpp-P3-05`.

#### rust: 적용 chain

| 순서 | patch subject 및 JSON pointer | target | field / 수 | 변경·실패 construct·근거 |
|---:|---|---|---|---|
| 1 | [`c2-patches/rust.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/c2-patches/rust.json) `/files/0` | `grammar.js` | `operations` / 25 | whitespace 집합, unsafe(attribute), safe/unsafe extern·contextual safe, empty trait bounds, let/parameter/closure top-level or-pattern 제한, raw ABI string·turbofish·negative const generic·inclusive range·empty struct update·parameter attrs/trailing comma·numeric field pattern·literal suffix·Unicode escape underscore·raw metavariable를 교정한다. 예: unsafe(no_mangle), let a\|b=..., extern r"C", S{..}, 1foo. |
| 2 | [`c2-patches/rust.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/c2-patches/rust.json) `/files/1` | `src/scanner.c` | `operations` / 1 | float suffix의 u/i/f 제한을 일반 alphabetic suffix로 넓히면서 exponent e/E를 구분한다. |

C2 등록 defect labels (17): `rust-P2-01`, `rust-P2-02`, `rust-P2-03`, `rust-P2-04`, `rust-P2-05`, `rust-P2-06`, `rust-P2-07`, `rust-P2-08`, `rust-P2-09`, `rust-P2-10`, `rust-P2-11`, `rust-P2-12`, `rust-P2-13`, `rust-P2-14`, `rust-P2-15`, `rust-P2-16`, `rust-P2-17`.

#### swift: 적용 chain

| 순서 | patch subject 및 JSON pointer | target | field / 수 | 변경·실패 construct·근거 |
|---:|---|---|---|---|
| 1 | [`prepare-p05/remedy-patches.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/prepare-p05/remedy-patches.json) `/patches/2/files/0` | `grammar.js` | `operations` / 6 | inline array [N of T]·Module::Type/expression/callee·signed integer generic argument를 추가한다. P05-SWIFT-REMEDY-r1, swift-V62/V64, #20. |
| 2 | [`c2-patches/swift.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/c2-patches/swift.json) `/files/0` | `grammar.js` | `operations` / 63 | sending/isolated ownership·return modifier·copy, macro generic call, const let/integer same-type/default type parameter, attribute·catch pattern·tuple/default/variadic·keypath·Type.Protocol, prefix/navigation·fallthrough, immediate attribute/string, some/any·metatype/composition·nonisolated(nonsending), switch directive·reference-specifier, operator-only backtick 거부, borrow/mutate accessor·Unicode ID·extended regex·module selector·postfix operator·specialized type·control transfer·repeat·dotted type를 교정한다. 예: parameter ownership, module::Type, break label/return expression. |
| 3 | [`c2-patches/swift.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/c2-patches/swift.json) `/files/1` | `src/scanner.c` | `operations` / 14 | raw string의 single/multiline·serialized state, built-in/custom operator 및 left-bound-only postfix, 둘 이상 # extended regex delimiter·line rule, module :: 뒤 newline 거부, semicolon mark_end를 교정한다. 예: ##/.../##, M::\nT, x!. |

C2 등록 defect labels (38): `swift-P2-01`, `swift-P2-14`, `swift-P2-15`, `swift-P2-27`, `swift-P2-36`, `swift-P2-22`, `swift-P2-37`, `swift-P2-13`, `swift-P2-16`, `swift-P2-17`, `swift-P2-31`, `swift-P2-30`, `swift-P2-29`, `swift-P2-21`, `swift-P2-03`, `swift-P2-19`, `swift-P2-20`, `swift-P2-05`, `swift-P2-28`, `swift-P2-32`, `swift-P2-33`, `swift-P2-02`, `swift-P2-07`, `swift-P2-12`, `swift-P2-26`, `swift-P2-35`, `swift-P2-18`, `swift-P2-25`, `swift-P2-11`, `swift-P2-04`, `swift-P2-24`, `swift-P2-34`, `swift-P2-10`, `swift-P2-06`, `swift-P2-09`, `swift-P2-08`, `swift-P2-23`, `swift-P2-39`.

#### dart: 적용 chain

| 순서 | patch subject 및 JSON pointer | target | field / 수 | 변경·실패 construct·근거 |
|---:|---|---|---|---|
| 1 | [`c2-patches/dart.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/c2-patches/dart.json) `/files/0` | `grammar.js` | `operations` / 30 | new 예약어·constructor tearoff/type args·dot-shorthand, function builtin ID·object pattern·nullable this/super formal, const empty collection·for pattern metadata·mixin modifier·super selector, &amp;&amp;/\|\| 우선순위·붙은 호출 args, primary constructor/semicolon body·formal modifier·this initializer·new/factory 생략 이름을 보완한다. 예: const [], C&lt;T&gt;.new, .new, class C(...);. |

C2 등록 defect labels (14): `dart-P2-01`, `dart-P2-03`, `dart-P2-02`, `dart-P2-13`, `dart-P2-04`, `dart-P2-05`, `dart-P2-06`, `dart-P2-11`, `dart-P2-12`, `dart-P2-14`, `dart-P2-07`, `dart-P2-08`, `dart-P2-09`, `dart-P2-10`.

#### php: 적용 chain

| 순서 | patch subject 및 JSON pointer | target | field / 수 | 변경·실패 construct·근거 |
|---:|---|---|---|---|
| 1 | [`c2-patches/php.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/c2-patches/php.json) `/files/0` | `common/define-grammar.js` | `operations` / 12 | legacy {expr} offset, PHP8.5 clone(args), asymmetric visibility의 (set) 한정, b/B heredoc/nowdoc·legacy invalid-octal·die alias·grouped use trailing comma·final/readonly property·final constant·void cast statement를 추가한다. 예: $a{0}, clone($o), public(set), die(), (void)$x;. |
| 2 | [`c2-patches/php.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/c2-patches/php.json) `/files/1` | `common/scanner.h` | `operations` / 10 | heredoc/nowdoc의 최소 nonblank body indentation을 직렬화하고 closing delimiter가 body보다 깊거나 spaces/tabs가 혼합되면 거부한다. |

C2 등록 defect labels (11): `php-P2-01`, `php-P2-03`, `php-P2-09`, `php-P2-04`, `php-P2-06`, `php-P2-07`, `php-P2-10`, `php-P2-11`, `php-P2-12`, `php-P2-13`, `php-P2-02`.

#### ruby: 적용 chain

| 순서 | patch subject 및 JSON pointer | target | field / 수 | 변경·실패 construct·근거 |
|---:|---|---|---|---|
| 1 | [`c2-patches/ruby.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/c2-patches/ruby.json) `/files/0` | `grammar.js` | `operations` / 4 | __LINE__/__FILE__/__ENCODING__ primary, control/meta escape, command-form method/singleton-method definition을 추가한다. |
| 2 | [`c2-patches/ruby.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/c2-patches/ruby.json) `/files/1` | `src/scanner.c` | `operations` / 2 | EOF에서 닫히는 heredoc과 Ruby4 줄 앞 \|\| continuation을 처리한다. |

C2 등록 defect labels (6): `ruby-P2-01`, `ruby-P2-02`, `ruby-P2-03`, `ruby-P2-04`, `ruby-P2-05`, `ruby-P2-06`.

#### r: 적용 chain

| 순서 | patch subject 및 JSON pointer | target | field / 수 | 변경·실패 construct·근거 |
|---:|---|---|---|---|
| 1 | [`c2-patches/r.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/c2-patches/r.json) `/files/0` | `grammar.js` | `operations` / 7 | reserved word·pipe placeholder _를 identifier에서 분리한다. call head/extraction lhs를 primary로, \|&gt; rhs를 call/placeholder extraction chain으로 제한하고 named argument _·_$coef·_[[...]]·_@slot을 추가한다. 예: x \|&gt; function(y)y, x \|&gt; f(a=_). #76·#95. |
| 2 | [`c2-patches/r.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/c2-patches/r.json) `/files/1` | `src/scanner.c` | `operations` / 1 | ERROR recovery에서 전부 거부하던 scanner가 열린 최상위 scope와 맞는 )/}/]/]]에만 응답하게 한다. MISSING operand 뒤 scope 종료·다음 statement 분리를 복원한다. 예: (x &lt;)\ny. |

C2 등록 defect labels (3): `r-P2-01`, `r-P2-02`, `r-P2-03`.

#### bash: 적용 chain

| 순서 | patch subject 및 JSON pointer | target | field / 수 | 변경·실패 construct·근거 |
|---:|---|---|---|---|
| 1 | [`c2-patches/bash.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/c2-patches/bash.json) `/files/0` | `grammar.js` | `operations` / 18 | coproc 이름·compound/command, for newline/빈 in 목록, function loop/case body·복수 redirect, subscript assignment·extglob·=/+= 우선순위·&lt;&gt; redirect·arithmetic argument·brace range와 increment·unary precedence·Bash5.3 funsub를 추가한다. 예: {a..z}, coproc NAME {...;}, exec {fd}&lt;&gt;file, ${ command; }/${\| command; }. |
| 2 | [`c2-patches/bash.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/c2-patches/bash.json) `/files/1` | `src/scanner.c` | `operations` / 7 | redirect target boundary·extglob token·heredoc delimiter quote 경계와 fd 변수/brace range를 구분한다. {fd}, {a..z}, 음수 숫자 range가 대상이다. |

C2 등록 defect labels (15): `bash-P2-01`, `bash-P2-02`, `bash-P2-03`, `bash-P2-04`, `bash-P2-05`, `bash-P2-06`, `bash-P2-07`, `bash-P2-08`, `bash-P2-09`, `bash-P2-10`, `bash-P2-11`, `bash-P2-12`, `bash-P2-13`, `bash-P2-14`, `bash-P2-15`.

#### powershell: 적용 chain

| 순서 | patch subject 및 JSON pointer | target | field / 수 | 변경·실패 construct·근거 |
|---:|---|---|---|---|
| 1 | [`c2-patches/powershell.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/c2-patches/powershell.json) `/files/0` | `grammar.js` | `operations` / 43 | named block/clean/using, decimal d suffix·curly quote·Unicode dash, ??=/??/ternary/null-conditional, ? 변수·braced escape, colon command parameter·--, pipeline&amp;, class default/base ctor/attribute/type·enum underlying type, empty scriptblock·argument bitwise/subexpression/range token 경계를 교정한다. 예: 1d, clean{}, -Name:$x, $x?.m, class C:B{C():base(){}}. |
| 2 | [`c2-patches/powershell.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/c2-patches/powershell.json) `/files/1` | `src/scanner.c` | `operations` / 3 | -- end-of-parameters와 이후 argument를 Unicode dash·delimiter에 맞춰 분리하고 statement terminator를 보존한다. |

C2 등록 defect labels (23): `powershell-P2-01`, `powershell-P2-02`, `powershell-P2-03`, `powershell-P2-04`, `powershell-P2-05`, `powershell-P2-06`, `powershell-P2-07`, `powershell-P2-08`, `powershell-P2-09`, `powershell-P2-10`, `powershell-P2-11`, `powershell-P2-12`, `powershell-P2-13`, `powershell-P2-14`, `powershell-P2-15`, `powershell-P2-16`, `powershell-P2-17`, `powershell-P2-18`, `powershell-P2-19`, `powershell-P2-20`, `powershell-P2-21`, `powershell-P2-22`, `powershell-P2-23`.

#### html: 적용 chain

| 순서 | patch subject 및 JSON pointer | target | field / 수 | 변경·실패 construct·근거 |
|---:|---|---|---|---|
| 1 | [`c2-patches/html.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/c2-patches/html.json) `/files/0` | `grammar.js` | `operations` / 7 | external text/entity/CDATA/erroneous_comment/content_whitespace·attribute separator·EOF raw-text missing token을 연결한다. PI·attribute separator·raw-text EOF recovery를 교정한다. 예: &lt;a x=1y=2&gt;, 닫히지 않은 script/style. |
| 2 | [`c2-patches/html.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/c2-patches/html.json) `/files/1` | `src/scanner.c` | `operations` / 9 | title/textarea RCDATA·plaintext, script DATA/ESCAPED/DOUBLE_ESCAPED, SVG/MathML namespace·integration point·CDATA, named/numeric character reference, content whitespace, --!&gt; comment close, optional end tag·EOF/attribute/text recovery를 구현한다. 예: &lt;!--x--!&gt;, &lt;textarea&gt;&amp;amp;&lt;/textarea&gt;, &lt;svg&gt;&lt;![CDATA[x]]&gt;&lt;/svg&gt;. |
| 3 | [`c2-patches/html.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/c2-patches/html.json) `/files/2` | `src/tag.h` | `operations` / 1 | OPTION/OPTGROUP/HR 및 CAPTION/COLGROUP/TBODY/TFOOT/THEAD/TR/TD/TH의 implicit end-tag 경계를 고친다. |

C2 등록 defect labels (12): `html-P2-01`, `html-P2-02`, `html-P2-03`, `html-P2-04`, `html-P2-05`, `html-P2-06`, `html-P2-07`, `html-P2-08`, `html-P2-09`, `html-P2-10`, `html-P2-11`, `html-P2-12`.

#### css: 적용 chain

| 순서 | patch subject 및 JSON pointer | target | field / 수 | 변경·실패 construct·근거 |
|---:|---|---|---|---|
| 1 | [`c2-patches/css.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/c2-patches/css.json) `/files/0` | `grammar.js` | `operations` / 27 | MQ4 range/general-enclosed·not/only scope, @import layer/supports·@layer·generic at-rule·ASCII case-insensitive keyword, empty ;·CDO/CDC, escape/custom name, nested rule와 {} declaration value, arbitrary custom property/!important, nth/An+B, escaped string newline, exponent+, url/math/division/component-value 경계를 교정한다. 예: @media(400px&lt;width&lt;1000px), --x:{a:b}, calc(1px / 2). #76·#95. |
| 2 | [`c2-patches/css.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/c2-patches/css.json) `/files/1` | `src/scanner.c` | `operations` / 5 | custom-property colon·quoted content·math/url name token, bad string newline 종료, nested block lookahead의 -/* 및 EOF 판정을 고친다. #76·#95. |

C2 등록 defect labels (28): `css-P2-01`, `css-P2-02`, `css-P2-03`, `css-P2-04`, `css-P2-05`, `css-P2-06`, `css-P2-07`, `css-P2-08`, `css-P2-09`, `css-P2-10`, `css-P2-11`, `css-P2-12`, `css-P2-13`, `css-P2-14`, `css-P2-15`, `css-P2-16`, `css-P2-17`, `css-P2-18`, `css-P2-19`, `css-P2-20`, `css-P2-21`, `css-P2-22`, `css-P2-23`, `css-P2-24`, `css-P2-25`, `css-P2-26`, `css-P2-27`, `css-P2-28`.

#### ★ json: 적용 chain

| 순서 | patch subject 및 JSON pointer | target | field / 수 | 변경·실패 construct·근거 |
|---:|---|---|---|---|
| 1 | [`c2-patches/json.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/c2-patches/json.json) `/files/0` | `grammar.js` | `operations` / 5 | RFC8259 whitespace만 허용하고 comment 제거·문서 한 value·raw U+0000–001F 거부·정확한 \u 4hex·fraction digit 필수를 적용한다. 예: //x, 1 2, "\uZZZZ", 1. 의 과잉 수용. |

C2 등록 defect labels (4): `json-P2-01`, `json-P2-02`, `json-P2-03`, `json-P2-04`.

#### yaml: 적용 chain

| 순서 | patch subject 및 JSON pointer | target | field / 수 | 변경·실패 construct·근거 |
|---:|---|---|---|---|
| 1 | [`c2-patches/yaml.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/c2-patches/yaml.json) `/files/0` | `src/scanner.c` | `operations` / 1 | 문서 prefix col0 U+FEFF BOM을 indentation 없이 넘기고 prefix 밖은 col0 --- 직전만 허용한다. 열린 block collection 종료 후 document boundary로 처리한다. YAML1.2.2 [202]/[211], #76·#93. |

C2 등록 defect labels (1): `yaml-P2-01`.

#### xml: 적용 chain

| 순서 | patch subject 및 JSON pointer | target | field / 수 | 변경·실패 construct·근거 |
|---:|---|---|---|---|
| 1 | [`c2-patches/xml.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/c2-patches/xml.json) `/files/0` | `common/common.mjs` | `operations` / 5 | XML Name/Nmtoken Unicode 범위, AttDef DefaultDecl 필수, NOTATION 반복 Name, EntityValue의 &lt;, AttValue의 불허 control/FFFE/FFFF를 교정한다. 예: &lt;!ATTLIST r a CDATA&gt;, NOTATION(a\|b\|c), Unicode tag. |
| 2 | [`c2-patches/xml.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/c2-patches/xml.json) `/files/1` | `xml/grammar.js` | `operations` / 2 | internal subset을 markupdecl/DeclSep 반복으로 바꿔 whitespace·parameter entity separator가 섞인 DTD를 허용한다. |
| 3 | [`c2-patches/xml.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/c2-patches/xml.json) `/files/2` | `common/scanner.h` | `operations` / 3 | NameStartChar/NameChar·is_xml_char, PI 종료를 newline 대신 ?&gt;로, comment의 불허 문자 경계를 교정한다. |
| 4 | [`c2-patches/xml.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/c2-patches/xml.json) `/files/3` | `xml/src/scanner.c` | `operations` / 3 | Unicode tag name을 온전한 UTF-8로 저장하고 character data·CDATA를 XML Char 범위로 제한한다. |

C2 등록 defect labels (7): `xml-P2-01`, `xml-P2-02`, `xml-P2-03`, `xml-P2-04`, `xml-P2-05`, `xml-P2-06`, `xml-P2-07`.

#### ★ tsql: 적용 chain

| 순서 | patch subject 및 JSON pointer | target | field / 수 | 변경·실패 construct·근거 |
|---:|---|---|---|---|
| 1 | [`prepare-p05/remedy-followup-r2.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/prepare-p05/remedy-followup-r2.json) `/patches/1/files/0` | `grammar.js` | `operations` / 1 | FROM/AS global·GENERATED column-constraint reserved set을 분리해 temporal/graph keyword가 identifier에 삼켜지지 않게 한다. |
| 2 | [`prepare-p05/remedy-followup-r2.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/prepare-p05/remedy-followup-r2.json) `/patches/1/files/1` | `grammar/keywords.js` | `operations` / 1 | ROW/GENERATED/ALWAYS/NODE/EDGE token을 추가한다. |
| 3 | [`prepare-p05/remedy-followup-r2.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/prepare-p05/remedy-followup-r2.json) `/patches/1/files/2` | `grammar/column-lists.js` | `operations` / 3 | GENERATED ALWAYS AS ROW START/END와 bare column flag를 구분한다. |
| 4 | [`prepare-p05/remedy-followup-r2.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/prepare-p05/remedy-followup-r2.json) `/patches/1/files/3` | `grammar/statements/create.js` | `operations` / 2 | CREATE TABLE … AS NODE/EDGE graph clause를 추가하고 column/filegroup 경계를 보존한다. |
| 5 | [`prepare-p05/remedy-tsql-r6.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/prepare-p05/remedy-tsql-r6.json) `/files/0` | `grammar.js` | `replacements` / 3 | IF ((SELECT COUNT(*) FROM t)=0) scalar subquery ambiguity, batch-first sp_who implicit call, ;WITH…/SELECT 1;; empty statement를 교정한다. |
| 6 | [`prepare-p05/remedy-tsql-r6.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/prepare-p05/remedy-tsql-r6.json) `/files/1` | `grammar/column-lists.js` | `replacements` / 3 | DEFAULT 0 WITH VALUES·DEFAULT ('x') NOT NULL·column PRIMARY KEY(a)/UNIQUE(a,b)를 수용하고 NOT의 잘못된 expression 결합을 막는다. |
| 7 | [`prepare-p05/remedy-tsql-r6.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/prepare-p05/remedy-tsql-r6.json) `/files/2` | `grammar/expressions.js` | `replacements` / 1 | subquery set-operation의 TOP … ORDER BY … UNION branch 소유권을 연결한다. |
| 8 | [`prepare-p05/remedy-tsql-r6.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/prepare-p05/remedy-tsql-r6.json) `/files/3` | `grammar/helpers.js` | `replacements` / 1 | 한글 등 Unicode identifier를 확장한다. 뒤 C2는 이를 SQL Server Unicode3.2 BMP 범위로 다시 제한한다. |
| 9 | [`prepare-p05/remedy-tsql-r6.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/prepare-p05/remedy-tsql-r6.json) `/files/4` | `grammar/keywords.js` | `replacements` / 1 | CHECKPOINT/SHUTDOWN keyword를 추가해 implicit call/alias 오인을 막는다. |
| 10 | [`prepare-p05/remedy-tsql-r6.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/prepare-p05/remedy-tsql-r6.json) `/files/5` | `grammar/statements/create-procedure.js` | `replacements` / 1 | procedure/trigger body statement 뒤 반복 ;를 수용한다. |
| 11 | [`prepare-p05/remedy-tsql-r6.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/prepare-p05/remedy-tsql-r6.json) `/files/6` | `grammar/statements/create.js` | `replacements` / 4 | CREATE TABLE(a int,b int,) final column comma·CREATE DATABASE ON PRIMARY/LOG ON filespec·% option을 추가한다. table-variable 목록은 무조건 넓히지 않는다. |
| 12 | [`prepare-p05/remedy-tsql-r6.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/prepare-p05/remedy-tsql-r6.json) `/files/7` | `grammar/statements/drop.js` | `replacements` / 1 | DROP INDEX i ON t WITH (ONLINE=OFF)의 trailing option 누락을 고친다. |
| 13 | [`prepare-p05/remedy-tsql-r6.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/prepare-p05/remedy-tsql-r6.json) `/files/8` | `grammar/statements/index.js` | `replacements` / 4 | BEGIN; … END·CHECKPOINT/SHUTDOWN·BACKUP SERVICE MASTER KEY/CERTIFICATE·RESTORE/MOVE를 추가한다. |
| 14 | [`prepare-p05/remedy-tsql-r6.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/prepare-p05/remedy-tsql-r6.json) `/files/9` | `grammar/statements/select.js` | `replacements` / 4 | FROM 없는 WHERE·scalar subquery·subquery TOP branch ORDER BY·SELECT @s +=…를 교정한다. |
| 15 | [`c2-patches/tsql.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/c2-patches/tsql.json) `/files/0` | `grammar.js` | `operations` / 55 | reserved keyword·precedence/conflict·batch head/tail, QUOTED_IDENTIFIER·MERGE terminator·CTE/trigger의 선행 terminator·scalar/predicate·GO·whitespace를 연결한다. PERCENT;와 bare KEY/DEFAULT/SCHEMA/NOT/WITH/TABLE의 identifier 우회, 무구분 후속 CTE를 차단한다. |
| 16 | [`c2-patches/tsql.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/c2-patches/tsql.json) `/files/1` | `src/scanner.c` | `operations` / 25 | nested block comment·QUOTED_IDENTIFIER 직렬화/double-quoted literal·MERGE lookahead·hint numeric·CR comment·GO 행/indent/tail/count/identifier·Unicode space를 처리한다. 미종결 /* outer /* inner */와 다음 줄 GO count 흡수, hint 0x/fraction/exponent 분할을 교정한다. |
| 17 | [`c2-patches/tsql.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/c2-patches/tsql.json) `/files/2` | `grammar/column-lists.js` | `operations` / 32 | computed-column constraint·XML COLUMN_SET·INDEX/HASH/FILESTREAM/ENCRYPTED/MASKED, QUEUE/ROUTE/SECURITY POLICY option-name/=/value, constant DEFAULT와 key ordering/list를 교정한다. |
| 18 | [`c2-patches/tsql.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/c2-patches/tsql.json) `/files/3` | `grammar/expressions.js` | `operations` / 86 | JSON aggregate·PARSE/TRY_PARSE·ODBC literal·full-text·CLR/XML method·NEXT VALUE FOR·Graph/SHORTEST_PATH·pseudo-column/SQLCMD variable·multipart/leading-dot reference·identifier를 보완한다. column-reference DEFAULT, wildcard/alias/DEFAULT 호출, scalar NOT, malformed DISTINCT/full-text·numeric EXEC·IN/parenthesized 빈/다중 element를 제한한다. |
| 19 | [`c2-patches/tsql.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/c2-patches/tsql.json) `/files/4` | `grammar/keywords.js` | `operations` / 8 | 누락 object-kind·DDL/admin·Graph/window/JSON/vector/rowset/hint keyword와 multiword phrase를 추가한다. Clause-start가 alias나 다음 문장으로 잘리는 문제를 고친다. |
| 20 | [`c2-patches/tsql.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/c2-patches/tsql.json) `/files/5` | `grammar/statements/alter.js` | `operations` / 49 | ALTER server config/resource pool/workload/external language/library/encryption key/stoplist/search property/credential/endpoint/availability, table mixed DROP·partition/index/masking/replication·DB/HADR/login option을 보완한다. ASSEMBLY file_name string·빈 ROUTE·binary cache handle·XML collection 이름·LOCK_ESCALATION TABLE을 교정한다. |
| 21 | [`c2-patches/tsql.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/c2-patches/tsql.json) `/files/6` | `grammar/statements/create-function.js` | `operations` / 7 | SQLCLR TVF RETURNS TABLE/ORDER/EXTERNAL NAME·cursor varying/nullability·native compilation/BEGIN ATOMIC·scalar/multistatement/inline body를 구분한다. formal parameter는 @name이어야 한다. |
| 22 | [`c2-patches/tsql.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/c2-patches/tsql.json) `/files/7` | `grammar/statements/create-procedure.js` | `operations` / 6 | procedure option·SQLCLR EXTERNAL NAME·native compilation/BEGIN ATOMIC 및 statement/body 소유권을 보완한다. |
| 23 | [`c2-patches/tsql.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/c2-patches/tsql.json) `/files/8` | `grammar/statements/create.js` | `operations` / 44 | ENDPOINT/resource/workload/AGGREGATE/availability/listener·external language/library/model/table·XML/JSON/spatial index·FILETABLE/graph·DB/credential/audit/assembly/fulltext/Broker 누락을 채운다. CREATE SCHEMA 소유권·final constraint comma·EVENT SESSION ADD/DROP·XML collection 이름을 교정한다. |
| 24 | [`c2-patches/tsql.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/c2-patches/tsql.json) `/files/9` | `grammar/statements/dml-shared.js` | `operations` / 6 | OUTPUT·SET @v=column+=…·UDT mutator/WRITE CST와 필수 DML VALUES element, column DEFAULT/variable·compound DEFAULT, column 이름 경계를 교정한다. |
| 25 | [`c2-patches/tsql.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/c2-patches/tsql.json) `/files/10` | `grammar/statements/drop.js` | `operations` / 7 | object-kind별 DROP·named DROP INDEX option·DROP ROLE을 추가하고 DROP SERVER ROLE IF EXISTS·DROP XML SCHEMA COLLECTION IF EXISTS·XML multipart 이름의 과잉 수용을 막는다. |
| 26 | [`c2-patches/tsql.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/c2-patches/tsql.json) `/files/11` | `grammar/statements/index.js` | `operations` / 57 | cursor/table-variable DECLARE·KILL·BACKUP/RESTORE/key·numbered procedure/EXEC RESULT SETS/pass-through·securable·signature/statistics·hint·block/TRY/CATCH를 보완한다. PASSWORD-only key·nonempty BEGIN·CTE/trigger terminator·EXEC argument/principal·dynamic concat·hint 인자·RESTORE MOVE @file을 교정한다. |
| 27 | [`c2-patches/tsql.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/c2-patches/tsql.json) `/files/12` | `grammar/statements/insert.js` | `operations` / 4 | INSERT INTO OPENQUERY/OPENROWSET write target/source를 추가하고 SELECT/set-operation의 OPTION 중복·BULK/일반 provider를 구분한다. |
| 28 | [`c2-patches/tsql.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/c2-patches/tsql.json) `/files/13` | `grammar/statements/module-shared.js` | `operations` / 1 | SQLCLR EXTERNAL NAME assembly.class.method와 module EXECUTE AS 공유 문법을 추가한다. |
| 29 | [`c2-patches/tsql.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/c2-patches/tsql.json) `/files/14` | `grammar/statements/security.js` | `operations` / 9 | key/certificate EXECUTABLE FILE·복수 encryptor·DROP SYMMETRIC KEY, SECURITY POLICY FILTER/BLOCK·scalar argument와 ALTER predicate/STATE/ADD·DROP NOT FOR REPLICATION의 분리 대안을 교정한다. |
| 30 | [`c2-patches/tsql.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/c2-patches/tsql.json) `/files/15` | `grammar/statements/select.js` | `operations` / 53 | SELECT INTO/ALL/assignment·APPROXIMATE·XML/JSON mode·window null option·hint·rowset schema/FOR PATH/Synapse join/GROUP BY를 보완한다. FROM 없는 GROUP/HAVING·WINDOW alias·TVF hint/schema 우회·BULK/OPENQUERY/OPENXML/PREDICT/OPENJSON·write TVF·CHANGETABLE·grouping tuple를 구분한다. |
| 31 | [`c2-patches/tsql.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/c2-patches/tsql.json) `/files/16` | `grammar/statements/service-broker.js` | `operations` / 2 | CREATE/ALTER BROKER PRIORITY·ALTER MESSAGE TYPE validation·SERVICE contract·REMOTE SERVICE BINDING·CONVERSATION TIMER/BEGIN DIALOG를 추가한다. |
| 32 | [`c2-patches/tsql.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/c2-patches/tsql.json) `/files/17` | `grammar/statements/set.js` | `operations` / 4 | cursor assignment·option-list 후반 QUOTED_IDENTIFIER·SET OFFSETS·2025 \|\|=를 추가하고 scanner state를 기존 option-name CST에 연결한다. |
| 33 | [`c2-patches/tsql.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/c2-patches/tsql.json) `/files/18` | `grammar/transactions.js` | `operations` / 2 | COMMIT DELAYED_DURABILITY·ROLLBACK transaction/savepoint 이름 대안을 보완한다. |
| 34 | [`c2-patches/tsql.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/c2-patches/tsql.json) `/files/19` | `grammar/types.js` | `operations` / 2 | JSON type·xml([CONTENT\|DOCUMENT] schema_collection)·varbinary 경계를 보완한다. |
| 35 | [`c2-patches/tsql.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/c2-patches/tsql.json) `/files/20` | `grammar/helpers.js` | `operations` / 2 | regular identifier를 SQL Server Unicode3.2 BMP letter/Nd로 고정한다. write-target hint 허용을 writer별로 분리하여 DELETE/UPDATE 제약이 다른 writer를 넓히지 않게 한다. |
| 36 | [`c2-patches/tsql.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/c2-patches/tsql.json) `/files/21` | `grammar/statements/merge.js` | `operations` / 3 | standalone MERGE 필수 ;와 composable MERGE를 구분한다. WHEN MATCHED/NOT MATCHED BY TARGET/SOURCE의 UPDATE/DELETE/INSERT·value list·predicate 대안을 제한한다. |

C2 등록 defect labels (136): `tsql-P2-01`, `tsql-P2-02`, `tsql-P2-03`, `tsql-P2-04`, `tsql-P2-05`, `tsql-P2-06`, `tsql-P2-07`, `tsql-P2-08`, `tsql-P2-09`, `tsql-P2-10`, `tsql-P2-11`, `tsql-P2-12`, `tsql-P2-13`, `tsql-P2-14`, `tsql-P2-15`, `tsql-P2-16`, `tsql-P2-17`, `tsql-P2-18`, `tsql-P2-19`, `tsql-P2-20`, `tsql-P2-21`, `tsql-P2-22`, `tsql-P2-23`, `tsql-P2-24`, `tsql-P2-25`, `tsql-P2-26`, `tsql-P2-27`, `tsql-P2-28`, `tsql-P2-29`, `tsql-P2-30`, `tsql-P2-31`, `tsql-P2-32`, `tsql-P2-33`, `tsql-P2-34`, `tsql-P2-35`, `tsql-P2-36`, `tsql-P2-37`, `tsql-P2-38`, `tsql-P2-39`, `tsql-P2-40`, `tsql-P2-41`, `tsql-P2-42`, `tsql-P2-43`, `tsql-P2-44`, `tsql-P2-45`, `tsql-P2-46`, `tsql-P2-47`, `tsql-P2-48`, `tsql-P2-49`, `tsql-P2-50`, `tsql-P2-51`, `tsql-P2-52`, `tsql-P2-53`, `tsql-P2-54`, `tsql-P2-55`, `tsql-P2-56`, `tsql-P2-57`, `tsql-P2-58`, `tsql-P2-59`, `tsql-P2-60`, `tsql-P2-61`, `tsql-P2-62`, `tsql-P2-63`, `tsql-P2-64`, `tsql-P2-65`, `tsql-P2-66`, `tsql-P2-67`, `tsql-P2-68`, `tsql-P2-69`, `tsql-P2-70`, `tsql-P2-71`, `tsql-P2-72`, `tsql-P2-73`, `tsql-P2-74`, `tsql-P2-75`, `tsql-P2-76`, `tsql-P2-77`, `tsql-P2-78`, `tsql-P2-79`, `tsql-P2-80`, `tsql-P2-81`, `tsql-P2-82`, `tsql-P2-83`, `tsql-P2-84`, `tsql-P2-85`, `tsql-P2-86`, `tsql-P2-87`, `tsql-P2-88`, `tsql-P2-89`, `tsql-P2-90`, `tsql-P2-91`, `tsql-P2-92`, `tsql-P2-93`, `tsql-P2-94`, `tsql-P2-95`, `tsql-P2-96`, `tsql-P2-97`, `tsql-P2-98`, `tsql-P2-99`, `tsql-P2-100`, `tsql-P2-101`, `tsql-P2-102`, `tsql-P2-103`, `tsql-P2-104`, `tsql-P2-105`, `tsql-P2-106`, `tsql-P2-107`, `tsql-P2-108`, `tsql-issue122-cte-terminator`, `tsql-issue123-unqualified-delimited-function`, `tsql-issue122-sqlcmd-cte-context`, `tsql-issue128-constant-default`, `tsql-issue128-corpus-context`, `tsql-issue131-overacceptance`, `tsql-issue132-security-policy`, `tsql-issue134-assembly-file-name`, `tsql-issue137-mutation-boundaries`, `tsql-issue138-leading-dot`, `tsql-issue139-formal-parameter`, `tsql-issue145-rowset-hint-boundary`, `tsql-issue146-hint-integer-boundary`, `tsql-issue148-ddl-create-alter-boundary`, `tsql-issue148-fulltext-quoted-literal`, `tsql-issue148-provider-schema-boundary`, `tsql-issue141-regexp-predicate`, `tsql-issue149-write-target-argument`, `tsql-issue149-cache-binary-handle`, `tsql-issue150-table-reserved`, `tsql-issue150-xml-schema-name-drop`, `issue154-cr-comment`, `issue158-go-line-boundary`, `issue159-nonempty-expression-list`, `issue165-comment-closure`, `issue166-changetable-arguments`, `issue168-nonempty-function-grouping-tuple`, `issue171-unicode-space-boundary`.

#### postgresql-sql: 적용 chain

| 순서 | patch subject 및 JSON pointer | target | field / 수 | 변경·실패 construct·근거 |
|---:|---|---|---|---|
| 1 | [`prepare-p05/remedy-exact-r1.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/prepare-p05/remedy-exact-r1.json) `/patches/1/change` | `postgres/grammar.js` | `operations` / 1 | OptWith에 WITH OIDS를 추가해 legacy WITH/WITHOUT OIDS 합집합을 보존한다. P05-PG-LEGACY-REMEDY-r1, #20. |
| 2 | [`c2-patches/postgresql-sql.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/c2-patches/postgresql-sql.json) `/files/0` | `postgres/grammar.js` | `operations` / 17 | nested block comment external화, U&amp; Unicode identifier/string·UESCAPE, hex/oct/bin integer·trailing-dot float, ANY/ALL subquery operator precedence·postfix operator target, ALTER SET WITH OIDS/COPY OIDS/CREATE FUNCTION opt_definition legacy syntax를 교정한다. |
| 3 | [`c2-patches/postgresql-sql.json`](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/src/dev/c2-patches/postgresql-sql.json) `/files/1` | `postgres/src/scanner.c` | `operations` / 3 | FF/VT whitespace, COMMENT token 및 중첩 /* */·-- line comment를 처리한다. |

C2 등록 defect labels (9): `postgresql-sql-P2-01`, `postgresql-sql-P2-02`, `postgresql-sql-P2-03`, `postgresql-sql-P2-04`, `postgresql-sql-P2-05`, `postgresql-sql-P2-06`, `postgresql-sql-P2-07`, `postgresql-sql-P2-08`, `postgresql-sql-P2-09`.


#### csharp — wave 1

PREPARE의 grammar-only 비교 대상은 upstream PR442의 고정 revision이다. 현재 upstream merge 증거는 아니다. `remedy-csharp-r5.json`의 27개 입력·5개 edit 및 `source-feature-feasibility.md`의 r4 구조 실패 → r5 교정이 초기 chain의 근거다.

| 단계 | 변경·결함 construct와 이유 | 근거 |
|---|---|---|
| remedy r1 grammar | `async`를 identifier와 local function/lambda/delegate modifier로 구분한다. Compilation unit 앞 `#:sdk`·`#:package`·`#:property`·`#:project`·`#:include`의 name/argument를 보존한다. | `remedy-patches.json /patches/0/files/0`; `P05-REMEDY-CS-ASYNC-LOCAL/LAMBDA/DELEGATE`, `P05-CS-DIRECTIVE-*` |
| remedy r1 scanner | 실제 행 시작의 `#:`만 인식하고 일반·verbatim·raw 보간 문자열 내부에서는 지시문으로 오인하지 않는다. | 같은 subject `/patches/0/files/1`; `P05-REMEDY-CS-INTERPOLATED-*` |
| remedy r2 | `await` contextual identifier, method return/new-object type의 `var`를 identifier로 보존한다. 실제 await/implicit type도 유지한다. | `remedy-r2.json /patches/0/file`; `P05-REMEDY-CS-AWAIT-ARG`·`P05-REMEDY-CS-VAR-TYPE` |
| remedy r3 | `var` identifier·implicit type·object creation의 GLR conflict를 추가한다. | `remedy-exact-r1.json /patches/0/change`; 앞 단계 generation conflict |
| remedy r4 | 같은 모호성의 method return type 경로도 열어 둔다. | `remedy-followup-r2.json /patches/0/files/0` |
| remedy r5 | `var` 반환형/`new var()`가 계속 `implicit_type`으로 관측된 r4 실패를 두 context의 dynamic precedence로 교정한다. | `remedy-csharp-r5.json /patch`; `P05-REMEDY-CS-VAR-TYPE` |
| C2 `grammar.js` | Unicode whitespace·NEL/LS/PS, `allows ref struct`, `extern unsafe ~F()`, namespace `extern alias`, 연속 switch label 뒤 필수 statement, handler 없는 `try`, 빈 `#error`/`#warning`, contextual `async`/`await`·`var` pattern을 교정한다. Unicode escape identifier에 literal과 같은 범주를 요구한다. Directive condition의 symbol/Boolean/operator, EOF·물리행·define/undef symbol·decimal `#line`·escape를 해석하지 않는 filename·unknown/불완전 pragma tail을 구분한다. Inactive nested directive를 재귀 검증한다. #182의 argument·initializer·if-header·try-handler fragment 네 문맥을 추가한다. String/char의 immediate closing/escape와 recovery literal 경계로 가짜 directive를 막는다. | `c2-patches/csharp.json /files/0`; P2-01..11·P2-PP, #152·153·157·160–163·179·182·184·233 |
| C2 `src/scanner.c` | `$""` empty interpolation, inactive skipped section·directive start·EOF/newline, directive 전용 keyword·symbol, 재진입/직렬화 상태를 처리한다. `#endif /*...*/` 과잉 수용을 막고 `//`는 허용한다. Malformed raw/verbatim/일반 문자열·Razor comment·중첩 interpolation이 다음 지시문을 삼키거나 내부 `#`를 지시문으로 만들지 않게 한다. `#line` filename 뒤 block comment/newline recovery, pragma `//`의 기존 `preproc_arg` 소유권, pragma 안 `@"a"""""`·`$@"..."`·`@$"..."` doubled quote를 교정한다. | `c2-patches/csharp.json /files/1`; 아래 보고서 |

근거 보고서: [잔여 구문](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/docs/reports/issues-157-169-residual-syntax.md), [inactive directive](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/docs/reports/issues-178-179-residual-runtime-csharp.md), [fragment·endif](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/docs/reports/issue-182-csharp-conditional-fragments.md), [lexical recovery 및 최종 pragma 교정](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/docs/reports/issue-233-csharp-lexical-recovery.md).

#233의 `directive-string-no-escape`·`razor-comment-lines`·`dollars-two-quotes-raw`·`nested-string-open-no-runaway`는 교정된 실패 입력이다. 2026-11-12 최종 pragma 회귀는 정상 30개 중 18개에서 다음 `#undef`·class가 사라졌던 문제이며 현재 patch에 교정이 들어 있다. 보고서의 새 30 fixture PASS, 원본 157의 승인 변화 5/ERROR 소실 3, 정상 88 full CST 불변은 고정 관측이다. 이번 인계에서 재실행하지 않았으며 #233 전체 acceptance 완료로 승격하지 않는다.

일반 lexical/syntax 교정은 upstream 기여 후보다. Literal-false 이외 첫 branch 선택 정책과 네 fragment 문맥은 upstream 요구와의 일치 여부를 별도 검토하고 known gap도 함께 공개한다.

#### tsql — wave 1: 근거 및 해석

36개 chain entry는 P05 followup의 4 entry/7 operations + r6의 10 entry/23 operations + C2의 22 entry/464 operations다. C2 label 136개는 P2-01..108과 후속 issue label 28개다. P05 `remedy-tsql-r6.json`의 `classes` 15행이 각 shape·공식 근거·당시 관측을 소유한다.

다음 보고서는 위 chain의 후속 root cause와 public 최소 입력을 연결한다.

| 근거 | 패치에 연결된 실패 construct·구분 |
|---|---|
| [#113](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/docs/reports/issue-113-tsql-boundaries.md) | `PERCENT;` regular identifier, Unicode3.2 밖 문자, master/service key의 PASSWORD 외 option, 빈 BEGIN 과잉 수용 |
| [#121](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/docs/reports/issue-121-tsql-engine-validation.md) | `BEGIN WITH…`는 실제 engine에서 유효하여 앞선 제한을 제거했다. 앞 statement 뒤 무세미콜론 CTE 오류319와 구분 |
| [#122·123](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/docs/reports/issue-122-123-tsql-boundaries.md) | SQLCMD를 사이에 둔 CTE terminator와 schema 없는 delimited-function 호출 경계 |
| [#128](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/docs/reports/issue-128-tsql-corpus-gaps.md) | constant DEFAULT·CONVERT·CAST 결과 XML method·column/table constraint·TRY/CATCH·FROM 없는 GROUP/HAVING |
| [#131](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/docs/reports/issue-131-tsql-overacceptance.md) | 공개 NEGATIVE_SYNTAX 18행의 reserved·CTE·MERGE/trigger terminator·EXEC·Graph·hint·DDL option/body |
| [#132](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/docs/reports/issue-132-security-policy-erratum.md) | ALTER SECURITY POLICY predicate/STATE/NOT FOR REPLICATION 대안. 문서 suffix와 pinned ScriptDOM/engine 불일치는 erratum으로 처분 |
| [#134](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/docs/reports/issue-134-assembly-file-name-erratum.md) | ADD/DROP ASSEMBLY FILE의 file_name은 identifier가 아닌 string. object name·source literal·DROP ALL 보존 |
| [#137–140](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/docs/reports/issues-137-139-tsql-token-boundaries.md) | predicate/call/option/fulltext·leading-dot·formal @parameter 경계 |
| [#141·145·146·148](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/docs/reports/issue-141-tsql-context-and-hints.md) | rowset/hint integer·DDL create/alter/provider별 syntax; offline context 진단은 별도 |
| [#149](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/docs/reports/issue-149-tsql-corpus-context.md) | write-side TVF argument·binary cache handle는 grammar. column permission/window/trigger context는 offline checker |
| [#150](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/docs/reports/issue-150-tsql-identifier-xml.md) | bare TABLE, XML collection 이름/IF EXISTS, LOCK_ESCALATION 정상 대조 |
| [#154·158·159·165·166·168·171](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/docs/reports/issues-157-169-residual-syntax.md) | CR comment, GO 물리행/반복 count, nonempty list, nested comment closure, CHANGETABLE, grouping tuple, Unicode space |

[#151 EXEC OUTPUT](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/docs/reports/issue-151-tsql-exec-output.md)은 `CheckTSQLContext`만 변경했으며 grammar/source/generated pin 변경은 없다. 이를 grammar patch 수에 더하지 않았다. Binding/type/collation·module/batch-first·arity·permission/runtime·SQLCMD substitution 모두를 grammar로 판단하는 계약도 아니다. SQL Server2025 engine의 compatibility110은 실제 2012 engine 실행이 아니며 PARSEONLY 수용은 binding·실행 성공이 아니다.

r6의 `gaps` 5행은 현재 잔여가 아니다. Column FOREIGN KEY explicit list, column PK/UNIQUE ASC/DESC, final table-constraint comma, CAST 결과 XML method와 RESTORE MOVE OS-file `@parameter`는 후속 C2에 들어 있다. #109의 과거 5종과 #131의 SECURITY POLICY/ASSEMBLY 보류도 각각 #113/#121, #132/#134의 후속 처분을 따른다.

### 미수정 문법·표현 격차와 기록된 한계

아래의 `unknown`은 현재 upstream 상태를 뜻한다. 표의 수는 alternative·지원 정책·표현 한계가 섞이므로 “잔여 버그 총 N개”로 합치지 않는다. 문서에 새 최소 재현 입력이 없는 한계는 소스 주석에 기록된 범위까지만 전달한다.

| route | 기록된 construct·한계 | 처분·근거 |
|---|---|---|
| ★ csharp | `csharp-B02.a079`의 `default` type-parameter constraint: `NOT_IN_PINNED_GRAMMAR`, 1 alternative | 고정 grammar에서 node type이 없는 표현/expectation gap. 실행으로 syntax rejection을 확인했다는 주장은 아니다. upstream 조사·기여 후보, 현재는 known gap. `csharp-g1-expected-b01b02-r2.json`·`csharp-g1-ledger-r1.json` |
| ★ csharp | 기본 parser는 literal-false가 아닌 첫 section을 선택한다. `(false)`·`!(true)`·`true == false`, source define/undef·caller symbols를 일반 평가하지 않는다. `inactive-broken`·`undef-inactive`·`nested-directives`의 다른 branch 관측 | 기본 구조 모드 `NOT_CLAIMED`인 known gap. Opt-in symbol mode와 구분한다. `language-feature-disposition.md` 및 `issue-198-core-language-holdout.md` |
| ★ csharp | #182는 argument·initializer·if-header·try-handler의 네 fragment 문맥만 등록 | 임의 token 경계의 모든 conditional 조합은 미주장. `issue-182-csharp-conditional-fragments.md` |
| ★ go | 괄호/대괄호 안 타입 뒤 newline 일부 허용; recovery 중 automatic semicolon을 내지 않아 newline resynchronization을 하지 않음 | scanner 주석의 두 한계. upstream 현존 `unknown`, known gap 측정 대상. `c2-patches/go.json /files/1/operations/0/after` |
| javascript / jsx | function/generator context `MAX_FRAMES=1000`보다 깊으면 deepest stored frame 재사용 | 각 route에 같은 한도 1개. `c2-patches/javascript.json` 및 `jsx.json` scanner의 `ponytail:` 주석. 경계에서의 실제 오판 수는 측정하지 않음 |
| kotlin | multi-dollar scanner의 dollar run은 2…255, `MD_MAX_DEPTH=64` | 한 scanner 표현 한도 범주이며 두 상수가 있다. `c2-patches/kotlin.json` scanner. upstream 상태 `unknown` |
| c / cpp | 미전개 macro/preprocessor의 모호한 source를 heuristic으로 처리하며 오류 없이 수용할 수 있음 | 각 route의 known limitation. `issue-220-v030-release.md` 잔존 제약, #101 C2 patch. macro expansion/표준 semantic 지원과 구분하고 upstream 기여 전 정책 재검토 |
| html | `annotation-xml`의 encoding attribute를 scanner가 못 보아 HTML integration point를 판정하지 않고 foreign 상태 유지 | `c2-patches/html.json` scanner 주석. known gap, upstream 상태 `unknown` |
| html | 모든 alphanumeric 이름 뒤 `;`를 character reference로 취급 | 같은 scanner의 `ponytail:` 주석. 유효 named-entity 목록과의 완전 대조를 주장하지 않음 |
| ★ tsql | batch-first implicit EXEC 앞 extra leading `;`, 예: `;sp_executesql …` | `fact-mapping.json`의 현재 known miss. ERROR가 procedure부터 시작하고 captures가 실제 batch-first를 입증할 때만 fallback 진단하며 ERROR가 `;`부터 시작하는 다른 모양은 보고하지 않는다. grammar/recovery·fact detector의 알려진 경계로 공개. 현재 upstream 실패를 새 실행으로 확정한 것은 아님 |
| ★ tsql | `CREATE EXTERNAL TABLE` 마지막 쉼표 | `language-sources.json`이 #128 완료 범위 밖이라고 명시. 현재 실행 실패가 아니라 미검증 범위 |
| ★ tsql | QUOTED_IDENTIFIER 기본 ON, 입력의 SET과 1-byte state만 추적. `SET ANSI_DEFAULTS`·connection/database default·client의 batch 간 상태는 미추적 | `c2-patches/tsql.json` scanner 주석. 상태 문맥 known gap |
| ★ tsql | #188의 Oracle PL/SQL/client named-parameter corpus 39개 source | route 밖 방언 경계. colon bind 426회는 Oracle 유효성 증명이 아니다. T-SQL upstream 버그로 세지 않는다. `issue-188-corpus-cst-api.md` |

표에서 별도로 기록한 current grammar representation `BLOCKED`는 C#의 **1 alternative**다. 다른 행은 정책·정적 구현 한계·미검증 경계이며 같은 단위의 defect 수가 아니다. 그 밖 route를 “미해결 결함 0”으로 보증하지 않는다.

#### G1 BLOCKED의 정확한 분모

[기대값 계약](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/docs/specs/csharp-g1-expected.md), `src/contracts/csharp-g1-expected-b01b02-r2.json` 및 `csharp-g1-ledger-r1.json`을 직접 집계했다. 과거 검토의 172를 현재 값으로 쓰지 않는다.

- B01/B02 기대값 505 alternatives = `EXPECTED` 426 + `BLOCKED` 79. B01 BLOCKED 37, B02 BLOCKED 42.
- 전체 ledger 1,531 alternatives = `EXECUTED` 426 + `BLOCKED` 79 + `NO_EXPECTED` 1,026.
- `NO_EXPECTED`와 “실행됨”은 전체 C# correctness를 승인했다는 뜻이 아니다.

| BLOCKED code | alternative 수 | 해석 |
|---|---:|---|
| NO_TREE_COUNTERPART | 19 | whitespace/newline/trivia 등의 표현 차이 |
| LEXICAL_META_PRODUCTION | 7 | 독립 node가 없는 meta-production |
| EXTERNAL_SCANNER_REPRESENTATION | 7 | scanner 표현으로 grammar.js에 직접 없음 |
| NO_WITNESS_IN_LINKED_SOURCES | 6 | 연결 source에서 witness 없음 |
| POSITION_DEPENDENT | 7 | 관측기에 syntactic slot이 없음 |
| ABSTRACT_CATEGORY | 10 | 독립 node가 없는 union production |
| SEMANTIC_BINDING | 22 | binding 없이 판별 불가 |
| NOT_IN_PINNED_GRAMMAR | 1 | 위 default constraint 표현 gap |
| 합계 | **79** | 79개의 grammar 버그로 합산하지 않음 |

#### 문법 결함이 아닌 BLOCKED

`qualification-c1.json`의 `expect_assessment == "BLOCKED"` 사례는 6개이며 모두 ordinary 26-route syntax defect와 다른 역할이다.

| 역할 | case | code·수 |
|---|---|---|
| n461-svc | n461-svc-language-vb-n-r1 | SVC_INLINE_UNSUPPORTED: 1 |
| csharp-symbols | define-inactive | PREPROCESS_ZERO_ACTIVE: 1 |
| csharp-symbols | define-after-token, malformed-expression, unterminated | PREPROCESS_MALFORMED: 3 |
| csharp-symbols | literal-true | INCREMENTAL_ROUTE_UNOBSERVABLE_SELECTION_STEP_1: 1 |

`src/testdata/native/reuse-holdouts.json`의 original histories는 T-SQL 12(#170의 11 + #198 unclosed-bracket 1), JSON 3(json-repair-close/break-close/repair-colon), 합계 **15**다. [#217 보고서](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/docs/reports/issue-217-reuse-dispositions.md)의 정상 old-tree/logger 관측에도 공개 TSNode.id 교집합이 0인 route proof 한계를 보존한다. 이것은 구문·fresh/incremental equality 실패 15개가 아니다.

#### 역사적 known_gaps 등록과 이미 해결된 항목

`language-sources.json`의 `known_gaps`는 **6 routes / 23 entries**다. route별 수는 csharp 3, typescript 3, tsx 3, swift 4, tsql 6, postgresql-sql 4이며 나머지 20 routes는 해당 배열 entry 0이다. 이 숫자는 **원시 등록 항목 수**이며 현재 미수정 문법 결함 수가 아니다.

| route | 23개 원시 entry의 내용과 읽는 법 |
|---|---|
| csharp (3) | 등록27 밖 contextual identifier/file-based directive 미검증, S04 patch/generation 재현성, S08 Windows/macOS. 이후 C2/보고서와 B2의 확장 사례를 먼저 확인 |
| typescript (3) | V59 밖 5.x–7 구문, npm JS0.23.1 closure, S08 Windows/macOS. 준비 시점 항목이며 현행 C2 존재를 반영해 해석 |
| tsx (3) | B01/B02/V29 밖 구문, 같은 npm closure, S08 Windows/macOS |
| swift (4) | upstream parser.c 부재로 generation 필요, scanner/ABI, 등록 범위 밖 문법, S08 Windows/macOS |
| tsql (6) | statement bounded 범위, ESM closure, SQL Server runtime NOT_RUN, S08 Windows/macOS, #128 해결/미포함 경계, #113 당시 제한. BEGIN 직후 CTE 제한은 #121에서 제거됨 |
| postgresql-sql (4) | PG19 기반 후보의 9.6–18 checkpoints, 당시 utility0, noopt state overflow/optimized 6GiB, S08 Windows/macOS. utility0은 현재 B2 사례를 대체하지 않는 준비 기록 |

PostgreSQL의 noopt **387,042 states > 65,535** 및 optimized generation의 **6 GiB**는 generation/resource 한계다. 이를 문법 실패로 세지 않는다. `source-feature-feasibility.md`의 채택 이전 `BLOCKED_EXTERNAL`도 현재 native 실패로 승계하지 않는다.

현재 잔여 목록에 다시 넣으면 안 되는 대표 항목은 다음과 같다.

- #233의 directive 위치 오판 4개와 pragma verbatim의 다음 directive/class 소실 18개는 현행 patch에 교정됐다. 원본157 strict4/C4의 중간 실패와 최종 승인5/ERROR소실3을 구분한다.
- #279의 primary CST 기대값은 **26 PASS / 0 MISMATCH / 4 NOT_APPLICABLE**이며 symbol outcome의 **25 PASS / 5 registered BLOCKED**와 다른 축이다.
- #198 Go U+200C identifier FP는 [#205](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/docs/reports/issue-205-go-identifiers.md)의 Unicode17 L/Nd/_ patch로 교정됐다.
- #109 비첫 `;;sp_executesql` known-miss 오분류는 현재 `src/kit/facts.go`의 batch/program·GO firstness 추적으로 교정됐다. `AT DATA_SOURCE`·`WITH RESULT SETS`의 옛 ERROR fallback도 현행 unsupported syntax로 세지 않는다.
- S08 당시 grammar gap62(C#43/T-SQL19)는 후속 교정 전 관측이다. #188 최종 ERROR49는 source damage10 + Oracle/client 방언39다.
- #128 NEGATIVE_SYNTAX18은 #131·#132·#134·#137–151 후속 처분을 따른다. #198 CTE FN2는 #219 engine의 오류319 거부로 ScriptDOM 과잉 수용 차이로 분류됐다.

### native chain의 공통 runtime patch — grammar 수에서 제외

`native-routes.json`은 `src/drivers/native-c/runtime-manifest.json`도 참조한다. 대상은 `tree-sitter/tree-sitter@10eb5ac119eec9cb4267a8e035066473983f5f3a`(v0.27.1, MIT)의 `lib/src/node.c`이며 26 routes에 공통이다.

| patch subject | pointer/field | operations | 기록된 결함·처분 |
|---|---|---:|---|
| src/drivers/native-c/runtime-field-lookup.patch.json | /files/0 operations | 1 | #118 visible alias·closest hidden field·ERROR/extra 경계의 field lookup |
| src/drivers/native-c/runtime-navigation.patch.json | /files/0 operations | 1 | #120 visible anonymous alias의 named-navigation boundary·hidden child flattening |
| src/drivers/native-c/runtime-sibling-identity.patch.json | /files/0 operations | 3 | #178 zero-width/missing/alias/hidden/ERROR/extra 사이 exact sibling identity |

**3 entries / 5 operations**이며 앞의 grammar97/1,423에 더하지 않았다. 현재 upstream 잔존 여부는 unknown, upstream contribution/재측정 후보다. Manifest의 remove_when은 동등한 owned/26-route/large/three-platform 검증을 요구한다. ADR-0020의 unmodified runtime과 kit의 patched native 관측은 서로 다른 비교 대상이므로 과거 결과를 새 runtime의 정답·PASS로 복사하지 않는다.

## B2. 26-route feature 범위

[language-feature-scope.md](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/docs/validation/language-feature-scope.md), [disposition](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f/docs/validation/language-feature-disposition.md)의 채택 범위와 `qualification-c1.json`의 `routes[].requirements`를 결합했다. 범위의 기준일은 2026-09-29이며 현재 최신 언어 release라는 주장이 아니다. 표는 요구·등록 상태이고, unmodified upstream의 성공이나 사양 전체 적합성을 뜻하지 않는다.

사례 수는 **`routes[].workload.cases` 배열 길이**다. 추가 역할인 SVC·large·symbols·reuse·owned/reference를 포함하지 않으며 플랫폼 수나 edit step 수를 곱하지 않았다. License는 `language-sources.json`의 `license_metadata`다. ★는 wave 1이다. Feature ID는 route 접두사를 생략했다.

| Route | 채택 언어 범위 | 등록 feature 계열 | REQ ID (행 수) | 사례 수 | grammar pin | License |
|---|---|---|---|---:|---|---|
| ★ `csharp` | C#7.3/net461 → C#14/net10.0 | lexical·문자열·directive, type/variable, expression/LINQ/lambda, statement, namespace/type/member; 7.3–13 변경; 14의 extension/assignment/generic/lambda·field/partial/operator·file-based directive | `B01, B02, B03, B04, B05, V73, V08, V09, V10, V11, V12, V13, V14a, V14b, V14c` (15) | 390 | [9150f7d56bb4](https://github.com/tree-sitter/tree-sitter-c-sharp/tree/9150f7d56bb47f1a809fa23623f1ba1413e93fa9) | MIT |
| ★ `go` | 1.17 → 1.27 | lexical/semicolon, type/declaration, expression, statement/package/import; generics/type set/alias; generic method·struct literal selector key | `B01, B02, B03, B04, V18, V27` (6) | 57 | [2346a3ab1bb3](https://github.com/tree-sitter/tree-sitter-go/tree/2346a3ab1bb3857b48b29d779a1ef9799a248cd7) | MIT |
| ★ `python` | 3.8 → 3.14 | indentation/literal/f-string, expression/comprehension, statement/definition; 3.8·3.9·3.10·3.11·3.12·3.13·3.14 delta | `B01, B02, B03, V38, V39, V310, V311, V312, V313, V314` (10) | 49 | [26855eabccb1](https://github.com/tree-sitter/tree-sitter-python/tree/26855eabccb19c6abf499fbc5b8dc7cc9ab8bc64) | MIT |
| `javascript` | ES5.1 → ECMAScript2026 | lexical/ASI·expression·statement·function/object, regexp, ES2015·2016–19·2020–21·2022–25 delta, script/Annex B | `B01, V15, V16, V20, V22, B02, L01` (7) | 58 | [58404d8cf191](https://github.com/tree-sitter/tree-sitter-javascript/tree/58404d8cf191d69f2674a8fd507bd5776f46cb11) | MIT |
| `jsx` | classic JSX → 고정 JSX spec | element/name, attribute/spread, text/entity/expression/fragment, JavaScript REQ 합성 | `B01, B02, B03, B04` (4) | 58 | [58404d8cf191](https://github.com/tree-sitter/tree-sitter-javascript/tree/58404d8cf191d69f2674a8fd507bd5776f46cb11) | MIT |
| ★ `typescript` | 2.8 → 7.0 (.ts/.d.ts, JSX 제외) | host JS·type/declaration, ambient/module/signature; 2.8–2.9·3.x·4.0–4.4·4.5–4.9·5.0–5.4·5.5–7.0 delta | `B01, B02, V28, V3, V40, V45, V50, V59` (8) | 81 | [75b3874edb2d](https://github.com/tree-sitter/tree-sitter-typescript/tree/75b3874edb2dc714fb1fd77a32013d0f8699989f), `typescript` | MIT |
| ★ `tsx` | TS2.8+JSX → TS7.0+고정 JSX | TypeScript/JSX 합성, generic-arrow/relational/JSX ambiguity·as, JSX type argument/namespaced attribute | `B01, B02, V29` (3) | 71 | [75b3874edb2d](https://github.com/tree-sitter/tree-sitter-typescript/tree/75b3874edb2dc714fb1fd77a32013d0f8699989f), `tsx` | MIT |
| `java` | Java SE8 → 27 (preview 제외) | Java8 기본; module/var, switch/text block/record/sealed, pattern/unnamed, module import/compact source/flexible constructor | `B01, V9, V14, V21, V25` (5) | 40 | [e10607b45ff7](https://github.com/tree-sitter/tree-sitter-java/tree/e10607b45ff745f5f876bfa3e94fbcc6b44bdc11) | MIT |
| `kotlin` | 1.3 → 2.4 language line | declaration/type/string, expression/control/lambda; 1.4–1.9·2.0–2.3 delta; context parameter/@all/backing field | `B01, B02, V14, V22, V24` (5) | 38 | [3dea6dfa9c01](https://github.com/tree-sitter-grammars/tree-sitter-kotlin/tree/3dea6dfa9c0129deb7c4315afbda806c85c41667) | MIT |
| `c` | C99 → C23 | C99 기본, preprocessor, C11/C17, C23 syntax/preprocessor, old-style function 등 채택 legacy union | `B01, B02, V11, V23, V23pp, L01` (6) | 58 | [b780e47fc780](https://github.com/tree-sitter/tree-sitter-c/tree/b780e47fc780ddc8da13afa35a3f4ed5c157823d) | MIT |
| `cpp` | C++11 → C++23 | C++11 기본/preprocessor, C++14/17/20/23 delta, 충돌 대안을 제외한 legacy union | `B01, V14, V17, V20, V23, L01` (6) | 77 | [c00922280863](https://github.com/tree-sitter/tree-sitter-cpp/tree/c009222808634c1014f82438d4883753516a2c24) | MIT |
| `rust` | Edition2015 → 2024 / rustc1.98.1까지 stable | lexical/item/type, expression/pattern/macro, 2018 이후 async/trait/generic, 2024/stable 추가, edition/예약어/제거 문법 경계 | `B01, B02, V18, V24, L01` (5) | 33 | [77a3747266f4](https://github.com/tree-sitter/tree-sitter-rust/tree/77a3747266f4d621d0757825e6b11edcbf991ca5) | MIT |
| `swift` | 5.0 → 6.4 | lexical/declaration/type, expression/pattern/statement/conditional compilation; 5.0–5.4·5.5–5.8·5.9–6.1·6.1·6.2·6.3–6.4 delta | `B01, B02, V51, V55, V59, V61, V62, V64` (8) | 76 | [35245fbfee2f](https://github.com/alex-pinkus/tree-sitter-swift/tree/35245fbfee2fccf16273c6f4299438fb76875970) | MIT |
| `dart` | 2.12 → 3.13 | declaration/type/library, expression/control/async; 2.13–2.19·3.0–3.3 pattern/record·3.6–3.9·3.10–3.13 delta, legacy | `B01, B02, V213, V30, V36, V310, L01` (7) | 53 | [be07cf7118d3](https://github.com/UserNobody14/tree-sitter-dart/tree/be07cf7118d3dba06236a3f19541685a68209934) | MIT |
| `php` | 5.6 → 8.5 (PHP/mixed HTML) | 기본 PHP/mixed; 5.6–7.1·7.2–7.4·8.0–8.1·8.2–8.3·8.4–8.5 delta, legacy | `B01, V56, V72, V80, V82, V84, L01` (7) | 48 | [92b5271b60be](https://github.com/tree-sitter/tree-sitter-php/tree/92b5271b60bec77fb65b5e5bc41561e8dac81299) | MIT |
| `ruby` | 2.7 → 4.0 (ERB 제외) | lexical/literal/call/block, declaration/control/rescue; pattern/forwarding·it; 4.0 줄 앞 logical continuation | `B01, B02, V27, V31, V40` (5) | 30 | [ad907a69da0c](https://github.com/tree-sitter/tree-sitter-ruby/tree/ad907a69da0c8a4f7a943a7fe012712208da6dee) | MIT |
| `r` | 3.6 → 4.6 | lexical/function/call/operator, control/body; 4.0–4.1 raw string/lambda/pipe, 4.2–4.6 pipe placeholder/extraction | `B01, B02, V40, V42` (4) | 17 | [58a22794466c](https://github.com/r-lib/tree-sitter-r/tree/58a22794466c0fc15b0d3b40531db751593721e8) | MIT |
| `bash` | 3.2 → 5.3 | word/quote/substitution/redirection, command/pipeline/control/function; 4.x–5.2 delta; 5.3 current-environment substitution | `B01, B02, V4, V53` (4) | 31 | [a06c2e4415e9](https://github.com/tree-sitter/tree-sitter-bash/tree/a06c2e4415e9bc0346c6b86d401879ffb44058f7) | MIT |
| `powershell` | Windows PowerShell5.1 syntax → PowerShell7.6 syntax | token/argument/expression/command, declaration/control/scriptblock, workflow/configuration legacy, 6.0–7.6 operator/pipeline/clean | `B01, B02, L01, V7` (4) | 39 | [e7bd348c49fd](https://github.com/airbus-cert/tree-sitter-powershell/tree/e7bd348c49fdfd5c853a146a670965ba516a6239) | MIT |
| `html` | HTML5 REC2014 → LS2026-09-28 | document/tag/attribute/text, raw-text/RCDATA/fragment/foreign integration, tokenizer/template/custom element | `B01, B02, B03` (3) | 34 | [73a3947324f6](https://github.com/tree-sitter/tree-sitter-html/tree/73a3947324f6efddf9e17c0ea58d454843590cc0) | MIT |
| `css` | CSS2.1 → Syntax3 + Snapshot2026 채택 module | CSS2.1 기본 + 아래 30 module | `B01, M01–M30` (31) | 246 | [dda5cfc5722c](https://github.com/tree-sitter/tree-sitter-css/tree/dda5cfc5722c429eaba1c910ca32c2c0c5bb1a3f) | MIT |
| ★ `json` | RFC8259 / ECMA-404 2판 (strict) | object/array/value/scalar root/whitespace; number/string/escape/Unicode/literal | `B01, B02` (2) | 23 | [254c42a64764](https://github.com/tree-sitter/tree-sitter-json/tree/254c42a6476413b776221e03982ac8ae159eeb72) | MIT |
| `yaml` | 1.1 → 1.2.2 | stream/directive/tag/anchor/alias, scalar/flow, block collection/scalar/chomping, version lexical/escape/linebreak | `B01, B02, B03, V12` (4) | 27 | [a1c4812a73ec](https://github.com/tree-sitter-grammars/tree-sitter-yaml/tree/a1c4812a73ec5e089de8e441fdea3a921e8d5079) | MIT |
| `xml` | XML1.0 5판 + XML1.1 2판 | declaration/prolog/doctype/element/namespace/reference, content/comment/PI/CDATA/internal DTD, Char/Name/line-end/version | `B01, B02, V11` (3) | 30 | [5000ae8f22d1](https://github.com/tree-sitter-grammars/tree-sitter-xml/tree/5000ae8f22d11fbe93939b05c1e37cf21117162d) | MIT |
| ★ `tsql` | SQL Server2012/compat110 → 2025/compat170 syntax | lexical/expression/type, query/DML, DDL/security/transaction, procedural/admin, GO/quoted-identifier context; 2016–19 JSON/temporal/graph, 2022–25 WINDOW/ledger/JSON/vector | `B01, B02, B03, B04, B05, V16, V22` (7) | 1,000 | [8620fbcfca94](https://github.com/meloncholera/tree-sitter-mssql/tree/8620fbcfca9438e1ff7104835bcb8305aba3bdfa) | MIT |
| `postgresql-sql` | PostgreSQL9.6 → 18 (PL/pgSQL/extension 제외) | lexical/type/expression, query/DML, DDL/security/transaction, utility/SQL body; 10–12·13–15·16–17·18 delta, legacy | `B01, B02, B03, B04, V10, V13, V16, V18, L01` (9) | 537 | [59d0d8cd7506](https://github.com/gmr/tree-sitter-postgres/tree/59d0d8cd7506d68de1229fb4bbce838c83b60c8a) | BSD-3-Clause |
| **합계** | **26 routes** | | **178 REQ 행** | **3,201** | 전체 git pin은 B1 참조 | |

CSS `M01–M30`의 정확한 module은 다음과 같다: M01 Syntax3, M02 Style Attributes, M03 Media Queries3, M04 Conditional Rules3, M05 Selectors3, M06 Namespaces3, M07 Cascade4, M08 Values3, M09 Variables1, M10 Box3, M11 Color4, M12 Backgrounds3, M13 Images3, M14 Fonts3, M15 Writing Modes3, M16 Multicol1, M17 Flexbox1, M18 UI3, M19 Containment1, M20 Transforms1, M21 Compositing1, M22 Easing1, M23 Counter Styles3, M24 Media Queries4, M25 Scroll Snap1, M26 Scrollbars1, M27 Grid1/2, M28 Cascade5, M29 Color Adjustment1, M30 Conditional4.

Grammar license는 PostgreSQL SQL의 BSD-3-Clause를 제외한 **25 routes가 MIT**다. 이는 grammar source metadata이며 각 case/W sample의 license와 같다는 뜻이 아니다.

### 사례 수 재구성

3,201 case 객체는 **3,199 requirement + 2 support**다. C#은 389+1, T-SQL은 999+1이고 나머지는 requirement다. Support는 dynamic SQL 각 1개다. 아래의 `routes`·`gaps`·`n461`·`queries`는 `src/testdata/native/<열>/<route>.json`의 `cases` 배열 길이다. Query case는 query 문자열 수가 아니다.

| Route | routes | gaps | n461 | query case | support | qualification |
|---|---:|---:|---:|---:|---:|---:|
| `csharp` | 169 | 177 | 15 | 28 | 1 | 390 |
| `go` | 54 | 0 | 0 | 3 | 0 | 57 |
| `python` | 41 | 0 | 0 | 8 | 0 | 49 |
| `javascript` | 54 | 0 | 0 | 4 | 0 | 58 |
| `jsx` | 55 | 0 | 0 | 3 | 0 | 58 |
| `typescript` | 68 | 6 | 0 | 7 | 0 | 81 |
| `tsx` | 64 | 5 | 0 | 2 | 0 | 71 |
| `java` | 35 | 0 | 0 | 5 | 0 | 40 |
| `kotlin` | 33 | 0 | 0 | 5 | 0 | 38 |
| `c` | 54 | 0 | 0 | 4 | 0 | 58 |
| `cpp` | 73 | 0 | 0 | 4 | 0 | 77 |
| `rust` | 29 | 0 | 0 | 4 | 0 | 33 |
| `swift` | 61 | 7 | 0 | 8 | 0 | 76 |
| `dart` | 46 | 0 | 0 | 7 | 0 | 53 |
| `php` | 43 | 0 | 0 | 5 | 0 | 48 |
| `ruby` | 26 | 0 | 0 | 4 | 0 | 30 |
| `r` | 14 | 0 | 0 | 3 | 0 | 17 |
| `bash` | 29 | 0 | 0 | 2 | 0 | 31 |
| `powershell` | 36 | 0 | 0 | 3 | 0 | 39 |
| `html` | 32 | 0 | 0 | 2 | 0 | 34 |
| `css` | 220 | 0 | 0 | 26 | 0 | 246 |
| `json` | 22 | 0 | 0 | 1 | 0 | 23 |
| `yaml` | 26 | 0 | 0 | 1 | 0 | 27 |
| `xml` | 23 | 0 | 6 | 1 | 0 | 30 |
| `tsql` | 640 | 353 | 0 | 6 | 1 | 1000 |
| `postgresql-sql` | 517 | 12 | 0 | 8 | 0 | 537 |
| **합계** | **2464** | **560** | **21** | **154** | **2** | **3201** |

조립 규칙은 `src/internal/foundation/qualification_test.go`의 `TestUpdateQualificationInventory` 주변에 있다. Routes/gaps/n461 case를 합치고, query case의 `source_case`로 기존 route source를 연결한 뒤 dynamic SQL support를 더한다. Source를 재사용하는 query case도 별도 객체이므로 3,201을 고유 source 수라고 부르지 않는다.

### go-treesitter에서 재사용할 corpus와 기대값 경계

| 재사용 원천 | 인계 내용·조건 |
|---|---|
| `src/testdata/native/routes/<route>.json` | 26 route의 `source_utf8`·edits·feature ID·버전/모드와 사례 설명 |
| `src/testdata/native/gaps/{csharp,typescript,tsx,swift,tsql,postgresql-sql}.json` | 결함·경계·recovery 회귀의 원문과 편집. 디렉터리 이름 gaps가 현재 미수정이라는 뜻은 아님 |
| `src/testdata/native/n461/{csharp,xml}.json` | 업무 source 경계. `n461/svc.json`은 26 routes 밖의 별도 composite 형식 |
| `src/testdata/native/queries/<route>.json` | query 문자열·source_case 연결. 구문 corpus와 query 의미 기대값을 분리해 판정 |
| `src/testdata/native/dynamic-sql/csharp-command.cs`·`tsql-dynamic.sql`·`expected.json` | dynamic SQL 지원 사례. grammar syntax와 host fact 분석은 다른 계약 |
| `src/testdata/native/samples/NOTICE.md` | W sample별 repository·commit·path·copyright/license 고지. MIT·Apache-2.0·BSD-2-Clause·BSD-3-Clause·PostgreSQL 등의 개별 고지를 보존 |

원문·edit와 명세 기반 허용/거부 조건을 corpus로 가져갈 수 있다. **native 출력에서 복사한 전체 CST·node명·capture/API 결과와 hash는 독립 기대값이 아니다.** 같은 구현 계열의 회귀/reference 비교에는 쓸 수 있지만, 그 자체로 문법 정답이나 query 의미 정확성을 입증하지 않는다. `expect`·`expected`라는 필드명만으로 독립성을 판단하지 말고 각 case의 공식 사양·별도 구조 리뷰·고정 Roslyn/DB engine 같은 근거를 함께 보존한다.

독립 기대값으로 쓸 때는 명세에서 도출한 구문·span/구조 조건과 특정 외부 oracle의 관측을 구분한다. Kit의 patched grammar용 node type·CST expectation이 unmodified upstream에 실패하면 우선 B1의 patch/known gap과 대조하고 새 pin에서 재판정한다. Grammar pin·runtime·generator가 달라지면 예전 PASS, platform qualification, API agreement를 자동 승계하지 않는다.

### 이번 인계에서 수행한 확인

PowerShell 7의 JSON decoder와 배열 집계로 26 route 집합, 97 chain pointer/target/field, 1,423 operation, 503 C2 label, 178 REQ 행, 3,201 case 및 source 조립 수를 대조했다. 패치 source·관련 보고서·현재 계약을 교차 읽어 역사적 실패와 현행 gap을 구분했다. 작성 산출물의 로컬 절대 경로 부재·링크 대상·UTF-8/역슬래시·완료 표식을 정적으로 확인했다.

Go test/go run, native/generation/measurement, 새 upstream 상태 조사는 수행하지 않았다. 두 저장소의 추적 파일과 Git/GitHub에 변경을 가하지 않고 지정된 인계 파일만 작성했다. 이 문서는 새 실행 결과나 upstream 기여·공개 완료의 증거가 아니다.


