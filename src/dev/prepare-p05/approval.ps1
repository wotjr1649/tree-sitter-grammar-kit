param([Parameter(Mandatory)][string]$Profile,[string]$Subject)
$ErrorActionPreference='Stop'
# This binds an approval subject; the caller must separately hold actual user authority.
if($Profile -ceq 'pinned-tsql-r1'){
    if($Subject -cne '9e07792474be6b96406cba915c30c90696a42299ffa9dd8ac60324b3b7a69367'){throw 'Separate pinned TSQL approval subject mismatch'}
} elseif($Profile -cne 'archive-r1' -or $Subject){throw 'Acquisition profile/approval mismatch'}
