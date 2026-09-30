# PREPARE-only exact remedy work; shared run.ps1 owns containment and supervision.
function ReadRemedySubjects {
    $pins=@{
        'remedy-patches.json'='988deb9d119c753ab53f5e5b60b6571acc8442e2fc3a283beb7be2cb387546c9'
        'remedy-cases.json'='d29fb48c55cbadc3d584e02246af06937744c16524379cdd48d595f459cd4903'
        'remedy-fact-oracles.json'='af130694ac859f9d32a867af45cccfbcfdea3655d716ba4c2e8285744a3285c4'
        'remedy-sources.json'='f2c26f754bd80ee58719d5b2d12271d338938ebaed6f952a3499faf774e76bc6'
    }
    $result=@{}
    foreach($name in $pins.Keys){
        $path=Join-Path $PSScriptRoot $name
        if((Get-FileHash -LiteralPath $path).Hash.ToLowerInvariant() -cne $pins[$name]){throw 'Exact remedy subject changed'}
        $result[$name.Replace('remedy-','').Replace('.json','')]=Get-Content -LiteralPath $path -Raw|ConvertFrom-Json -AsHashtable
    }
    if($result.cases.cases.Count -ne 34 -or @($result.cases.cases|Where-Object edit).Count -ne 6 -or $result.patches.patches.Count -ne 3 -or $result.sources.sql.selected_regular_files.Count -ne 30 -or $result.sources.sql.adoption_authorized -or $result.sources.sql.grammar_patch_authorized){throw 'Remedy scope changed'}
    return $result
}
function AssertRemedyObject([long]$bytes,[string]$sha256,$pin){
    if($bytes -ne $pin.bytes -or $sha256 -cne $pin.sha256){throw 'Exact remedy object identity mismatch'}
}
function ApplyLiteralPatch([string]$source,$pin){
    $bytes=[Text.Encoding]::UTF8.GetBytes($source)
    AssertRemedyObject $bytes.Length ([Convert]::ToHexString([Security.Cryptography.SHA256]::HashData($bytes)).ToLowerInvariant()) @{bytes=$pin.original_bytes;sha256=$pin.original_sha256}
    foreach($operation in $pin.operations){
        if([regex]::Matches($source,[regex]::Escape($operation.before)).Count -ne $operation.occurrences){throw 'Literal patch occurrence mismatch'}
        $source=$source.Replace($operation.before,$operation.after)
    }
    $bytes=[Text.Encoding]::UTF8.GetBytes($source)
    AssertRemedyObject $bytes.Length ([Convert]::ToHexString([Security.Cryptography.SHA256]::HashData($bytes)).ToLowerInvariant()) @{bytes=$pin.proposed_result_bytes;sha256=$pin.proposed_result_sha256}
    return $source
}
function NewRemedyRows([string]$stage,$original,$additional){
    $rows=[Collections.Generic.List[object]]::new()
    if($stage -ceq 'patch-r1'){
        foreach($route in @('csharp','typescript','tsx','swift')){
            foreach($producer in @(($route+'-candidate'),($route+'-original'))){
                $cases=@($additional|Where-Object route -CEQ $route)
                if($producer.EndsWith('-candidate')){$cases=@($original|Where-Object route -CEQ $route)+$cases}
                foreach($case in $cases){$rows.Add(@{id=$case.id;route=$route;producer=$producer;case=$case;state='NOT_RUN';dependency='GENERATION_BUILD_AND_SAFETY_PREREQUISITES';command_label=('case-'+$producer+'-'+$case.id.ToLowerInvariant());raw_stdout=$null;exit_code=$null})}
            }
        }
        $expected=85
    }elseif($stage -ceq 'sql-pg-r1'){
        foreach($producer in @('derek-sql-candidate','postgresql-lfs-baseline','postgresql-noopt-regenerated')){
            $route=if($producer -ceq 'derek-sql-candidate'){'tsql'}else{'postgresql-sql'}
            $cases=@($original|Where-Object route -CEQ $route)
            if($route -ceq 'tsql'){$cases+=@($additional|Where-Object route -CEQ $route)}
            foreach($case in $cases){$rows.Add(@{id=$case.id;route=$route;producer=$producer;case=$case;state='NOT_RUN';dependency='GENERATION_BUILD_AND_SAFETY_PREREQUISITES';command_label=('case-'+$producer+'-'+$case.id.ToLowerInvariant());raw_stdout=$null;exit_code=$null})}
        }
        $expected=46
    }else{throw 'Unknown exact remedy stage'}
    if($rows.Count -ne $expected -or @($rows.command_label|Sort-Object -Unique).Count -ne $expected){throw 'Remedy producer/case plan mismatch'}
    return ,$rows.ToArray()
}
function PrepareRemedyInputs($subjects){
    Record 'remedy-authority-binding' @{stage=$RemedyStage;subject=$RemedyApprovalSubject;actual_user_authority='caller acceptance record separately required; subject alone is not permission';original_inputs_sha256=(Get-FileHash $inputsPath).Hash.ToLowerInvariant();prior_aggregate_sha256='68053aac3fc0dab788b48d9c78cbd577a3f182dfb7f2e092f4d0f944f64de274';prior_execution_is_not_current=$true;conditional_registration='NOT_ADOPTED';source_application_execution=$false}
    foreach($identity in @($acquisition.remedy_identities)){
        foreach($file in $identity.files){
            $relative=$identity.task_relative_source_root+'/'+$file.path
            $actual=FileIdentity (Join-Path $root $relative)
            AssertRemedyObject $actual.bytes $actual.sha256 $file
            $script:verifiedSource[$relative]=$actual.sha256
        }
    }
    if($RemedyStage -cne 'patch-r1'){return}
    foreach($patch in $subjects.patches.patches){
        $key=$patch.repository.Replace('/','--')+'--'+$patch.base_commit
        $sourceRoot=Join-Path $root ('acquisition/sources/'+$key)
        $candidateRoot=Join-Path $root ('candidates/'+$patch.id)
        if(Test-Path -LiteralPath $candidateRoot){throw 'Candidate already exists'}
        [void][IO.Directory]::CreateDirectory($candidateRoot)
        $records=@()
        foreach($file in Get-ChildItem -LiteralPath $sourceRoot -File -Recurse){
            $identity=FileIdentity $file.FullName
            if(-not $script:verifiedSource.ContainsKey($identity.path) -or $script:verifiedSource[$identity.path] -cne $identity.sha256){throw 'Candidate copy contains unverified source'}
            $relative=[IO.Path]::GetRelativePath($sourceRoot,$file.FullName).Replace('\','/')
            $target=Join-Path $candidateRoot $relative
            [void][IO.Directory]::CreateDirectory([IO.Path]::GetDirectoryName($target))
            $filePatch=@($patch.files|Where-Object target -CEQ $relative)
            if($filePatch.Count -gt 1){throw 'Duplicate file patch'}
            if($filePatch.Count){[IO.File]::WriteAllText($target,(ApplyLiteralPatch ([IO.File]::ReadAllText($file.FullName)) $filePatch[0]),[Text.UTF8Encoding]::new($false))}
            else{[IO.File]::Copy($file.FullName,$target)}
            $candidateIdentity=FileIdentity $target;$script:verifiedSource[$candidateIdentity.path]=$candidateIdentity.sha256
            $records+=@{original=$identity;candidate=$candidateIdentity;patched=[bool]$filePatch.Count}
        }
        if(@($records|Where-Object patched).Count -ne $patch.files.Count){throw 'Missing literal patch target'}
        Record ('candidate-'+$patch.id.ToLowerInvariant()) @{proposal_id=$patch.id;original_repository=$patch.repository;original_commit=$patch.base_commit;files=$records;source_repository_mutated=$false;adopted=$false;native_support='NOT_RUN'}
    }
}
function CheckSqlJsInputs([string]$entry,[string]$sourceRoot){
    $queue=[Collections.Generic.Queue[string]]::new();$queue.Enqueue($entry)
    $seen=[Collections.Generic.HashSet[string]]::new([StringComparer]::Ordinal);$identities=@()
    $prefix=[IO.Path]::GetFullPath($sourceRoot).TrimEnd('/','\')+[IO.Path]::DirectorySeparatorChar
    while($queue.Count){
        $path=[IO.Path]::GetFullPath($queue.Dequeue());if(-not $seen.Add($path)){continue}
        if(-not $path.StartsWith($prefix,[StringComparison]::Ordinal)){throw 'SQL import outside fixed root'}
        $identity=FileIdentity $path
        if(-not $script:verifiedSource.ContainsKey($identity.path) -or $script:verifiedSource[$identity.path] -cne $identity.sha256){throw 'SQL dependency not in fixed30 files'}
        $source=[IO.File]::ReadAllText($path)
        if($source -match '\b(?:require|createRequire|eval|Function)\b'){throw 'Unreviewed SQL loader/evaluation'}
        $imports=[regex]::Matches($source,'(?m)^\s*import\s+(?:[A-Za-z_$][\w$]*|\{[\w\s,$]*\})\s+from\s+["''](\.{1,2}/[A-Za-z0-9_./-]+\.js)["''];?\s*$')
        if([regex]::Matches($source,'\bimport\b').Count -ne $imports.Count){throw 'Unresolved SQL ESM import'}
        foreach($import in $imports){$queue.Enqueue((Join-Path ([IO.Path]::GetDirectoryName($path)) $import.Groups[1].Value))}
        $identities+=$identity
    }
    return ,$identities
}
function CheckRemedyQuotedIncludes([string[]]$entries,[string]$parserRoot){
    $queue=[Collections.Generic.Queue[string]]::new();foreach($entry in $entries){$queue.Enqueue($entry)}
    $seen=[Collections.Generic.HashSet[string]]::new([StringComparer]::Ordinal);$identities=@()
    $runtimeRoot=Join-Path $root 'acquisition/sources/tree-sitter--tree-sitter--659cda7c7f86ebe31cc825dc5da59e9add172dc7'
    $parserPrefix=[IO.Path]::GetFullPath($parserRoot).TrimEnd('/','\')+[IO.Path]::DirectorySeparatorChar
    while($queue.Count){
        $path=[IO.Path]::GetFullPath($queue.Dequeue());if(-not $seen.Add($path)){continue}
        if($seen.Count -gt 512){throw 'Quoted include count limit'}
        $identity=FileIdentity $path
        if($path -cne (Join-Path $root 'probe.c') -and -not $script:verifiedSource.ContainsKey($identity.path) -and -not $path.StartsWith($parserPrefix,[StringComparison]::Ordinal)){throw 'Unpinned quoted include input'}
        if($script:verifiedSource.ContainsKey($identity.path) -and $script:verifiedSource[$identity.path] -cne $identity.sha256){throw 'Quoted include bytes changed'}
        $source=[IO.File]::ReadAllText($path)
        foreach($match in [regex]::Matches($source,'(?m)^\s*#\s*include\s*"([^"\r\n]+)"')){
            $name=$match.Groups[1].Value
            if($name -notmatch '^[A-Za-z0-9_./-]+$'){throw 'Unsafe quoted include'}
            $candidates=@((Join-Path ([IO.Path]::GetDirectoryName($path)) $name),(Join-Path $parserRoot $name),(Join-Path $runtimeRoot ('lib/src/'+$name)),(Join-Path $runtimeRoot ('lib/include/'+$name)))
            $next=@($candidates|Where-Object {Test-Path -LiteralPath $_ -PathType Leaf})
            if(-not $next.Count){throw 'Quoted include input missing before build'}
            $queue.Enqueue($next[0])
        }
        $identities+=$identity
    }
    return ,$identities
}
function InvokeRemedyBuildAndCases([string]$producer,[string]$grammarName,[string]$parser,[string]$parserRoot,[string]$scanner,[string]$scannerRoot){
    if($grammarName -notmatch '^[A-Za-z_][A-Za-z0-9_]{0,63}$'){throw 'Invalid remedy entry point'}
    $label='build-'+$producer
    $entries=@((Join-Path $parserRoot 'parser.c'),(Join-Path $root 'probe.c'))
    if($scanner){$entries+=Join-Path $root $scannerRoot}
    $closure=CheckRemedyQuotedIncludes $entries $parserRoot
    $parserText=[IO.File]::ReadAllText((Join-Path $parserRoot 'parser.c'))
    if($parserText -cnotmatch '(?m)^#define LANGUAGE_VERSION ([0-9]+)\s*$'){throw 'Parser ABI unavailable before build'}
    $languageAbi=[int]$Matches[1]
    $runtimeApi=[IO.File]::ReadAllText((Join-Path $root 'acquisition/sources/tree-sitter--tree-sitter--659cda7c7f86ebe31cc825dc5da59e9add172dc7/lib/include/tree_sitter/api.h'))
    if($runtimeApi -cnotmatch '(?m)^#define TREE_SITTER_LANGUAGE_VERSION ([0-9]+)\s*$'){throw 'Runtime maximum ABI unavailable'};$maxAbi=[int]$Matches[1]
    if($runtimeApi -cnotmatch '(?m)^#define TREE_SITTER_MIN_COMPATIBLE_LANGUAGE_VERSION ([0-9]+)\s*$'){throw 'Runtime minimum ABI unavailable'};$minAbi=[int]$Matches[1]
    if($languageAbi -lt $minAbi -or $languageAbi -gt $maxAbi){throw 'Parser ABI outside fixed runtime range'}
    Record ($label+'-inputs') @{before_build=$true;files=$closure;parser_abi=$languageAbi;runtime_abi_min=$minAbi;runtime_abi_max=$maxAbi;runtime_commit=$inputs.runtime.commit;compiler_image=$toolchain.image;scanner=$scanner;source_or_candidate_identity_preserved=$true}
    $arguments=@('-std=c11','-D_DEFAULT_SOURCE','-O0','-Wall','-Wextra','-H',('-DLANGUAGE=tree_sitter_'+$grammarName),('-I'+$runtime+'/lib/include'),('-I'+$runtime+'/lib/src'),('-I'+$parser),'/inputs/probe.c',($runtime+'/lib/src/lib.c'),($parser+'/parser.c'))
    if($scanner){$arguments+=$scanner}
    $arguments+=@('-o','/work/probe')
    $build=Native build $label (@('/bin/sh','-ec',$compileAndInspect,'p05-remedy-gcc')+$arguments) 120 134217728
    if($build.exit_code -ne 0){foreach($row in $script:remedyRows|Where-Object producer -CEQ $producer){$row.dependency='BUILD_FAILED'};return}
    CheckBuildHeaders $label $parserRoot
    $binary=RecoveredExecutable $label
    foreach($row in $script:remedyRows|Where-Object producer -CEQ $producer){
        $case=$row.case;$identity=FileIdentity (Join-Path $root ('cases/'+$case.id))
        AssertRemedyObject $identity.bytes $identity.sha256 @{bytes=$case.input_bytes;sha256=$case.input_sha256}
        $arguments=@('/bin/sh','-ec',$execCheck,'p05-remedy-exec',$binary.sha256,('/inputs/results/'+$label+'/probe'),('/inputs/cases/'+$case.id))
        if($case.edit){$arguments+=@([string]$case.edit.start_byte,[string]$case.edit.old_end_byte)}
        $command=Native execution $row.command_label $arguments 10 8388608
        $row.state=$command.termination;$row.dependency='BUILT_EXE_MODE_HASH_ELF_RECHECKED';$row.exit_code=$command.exit_code;$row.raw_stdout='raw/'+$row.command_label+'.stdout'
        $row['binary']=$binary;$row['structure_assessment']='REVIEW_REQUIRED_ORIGINAL_EXPECTATIONS_UNCHANGED'
    }
}
function InvokeRemedyGeneration([string]$producer,[string]$grammarPath,[string]$sourceRoot,[switch]$Sql,[switch]$Postgres){
    $label='generate-'+$producer
    $localGrammar=Join-Path $root $grammarPath
    $files=if($Postgres){@(FileIdentity $localGrammar)}elseif($Sql){CheckSqlJsInputs $localGrammar $sourceRoot}else{CheckJsInputs $localGrammar $sourceRoot}
    Record ($label+'-inputs') @{files=$files;before_generation=$true;literal_dependencies_verified=$true;arbitrary_javascript_dependency_proof=$false;options=$(if($Postgres){'--disable-optimizations'}else{'--js-runtime node'});toolchain_image=$toolchain.image}
    $arguments=@('/inputs/acquisition/tools/tree-sitter','generate','--abi','15','--output','/work/generated')
    if($Postgres){$arguments+='--disable-optimizations'}else{$arguments+=@('--js-runtime','node')}
    $arguments+=('/inputs/'+$grammarPath)
    $result=Native generation $label $arguments 300 536870912 -MemoryEvents:$Postgres
    if($result.exit_code -ne 0){foreach($row in $script:remedyRows|Where-Object producer -CEQ $producer){$row.dependency='GENERATION_FAILED'};return $null}
    return 'results/'+$label+'/generated'
}
function InvokeRemedyProducers($subjects){
    if($RemedyStage -ceq 'patch-r1'){
        foreach($routeName in @('csharp','typescript','tsx','swift')){
            $route=@($inputs.selected_routes|Where-Object route -CEQ $routeName)[0]
            $key=$route.repository.Replace('/','--')+'--'+$route.commit
            $originalRoot='acquisition/sources/'+$key
            $patch=@($subjects.patches.patches|Where-Object repository -CEQ $route.repository)[0]
            $candidateRoot='candidates/'+$patch.id
            $selector=if($route.subdirectory -ceq '.') {''}else{'/'+$route.subdirectory}
            $grammarName=ReadGrammarName (Join-Path $root ($originalRoot+$selector+'/src/grammar.json'))
            $producer=$routeName+'-original'
            $parser=if($routeName -ceq 'swift'){InvokeRemedyGeneration $producer ($originalRoot+$selector+'/grammar.js') (Join-Path $root $originalRoot)}else{$originalRoot+$selector+'/src'}
            if($parser){
                $scanner=if(Test-Path (Join-Path $root ($originalRoot+$selector+'/src/scanner.c'))){$originalRoot+$selector+'/src/scanner.c'}else{''}
                InvokeRemedyBuildAndCases $producer $grammarName ('/inputs/'+$parser) (Join-Path $root $parser) $(if($scanner){'/inputs/'+$scanner}else{''}) $scanner
            }
            $producer=$routeName+'-candidate'
            $parser=InvokeRemedyGeneration $producer ($candidateRoot+$selector+'/grammar.js') (Join-Path $root $candidateRoot)
            if($parser){
                $scanner=if(Test-Path (Join-Path $root ($candidateRoot+$selector+'/src/scanner.c'))){$candidateRoot+$selector+'/src/scanner.c'}else{''}
                InvokeRemedyBuildAndCases $producer $grammarName ('/inputs/'+$parser) (Join-Path $root $parser) $(if($scanner){'/inputs/'+$scanner}else{''}) $scanner
            }
        }
    }else{
        $sqlRoot='candidate-evaluation/derek-sql-97614d0'
        $license=[IO.File]::ReadAllText((Join-Path $root ($sqlRoot+'/LICENSE')))
        if($license -notmatch 'MIT License' -or $license -notmatch 'Permission is hereby granted'){throw 'SQL candidate license mismatch'}
        Record 'sql-license' @{identity=(FileIdentity (Join-Path $root ($sqlRoot+'/LICENSE')));license='MIT';adoption_authorized=$false}
        $parser=InvokeRemedyGeneration 'derek-sql-candidate' ($sqlRoot+'/grammar.js') (Join-Path $root $sqlRoot) -Sql
        if($parser){$grammarName=ReadGrammarName (Join-Path $root ($parser+'/grammar.json'));InvokeRemedyBuildAndCases 'derek-sql-candidate' $grammarName ('/inputs/'+$parser) (Join-Path $root $parser) ('/inputs/'+$sqlRoot+'/src/scanner.c') ($sqlRoot+'/src/scanner.c')}
        $pgRoot='acquisition/sources/gmr--tree-sitter-postgres--59d0d8cd7506d68de1229fb4bbce838c83b60c8a/postgres'
        $pgParser='materialized-lfs/postgres/src'
        foreach($name in @('parser.h','alloc.h','array.h')){
            $original=Join-Path $root ($pgRoot+'/src/tree_sitter/'+$name)
            if(-not (Test-Path -LiteralPath $original)){continue}
            $target=Join-Path $root ($pgParser+'/tree_sitter/'+$name);[void][IO.Directory]::CreateDirectory([IO.Path]::GetDirectoryName($target));[IO.File]::Copy($original,$target)
            $originalId=FileIdentity $original;$identity=FileIdentity $target
            if($originalId.sha256 -cne $identity.sha256 -or $script:verifiedSource[$originalId.path] -cne $identity.sha256){throw 'LFS companion header changed'};$script:verifiedSource[$identity.path]=$identity.sha256
        }
        $grammarName=ReadGrammarName (Join-Path $root ($pgRoot+'/src/grammar.json'))
        $scanner=$pgRoot+'/src/scanner.c';if(-not (Test-Path (Join-Path $root $scanner))){$scanner=''}
        InvokeRemedyBuildAndCases 'postgresql-lfs-baseline' $grammarName ('/inputs/'+$pgParser) (Join-Path $root $pgParser) $(if($scanner){'/inputs/'+$scanner}else{''}) $scanner
        $parser=InvokeRemedyGeneration 'postgresql-noopt-regenerated' ($pgRoot+'/src/grammar.json') (Join-Path $root $pgRoot) -Postgres
        if($parser){InvokeRemedyBuildAndCases 'postgresql-noopt-regenerated' $grammarName ('/inputs/'+$parser) (Join-Path $root $parser) $(if($scanner){'/inputs/'+$scanner}else{''}) $scanner}
    }
}
