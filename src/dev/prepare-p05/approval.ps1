param([Parameter(Mandatory)][string]$Profile,[string]$Subject,[switch]$Execution,[string]$ExecutionSubject,[string]$ImageProfile,[string]$ImageSubject,[string]$RemedyStage,[string]$RemedySubject)
$ErrorActionPreference='Stop'
# This binds an approval subject; the caller must separately hold actual user authority.
if($Profile -ceq 'pinned-tsql-r1'){
    if($Subject -cne '9e07792474be6b96406cba915c30c90696a42299ffa9dd8ac60324b3b7a69367'){throw 'Separate pinned TSQL approval subject mismatch'}
} elseif($Profile -cne 'archive-r1' -or $Subject){throw 'Acquisition profile/approval mismatch'}
if($Execution -and $ExecutionSubject -cne '1cdf1711088ebaba3347ce617ddfc733b0e4323a401a6c9efe50c37cf1deb42e'){throw 'Separate tmpfs capture approval subject mismatch'}
if($RemedyStage){
    if($RemedyStage -cnotin @('patch-r1','sql-pg-r1') -or $RemedySubject -cne '42396d74938e6938d38aa9adc1ea84fbe05a708fa220ca09074dc1bb56d671f4' -or $Profile -cne 'pinned-tsql-r1' -or $ImageProfile -cne 'trixie-r1'){throw 'Separate exact remedy approval subject/profile mismatch'}
}elseif($RemedySubject){throw 'Remedy subject without stage'}
if($ImageProfile -ceq 'trixie-r1'){
    if($ImageSubject -cne '5dc3d89579acd801130549ba35c989055b8e548d4ba73e51cc365760d9c4ac09'){throw 'Separate GLIBC image approval subject mismatch'}
    return @{profile=$ImageProfile;image='node@sha256:98ad2493de85738f55c11fe22e8586caf1fd917b7a8075c57ab9c55116e06492';compressed_bytes=440298459L;approval_subject=$ImageSubject}
} elseif($ImageProfile -ceq 'bookworm-r1'){
    if($ImageSubject){throw 'Unexpected legacy image subject'}
    return @{profile=$ImageProfile;image='node@sha256:5a750d3be5e5c80275f8c9a5367c3aed99c2875656590c8d0701c7ee687f5f0a';compressed_bytes=409764189L;approval_subject='original B image'}
} elseif($ImageProfile -or $ImageSubject){throw 'Image profile/approval mismatch'}
