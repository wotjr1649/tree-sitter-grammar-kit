param([Parameter(Mandatory)][string]$Profile,[string]$Subject,[switch]$Execution,[string]$ExecutionSubject)
$ErrorActionPreference='Stop'
# This binds an approval subject; the caller must separately hold actual user authority.
if($Profile -ceq 'pinned-tsql-r1'){
    if($Subject -cne '9e07792474be6b96406cba915c30c90696a42299ffa9dd8ac60324b3b7a69367'){throw 'Separate pinned TSQL approval subject mismatch'}
} elseif($Profile -cne 'archive-r1' -or $Subject){throw 'Acquisition profile/approval mismatch'}
if($Execution -and $ExecutionSubject -cne '1cdf1711088ebaba3347ce617ddfc733b0e4323a401a6c9efe50c37cf1deb42e'){throw 'Separate tmpfs capture approval subject mismatch'}
