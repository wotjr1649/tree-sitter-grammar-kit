# 변경 기록

## Unreleased

- go route에 Go 사양 "Semicolons" 규칙의 자동 세미콜론 external scanner를 더했다(#98, 사용자 결정). 원래 입력 그대로의 `probe-go-B03-R-1`(닫히지 않은 `(xs[1] * 2` 뒤의 `func after()`)이 통과한다.
  - C2 patch가 고정 commit에 없는 파일을 만들 수 있다. subject의 file 항목에 `"create": true`를 두고, operation 하나(빈 `before`, `occurrences` 0, 파일 전체를 `after`)로 내용을 준다. `prepare-routes.ps1`은 그런 파일을 빈 text에서 시작하고, 고정 commit에 파일이 있으면 실패한다. 빈 `before`는 `create` 파일의 첫 operation에서만 받는다. `wire.py`는 만든 파일을 route 파일(role `scanner`, origin `patched`)로 등록한다.
  - foundation 시험이 다음을 거부한다: 고정 commit에 있는 파일(upstream scanner·공유 파일, 생성기 입력, adoption 파일, upstream route 파일)을 만드는 C2 subject, 그리고 `c2_patch`·native patched file·route 파일(origin `patched`, role `scanner`) 중 어디에든 고정되지 않은 만든 파일. source 기록 쪽에서는 만든 파일이 `scanner_and_shared`에 없고 `c2_patch`에 고정돼 있어야 한다. 각 규칙에 음성 대조 시험이 있다.
  - go scanner(`src/scanner.c`, 새 파일)는 줄 끝 terminator를 external token으로 낸다. scanner는 앞 token을 볼 수 없으므로 parse state로 판단한다. terminator가 유효하면 문장·선언·spec·field가 막 끝난 것이고, scanner가 읽는 `||`가 유효하면 완결된 식 뒤다. 둘 다 사양의 줄 끝 token(identifier, literal, `break`·`continue`·`fallthrough`·`return`, `++`·`--`·`)`·`]`·`}`)으로 끝난다. 이때 줄바꿈(또는 EOF, 줄바꿈을 품은 `/* */` 주석)이 세미콜론이다. 괄호 안처럼 `||`만 유효한 곳의 세미콜론은 Go처럼 구문 오류다: `f(a` 줄바꿈 `)`, 쉼표 없는 composite literal 끝, `if x` 줄바꿈 `{`, 줄 앞 `.b()`. 그 밖의 줄바꿈은 공백이다.
  - 결과: go의 등록 사례가 routes·oracle 모두 PASS다. corpus와 examples의 ERROR 수는 그대로다. tree가 바뀐 곳은 `examples/proc.go`·`value.go`의 case 절 `statement_list` 끝이 뒤따르는 빈 줄을 더는 포함하지 않는 것뿐이다(terminator가 문장 바로 뒤 줄바꿈이다). node-types는 바뀌지 않았다. 남은 한계: 괄호 안의 type 뒤 줄바꿈(`func f(a int` 줄바꿈 `)`, `make(map[K]V` 줄바꿈 `)`)은 `||`가 유효하지 않아 공백으로 읽고 받아들인다.

- C2 grammar patch를 batch A의 17 route에 연결했다(#76, `C2-PATCH-r1`). `wire.py`로 세 등록부(chain, patched file, 출력 6개 고정)를 갱신했고, Windows amd64에서 route마다 `prepare-routes.ps1`이 등록 출력을 재현한 뒤 `run-routes.ps1 -Oracle`로 main의 Windows baseline 실행과 사례별로 비교했다. qualification inventory를 다시 생성했다.
  - Windows 결과: 17 route 모두 prepare가 등록 출력을 재현했고, routes 절의 등록 사례는 go `probe-go-B03-R-1` 하나를 빼고 모두 PASS다(baseline PASS에서 FAIL이 된 사례 없음, `reused`가 0으로 떨어진 step 없음). `probe-go-B03-R-1`은 main과 같은 입력으로 되돌렸고 baseline에서도 FAIL이다(아래 mixed1). oracle 절의 non-PASS는 같은 `probe-go-B03-R-1`과, 판정에 들어가지 않는 API claim 전용 finding 72건이다(swift 64, python 7, xml 1, 모두 baseline에서도 non-PASS). php `alt-php-L01-05`는 main에서 버전 충돌로 빠졌다(#91).
  - mixed2(php, ruby, r, bash, powershell): php는 legacy `$str{0}` offset, heredoc 닫는 식별자 들여쓰기, PHP 8.5 `clone($o, [...])`와 8.x 구문을 받는다. ruby는 EOF에서 끝나는 heredoc, `||`로 시작하는 연속 줄(Ruby 4.0), `__FILE__` 같은 keyword 변수와 escape를 고쳤다. r은 `|>` 오른쪽이 call이 아닌 경우, `$`/`@` 뒤 이름이 없는 경우, placeholder `_` 위치를 오류로 둔다. r scanner는 오류 복구 중에도 가장 안쪽 scope와 짝이 맞는 닫는 괄호(`)`, `}`, `]`, `]]`)를 token으로 내고 scope를 닫는다. 그래서 `(x <)`처럼 피연산자가 빠지면 MISSING identifier로 복구되고, 열린 괄호 안처럼 줄바꿈을 건너뛰며 뒤 문장을 삼키지 않는다(`probe-r-B02-R-1`이 main의 입력 그대로 통과한다). corpus의 ERROR 입력 수는 그대로이고, 오류 입력 3개(`function(x = function()) {}`, `()`, `fn(NA = )`)의 오류 범위가 좁아졌다. bash는 Bash 5.3 `${ list; }`·`${| list; }`, brace 범위 `{a..e}`·`{1..10..2}`, `{fd}>file`, `coproc`, redirect를 고쳤다. powershell은 끝 `&` job 연산자, `clean`·최상위 named block, 빈 `{ }`, en/em dash 연산자·parameter, 둥근 따옴표 문자열, real `d` 접미사, label 있는 break/continue, `-Path:` 콜론, 숫자로 시작하는 pipeline, `using`과 class 구문 등 23개 결함을 고쳤다. 새 node type: bash `coproc_statement`, powershell `null_coalescing_expression`·`ternary_expression`·`using_statement`. 사례 수정: ruby `probe-ruby-V40-PNRE-1`의 step 0/2/4에 `binary` anchor `user.admin?\n    or user.owner?`를 더했다(worklist case_defect: `or` 연속 줄이 contains만으로 통과하던 위험). `alt-powershell-B02-06`의 `class_statement` anchor가 attribute `[NoRunspaceAffinity()]`부터 시작한다(about_Classes, pwsh 7.6 parser [136,475)). r corpus의 ERROR 증가는 무효 R 입력이라 받아들였다.
  - mixed1(go, java, rust, python, postgresql-sql): go는 generic method 선언(Go 1.27), `s[1,]`, 비-type operand 인스턴스화를 받고 최상위 statement를 오류로 둔다. java는 flexible constructor body, `import module`, `...` 앞 annotation, 연속 `_` 숫자 literal, identifier 안 Unicode escape, case pattern 여러 개, 끝의 SUB를 받는다. rust는 extern block의 `safe`/`unsafe` 항목, 빈 where bound, 타입 경로의 `::<` 등 17개 결함을 고쳤고, shebang과 line doc comment token은 Reference대로 줄바꿈을 포함한다(`alt-rust-B01-01` anchor를 LF 포함으로 고쳤다). python은 매개변수 `/`·`*` 순서, except/except* 혼용, PEP 696 type parameter 기본값 등을 고쳤다. postgresql-sql은 `U&"..."`/`U&'...'` [UESCAPE], legacy `SET WITH OIDS`·COPY `OIDS` 등을 받는다. 새 node type: python `as_pattern_target`·`default_type_parameter`, postgresql-sql `opt_oids`·`unicode_quoted_identifier`·`unicode_string_literal`. 사례 수정: python V313/V38 probe edit 범위, postgresql-sql L01-05의 모순 anchor 삭제. go·rust corpus의 ERROR 증가는 받아들였다. 위험: postgresql-sql `parser.c`가 104.3 MB로 build 파일 상한 104.86 MB에 가깝다. trade-off: postgresql-sql의 legacy postfix 연산자(`SELECT 5 !`, 9.6 gram.y)는 parser 크기 상한 안에 머물려고 select-list 항목(`target_el`) 끝에서만 받는다. 남은 FAIL: go `probe-go-B03-R-1` step 1(닫히지 않은 `(` 뒤 줄바꿈)은 main 입력 그대로 두었다. tree-sitter-go는 괄호 안 줄바꿈을 공백으로 읽어(Go 사양의 세미콜론 자동 삽입이 없다) 다음 줄들이 같은 식으로 이어지고, cost 기반 복구가 `compute`의 본문 `{`부터 파일 끝까지를 ERROR 하나로 묶어 `func after()` 선언을 잃는다. 최상위 선언 사이 terminator 생략, 본문 필수 function 선언, 괄호 안 줄바꿈 token화의 세 grammar 시도 모두 이 선택을 바꾸지 못했다.
  - markup(json, xml, html, css): json은 RFC 8259 strict다(사용자 결정). `\uXXXX` escape 전체가 escape_sequence이고, 주석, top-level 값 0개·여러 개, 문자열 안 원시 제어 문자가 ERROR다. `comment` node type이 없어졌고 corpus ERROR가 0에서 2(`Comments`, `Multiple top-level objects`)가 됐다. JSONC·JSON Lines 입력은 이제 ERROR다. xml은 XML 1.0 Name/Nmtoken 문자, 내용 있는 PI의 끝, 필수 DefaultDecl, Char만 허용하는 내용, `NOTATION` 목록, EntityValue 안 `<`, intSubset의 DeclSep을 고쳤다. html은 implied end tag, RCDATA·plaintext·script escape 상태, foreign content namespace와 CDATA, character reference, processing instruction, `--!>` comment, 속성 구분, raw text EOF 복구를 고쳤다. character reference는 관대하게 받는다: `&foo;`, `&#65`, `;` 없는 `&amp`가 오류 node 없이 `entity`가 된다. WHATWG는 이들을 parse error로 다루지만 upstream도 같게 동작했다. 새 node type: `cdata_section`, `erroneous_comment`, `processing_instruction`(WHATWG 13.2.5.72-76 PI 상태에 따른 일반 node). `erroneous_comment`를 html `error_nodes`에 더했고 `probe-html-B03-R-1`은 step 0 `NO_ERROR`에 `contains` [`erroneous_comment`, `element`]로 이 node를 관측한다. css는 MQ4 range, general-enclosed, `not`/`only` 범위, generic·`@layer` prelude, escape된 이름, custom property, An+B, url-token, math function의 `/`, 빈 statement, CDO/CDC, 복구를 고쳤고, 선언 값 안의 `{}` block과 at-keyword를 받는다(CSS Syntax 2021 consume-a-declaration, 둘 다 유효하면 GLR이 중첩 rule을 고른다). 공백 descendant combinator는 문장 끝(`;`, `}`) 전에 `{`가 올 때만 combinator다. 이 lookahead는 주석, 따옴표 문자열과 escape를 건너뛰므로 `.a .b, /* legacy; drop */ .c { }`, `a [title="x;y"] { }`, `.a .b /* } */ { }`가 descendant selector로 남는다(`alt-css-M05-07`..`09`가 고정한다). 새 node type: `general_enclosed`, `range_query`. css-M02(declaration list 진입점)는 설계 제안만 남기고 미뤘다. 지금 사례는 stylesheet root로 통과한다.
  - mobile(dart, kotlin, swift): dart는 `C.new` tear-off, 내장 식별자 `Function`을 이름·object pattern type으로 쓰는 경우, `List<int>.filled(...)` 등을 고쳤고 `V30-03` 사례의 anchor occurrence를 고쳤다. kotlin은 class 본문 끝 member 뒤 같은 줄의 `}`, multi-dollar 문자열, 숫자 구분자, 한 줄 문자열의 Unicode escape, `$name` 보간, 세 따옴표 닫기 등 22개 결함을 grammar·scanner patch로 고쳤고 worklist의 case_defect를 반영했다. `alt-kotlin-B02-03`에서 사양에 없는 `fun <T> (t: T) where T : Any = t`(anonymousFunction에는 typeParameters가 없다)를 `fun(t: Int): Int where Int : Any = t`로 바꿔 `type_constraints` anchor와 `kotlin-B01.a452`(typeConstraints)를 유지했다. 새 node type `context_parameters`. `alt-kotlin-V22-01`의 `contains`에서 `if_expression`을 뺐다(when guard의 `'if' expression`은 if 식이 아니다). swift는 grammar·scanner patch로 38개 결함을 고쳤다. 새 node type: `computed_borrow`, `computed_mutate`, `inline_array_type`, `module_qualified_expression`, `module_selector`. 위험: swift corpus의 ERROR가 늘었다(이모지 ⭐☁☀♂는 TSPL operator 문자라 사양 token 규칙대로 받아들였다). swift `any P & Q?`를 이제 받는다(이를 고정하는 사례는 없다).
  - registry 음성 대조 시험의 재생성 전용 route는 C2 patch를 뗀 go로 만든다. `patched origin without a chain`은 C2 patch가 scanner를 건드리지 않는 dart의 `src/scanner.c`를 쓴다(r의 C2 patch가 이제 scanner를 고친다).

- 26 route 어느 것이든 저장소 안 C2 patch를 실을 수 있게 했다(#76, `C2-PATCH-r1`: 모든 route를 FULL PASS까지 patch할 수 있다는 사용자 결정). 첫 patch는 yaml `yaml-P2-01`이다.
  - patch subject는 `src/dev/c2-patches/<route>.json`(`tsgk-c2-patch/r1`)이다. `language-sources.json`의 route에 `c2_patch` 기록(`decision`, `subjects`, chain 전체 뒤의 `patched_files` hash)을 둔다. `native-routes.json`의 chain은 채택 chain(채택 route만) 뒤에 C2 subject의 `/files/<i>` 단계가 순서대로 온다. native `patched_files`는 C2 기록의 것이다. 채택 6 route의 adoption 기록은 P05 identity 그대로다. 채택 route의 C2 기록은 adoption의 patched file을 모두 담고, C2 단계가 대상으로 삼지 않는 file은 adoption hash 그대로다. C2 patch가 있는 route는 채택 route라도 `reproduction-routes.json`이 출력 6개를 모두 고정한다.
  - foundation 시험이 다음을 거부한다: 기록 없는 C2 chain, chain 없는 기록, 다른 route의 subject, C2 단계의 pointer·target 불일치, native와 기록의 patched file hash 불일치, chain target이 아닌 patched file과 patched file 없는 target, upstream 그대로 쓰는 patched file, 출력 고정 누락·불일치(채택 route 포함), adoption subject가 아닌 채택 chain 단계, 채택 route의 C2 기록에서 빠진 adoption patched file, C2 단계가 대상으로 삼지 않는데 adoption hash와 다른 file, adoption patched file도 C2 대상도 아닌 C2 기록 file, adoption hash가 없는 file을 대상으로 하는 채택 chain 단계, C2 기록이 없는 채택 route에서 adoption 기록과 다른 native patched file, C2 기록의 decision·subject 경로·patched file 형식 오류. 각 규칙에 음성 대조 시험이 있다.
  - `prepare-routes.ps1`은 채택 단계까지의 결과를 `language-sources.json`의 adoption hash와 대조한 뒤 C2 단계를 적용하고, 결과를 native 등록부의 patched file hash와 대조한다. `node_modules/<package>/…` patch 대상의 원문은 등록 npm tarball에서 가져온다(cpp의 tree-sitter-c, typescript·tsx의 tree-sitter-javascript patch용). 등록되지 않은 package면 실패한다.
  - yaml scanner가 줄 첫 칸(column 0)의 BOM(U+FEFF)을 두 경우에만 받는다(YAML 1.2.2 [202] `l-document-prefix`, [211] `l-yaml-stream`). directive가 올 수 있는 document prefix(stream 시작, `...` 뒤)에서는 BOM을 건너뛰고 그 뒤를 평소처럼 읽는다. 그 밖에서는 BOM 바로 뒤, 사이에 아무것도 없이 column 0의 `---`가 올 때만 받고(열린 block collection은 먼저 닫는다), 그렇지 않으면 오류다. BOM은 들여쓰기로 세지 않는다. 따옴표 scalar 문맥과 오류 복구 호출(`ERR_REC`)에서는 BOM을 건너뛰지 않는다. BOM 줄이 있는 무효 입력은 계속 오류지만 일부는 오류 tree 모양이 바뀌었다(예: corpus `Invalid use of BOM`). scanner.c는 생성기 입력이 아니므로 출력 6개는 바뀌지 않았다. `alt-yaml-V12-01`이 PASS가 됐고, 사례 4개를 등록했다: `alt-yaml-V12-04`(prefix 밖, block mapping 뒤 `\uFEFF---`, NO_ERROR)와 `probe-yaml-V12-NR-1~3`(prefix 밖 `\uFEFF   ---`, `\uFEFF\n  ---`, `\uFEFFkey: v`, ERROR). 알려진 한계: [211]은 prefix 밖에서도 BOM 뒤에 주석·빈 줄·`...`·입력 끝을 허용하지만, 이 scanner는 이 경우를 모두 오류로 둔다(patch 전에도 오류였다).
  - qualification inventory(`qualification-c1.json`)를 다시 생성했다(yaml `src/scanner.c` hash와 새 사례 4개).

- 버전 충돌 legacy 대안을 구문 합집합에서 뺐다(#91, #76 사용자 결정). 같은 byte가 채택 범위의 두 version에서 각각 유효한 다른 구문이라 grammar 하나로 함께 검증할 수 없는 대안이다: C++17 이전 `module`/`import` 줄(`cpp-L01.a13`), C++23 이전 `arr[1, 2]`(`cpp-L01.a18`), PHP 7.0에서 제거된 ASP·script tag(`php-L01.a006`·`a007`), 8.0 이전 `#[` 주석(`php-L01.a009`). 사양 인용은 [disposition 문서](docs/validation/language-feature-disposition.md#읽는-법과-완료-경계)에 있다.
  - 이 대안만 담던 사례 `alt-cpp-L01-06`·`09`, `alt-php-L01-05`를 뺐다. `alt-php-L01-04`는 여전히 유효한 `<?` short tag(`php-L01.a008`)만 남겼다.
  - 이후 version에서 ill-formed가 될 뿐인 형식(`import<int> f();`, `Y<operator<=> y;`, N3337 attribute 위치)은 충돌이 아니라 REQ로 남는다.

- 비채택 20 route도 고정 생성기 재생성 경로에 올렸다(#89, `C2-REGENERATE-r1`: #76에서 사용자가 승인한 26 route patch 경로의 첫 단계). grammar는 바꾸지 않았다.
  - 20 route의 native 입력 `parser.c`·`tree_sitter/*.h`는 이제 upstream에 들어 있는 이전 CLI 생성물이 아니다. 고정 commit의 patch 없는 grammar를 tree-sitter 0.27.0·Node 24.21.0·ABI 15로 재생성한 출력이다. `native-routes.json`의 `regeneration`은 patch chain과 patched file이 비어 있다. `language-sources.json`의 `regeneration` 기록과 `reproduction-routes.json`의 `PREPARE_GENERATED` 기준이 출력 6개를 모두 고정한다. Windows에서 20 route 모두 두 작업 공간 출력이 byte 단위로 같았고 S04 재생성 출력과도 같았다. php는 upstream 생성물과 같고, 나머지 19 route는 `array.h` 등이 다르다.
  - foundation 시험은 두 종류의 route만 받는다. 하나는 기존 규칙 그대로인 채택 route이고, 다른 하나는 adoption·patch 없이 출력이 고정된 재생성 전용 route다. 다음은 거부한다: 둘 다 아니거나 둘 다인 route, upstream parser, chain 없는 patched file, 빠지거나 다른 출력 고정, 재생성 기록이 있는 upstream 기준. 이 거부 사례마다 음성 대조 시험이 있다.
  - qualification inventory(`qualification-c1.json`)의 grammar 파일 hash를 등록부에서 다시 생성했다.
  - Windows amd64에서 등록 사례를 baseline(main 6d151e0)과 비교했다. 같은 사례와 같은 compiler를 썼고, 19 route는 새 parser로 build했다. 판정·code·claim·기대값 실패가 바뀐 사례는 없다(s05 2394, s06 2532). s05 step tree digest 3007개도 모두 같다. Linux·macOS의 native 결과는 이 변경 뒤 CI가 근거다.

- native CI의 요구 축과 kit 축을 분리했다(#76). 오늘의 grammar에서 정직하게 실패하는 등록 사례가 kit 결함처럼 job과 mechanism gate를 실패시켰다.
  - 오류 tree 위의 incremental route를 BLOCKED로 둔다. route 계측이 있고 바뀐 것이 있는데 incremental·fresh 어느 parse도 node를 재사용하지 않은 edit step에서, 이전 tree와 새 incremental tree가 모두 full tree이고 그중 하나가 `has_error`인 경우다. summary·record 형식 tree는 replay가 `has_error`를 다시 계산하지 못하므로 FAIL로 남는다. tree-sitter는 오류 tree에서도 node를 재사용할 수 있지만, grammar의 오류 tree에서는 재사용할 node가 남지 않을 수 있으므로 이 무재사용을 kit 결함이 아니라 관측 불가로 취급한다. code는 `INCREMENTAL_ROUTE_UNOBSERVABLE_ERROR_TREE_STEP_<n>`이고, 다른 실패 code가 없을 때만 사례 code가 된다. 깨끗한 tree, 계측 없음, 변경 없음, fresh 재사용은 그대로 FAIL이다. S05 판정, r2 replay reducer와 qualify의 `incremental-route` gate가 같은 규칙으로 다시 계산한다.
  - replay reducer `native-result-r2`·`oracle-set-r2`·`private-corpus-r2`를 더했다(#87). 각 r1과 같고 이 route 규칙만 더한다. r1은 그대로 남아 규칙 이전의 근거를 전과 같은 결과로 replay한다. reducer는 registration의 `reducer` id로 고른다. `tsgk qualify`는 새 CI 근거를 r2 규칙으로 판정한다.
  - qualify는 route BLOCKED를 `KIT_CLAIM_FAILED`로 세지 않는다. kit 축은 PASS로 남고 E 의무가 BLOCKED가 되므로 칸은 `INCOMPLETE`다. FULL PASS에는 여전히 E PASS가 필요하다. 기록과 다시 계산한 route claim이 다르면 gate 실패다.
  - `run-routes.ps1`은 oracle `query_expectations` FAIL·BLOCKED와 route BLOCKED로 job을 실패시키지 않는다. 이 결과는 요구 축 결과(Q는 `tsgk qualify`가 판정)로 보고, 실패 내용과 함께 `summary.json`의 `requirement_results`에 기록한다. 그 밖의 job 실패 조건은 그대로다. set 검증 실패, error finding, build 실패·거부, 완료되지 않은 사례, incremental equality FAIL·BLOCKED, route FAIL, query equality·fact reproduction·dynamic SQL FAIL·BLOCKED가 이에 해당한다.
  - 결과·record schema는 claim 값(`BLOCKED` 포함)과 code를 열거하지 않으므로 revision을 올리지 않았다.
  - 실제 route 결함이 오류 tree BLOCKED 뒤에 숨지 않도록, foundation 시험이 등록 route마다 route·gap 사례 파일(n461 제외)에 모든 step이 `NO_ERROR`인 edit 사례가 하나 이상 있는지 확인한다(#87). 지금 26 route 모두 충족한다.
  - 알려진 한계: SVC 관측 전용 기록은 parse하지 않으므로 route claim이 없고 route gate를 거치지 않는다(#87). 이런 사례는 route 근거가 되지 않는다.
- native `incremental_equality` claim이 앞 step의 FAIL 뒤에 비교 없는 step이 오면 BLOCKED로 덮이던 결함을 고쳤다(#76). 이제 replay처럼 step 중 가장 나쁜 값(FAIL > BLOCKED > PASS)이다.

- 단계 기대값에 anchor를 더했다(#76). `contains`는 같은 type이 다른 줄에 있어도 통과하므로, 대상 구조를 잘못 parse한 사례가 통과할 수 있었다. TypeScript generic tagged template이 `binary_expression`으로 parse됐는데도 다른 곳의 `call_expression` 때문에 통과한 것이 그 예다.
  - 기대값의 선택 필드 `anchors`(`{type, start_byte, end_byte}`)는 그 step의 full tree에 정확히 그 type·byte 범위의 named node가 있어야 통과한다. 없으면 FAIL, full tree가 아니면 BLOCKED다. S05 판정, replay, qualify가 같은 규칙으로 다시 계산한다. anchor가 없는 기대값의 판정은 그대로다.
  - profile이 `tsgk-incremental/r2`·`tsgk-oracle/r2`, inventory가 `tsgk-qualification-inventory/r3`이 됐다. 형식이 틀리거나 step source 밖인 anchor는 profile에서 `EXPECT_ANCHOR_INVALID`, inventory에서 `CASE_INVALID`다. r1 profile은 이전 근거를 replay할 수 있게 anchor 없이 계속 읽는다.
  - 원본 사례 파일은 anchor를 `{type, text, occurrence}`로 적는다. `run-routes.ps1`과 inventory 생성기가 같은 방식으로 byte 범위로 바꾸고, foundation test가 둘을 독립 구현과 대조한다. 아직 anchor를 단 등록 사례는 없다.
- `DeriveDeclarations`가 한 match 안에 같은 level의 node가 여럿일 때 마지막 node만 남기던 결함을 고쳤다. PostgreSQL `CREATE TABLE … PARTITION OF …`의 선언 이름이 이 결함 때문에 잘못 나왔다(#77).
- 개발 helper `run-routes.ps1`이 capture 기대값을 잘못 직렬화하던 결함을 고쳤다(#77).
  - capture가 하나이면 scalar가 되어 `JSON_TYPE`으로 거부됐다.
  - capture가 0개이면 `null`, 즉 "검사하지 않음"이 됐다.
- `tsgk qualify` 결과가 `tsgk-qualification-result/r2`가 됐다. r1에 platform별 지원 claim `platform_claims`(platform id → `SUPPORTED`|`BLOCKED`)를 더한다. platform은 근거 무결성(완결성 PASS, finding 없음), 그 platform의 모든 칸 kit 축·요구 축 PASS, 실행된 추가 역할 행 PASS일 때만 `SUPPORTED`다. platform 간 비교는 넣지 않는다. 전역 `support_claim`·`mechanism_gate`·assessment·exit는 그대로다.
- qualification inventory 사례에 `expect_assessment`·`expect_code`를 더했다. SVC 관측 전용이고 요구 행을 덮지 않는 사례만 등록할 수 있으며(그 밖은 `CASE_INVALID`), 기록된 판정과 code가 등록값과 같을 때만 등록 검사가 PASS이고 다르면 FAIL이다. `n461-svc`의 의도된 BLOCKED 8사례(`SVC_INLINE_UNRESOLVED` 4, `SVC_INLINE_UNSUPPORTED` 1, `SVC_INLINE_NOT_PARSED` 1, `SVC_DIRECTIVE_DIAGNOSTICS` 2)를 등록했다. 그 역할 행의 PASS는 다음 CI qualification 근거로 확인한다.
- coverage 규칙이 `tsgk-coverage-rule/r2`, inventory가 `tsgk-qualification-inventory/r2`, 결과가 `tsgk-qualification-result/r3`이 됐다(#76).
  - 행의 P 의무는 production 대안 등록부 `src/contracts/feature-alternatives.json`에서 그 행이 `COMPLETE`이고, 모든 대안을 그 대안을 적은(`alternatives`) P 사례가 덮을 때만 덮인다. 그 밖은 `NOT_COVERED`다. 결과의 P 의무에 `alternatives`·`alternatives_uncovered`를 더한다.
  - 등록부는 178개 REQ 행 전부를 `PENDING`, 대안 없음으로 시작한다. 따라서 지금은 모든 P 의무가 `NOT_COVERED`이고 그 칸은 PASS가 되지 않는다. 정책이 처음부터 요구한 경계를 반영한 것이다.
  - route별 `error_nodes`(`native-routes.json`, html은 `erroneous_end_tag`)를 등록했다. 이 node를 `contains`에 적은 `NO_ERROR` step은 N, 다른 구조 node가 함께 있으면 R을 만든다. P와 W는 만들지 않는다. 이런 route에서 P·W step은 기록된 full tree에 error node가 없어야 한다(`has_error`는 이 node를 보지 않는다). 있으면 FAIL이고, full tree가 아니면 BLOCKED다(#81).
  - W 생산자를 등록했다. `sample`(repository·commit·path·SPDX license·sha256·bytes)이 있는 요구 사례가 대상이다. license는 MIT·Apache-2.0·BSD-2/3-Clause·PostgreSQL이다. 크기는 65536 bytes 이하이고 step 0 tree는 10000 node 이하다. step 0이 `NO_ERROR`를 기대하는 사례가 W를 덮는다. sample마다 `src/testdata/native/samples/NOTICE.md`에 고지 항목이 있어야 한다.

## v0.1.0 — 2026-10-04

첫 release의 범위는 [제품 범위](docs/specs/scope.md)가 정하고, 근거와 한계는 [플랫폼 지원 계약](docs/specs/platform-support.md)에 있다. tag `v0.1.0`(`3625072`)과 GitHub Release로 소스만 발행했다. binary와 package는 없다.

### VERIFIED(Windows amd64, Linux amd64, macOS arm64)

- offline core와 그 공개 API(`src/kit`): `inspect`, `identity`, `verify`(디렉터리와 ZIP, 압축을 풀지 않음), `schema check`/`schema diff`. CGO-free이고 CLI와 API 결과 bytes가 같다.

### experimental(호환성 약속 없음)

- `corpus`(비공개 corpus inventory)
- `reproduce`(두 작업 공간 생성 비교)
- `incremental`·`oracle record`(native parse/edit/query 기록; 승인 capability와 host compiler 필요)
- `replay`·`evidence verify`
- `qualify`(26 route × 3 platform 집계)

### 상태

- 26 route 지원 claim은 **BLOCKED**다. 필수 feature 사례가 부족하고(#70) 채택 route에 grammar gap이 남아 있다. 최종 qualification 근거는 main `03ea1f0`의 CI run 37184449649이다.

### 알려진 한계

- tree-sitter runtime의 field 조회와 cursor가 다른 API claim FAIL 23 사례(6 route)
- T-SQL 과잉 수용(r6에서 3종, r5부터 2종)
- `--out`이 `subst`·bind mount 별칭을 검출하지 못함
- 동적 SQL 탐지는 첫 문장이 아닌 `;sp_executesql`을 known miss로 둠
- BrightScript·cooklang 역할 NOT_RUN, `n461-svc` 역할 INCOMPLETE(SVC negative 기대값 8개, #70)
- `n461-large` 대형 실사용 profile은 Windows에서만 실행(Linux·macOS NOT_APPLICABLE)
- 시험 시간 여유(#65)

### Session 이력

- Session 00: Go CGO-free foundation, 제품·검증 계약, 세 OS CI와 순차 개발 campaign을 준비한다.
- Session 01: `tsgk inspect`/`identity`/`corpus`와 공개 offline API `src/kit`를 구현한다. 다음을 포함한다.
  - manifest r2(판별 encoding 결속), E0 report
  - known-paths discovery와 정적 closure 관측
  - no-follow guard, 유한 한도, kit wall과 caller 취소의 구분
  - no-clobber `--out`
  - 비공개 corpus inventory(N461 역할, PRESENCE_ONLY, `.csproj` 선언 관측)
- Session 02~07: strict verify, schema, reproduce, incremental, oracle record, replay·evidence verify (각 Session 보고서 참고).
- Session 08: 다음을 더한다. 지원 claim은 BLOCKED다.
  - `tsgk qualify`와 `kit.Qualify`
  - qualification inventory(`src/contracts/qualification-c1.json`)
  - CI의 host 실행 identity·기록 set 보관과 qualification job
  - module proxy 소비자 시험
  - T-SQL patch r6
- release 후보 준비(#72):
  - `src/contracts/campaign-01.json`에 Session 통합 상태(`INTEGRATED`, PR, merge commit, post-merge run)를 기록하고 guard로 검사한다.
  - 플랫폼 지원 상태표를 현재 근거로 갱신한다.
  - Linux `race diagnostic` job을 추가한다(비필수, CGO 진단 lane).
