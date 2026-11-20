<#
  desk-window-check.ps1: the assistant's own window takes its key, proved in
  the real window on Windows (ci.yml, "The Windows window takes its key").

    powershell -NoProfile -File scripts/desk-window-check.ps1 -ProcessId <id> -Address 127.0.0.1:8895

  The program opens its window at a one-time address (server/desk.go); the
  page there swaps the code for the window's key, keeps it for the tab and
  goes on to the first page; whatever the page posts afterwards carries the
  key (desk.js). Everything else on this computer is refused. Node tests
  prove the scripts and Go tests the guard; only a real WebView2 proves that
  its window does this.

  Done the way assistive technology does it, through UI Automation, which
  the web view answers for its page: no debugging port, nothing changed in
  the program. First, as a program on this computer: the switch "which
  programs are installed", posted without the key, is refused for the key.
  Then, as a person: the next arrow is pressed until the Programs view shows
  the switch, the switch is ticked, and the assistant must keep it.

  Exit 0 and the line "desk window check OK" when all of it held.
#>
param(
    [Parameter(Mandatory = $true)][int]$ProcessId,
    [Parameter(Mandatory = $true)][string]$Address
)

$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName UIAutomationClient, UIAutomationTypes

$base = "http://$Address"

function Fail([string]$why) {
    Write-Output "desk window check FAILED: $why"
    exit 1
}

function Switch-State {
    $state = Invoke-RestMethod -TimeoutSec 10 "$base/api/outside"
    if ($state.data) { $state = $state.data }
    $row = @($state.subjects | Where-Object { $_.id -eq 'programs' })[0]
    if ($null -eq $row) { return $null }
    return [bool]$row.allowed
}

$A = [System.Windows.Automation.AutomationElement]
$Tree = [System.Windows.Automation.TreeScope]

function Find-ById($root, [string]$id) {
    $c = New-Object System.Windows.Automation.PropertyCondition($A::AutomationIdProperty, $id)
    return $root.FindFirst($Tree::Descendants, $c)
}

# 1. A program on this computer: refused for the key, and the switch stays off.
if ((Switch-State) -ne $false) { Fail 'the switch "which programs are installed" should start off' }

$status = 0
$said = ''
try {
    Invoke-WebRequest -UseBasicParsing -TimeoutSec 10 -Method Post -ContentType 'application/json' `
        -Body '{"subject":"programs","allowed":true}' "$base/api/outside/allow" | Out-Null
    $status = 200
} catch {
    if ($_.Exception.Response) { $status = [int]$_.Exception.Response.StatusCode }
    $said = "$($_.ErrorDetails.Message)"
}

# Refused for the key, not for something else on the way (an Origin the
# guard did not like would be a 403 too, and would prove nothing here).
if ($status -ne 403 -or $said -notmatch '"desk"\s*:\s*"not the window"' -or (Switch-State) -ne $false) {
    Fail "a program without the key was not refused for the key (answer $status, $said)"
}
Write-Output 'a program on this computer without the key was refused for not being the window (403) and the switch stayed off'

# 2. The window, found by its process.
$window = $null
for ($i = 0; $i -lt 60 -and $null -eq $window; $i++) {
    $c = New-Object System.Windows.Automation.PropertyCondition($A::ProcessIdProperty, $ProcessId)
    $window = $A::RootElement.FindFirst($Tree::Children, $c)
    if ($null -eq $window) { Start-Sleep 1 }
}
if ($null -eq $window) { Fail 'the program has no window' }
Write-Output "window: $($window.Current.Name)"

# 3. A person goes to Programs with the next arrow, as the page offers it.
$box = $null
$next = $null
for ($i = 0; $i -lt 90 -and $null -eq $box; $i++) {
    $box = Find-ById $window 'outside-programs'
    if ($null -ne $box) { break }

    if ($null -eq $next) { $next = Find-ById $window 'stage-next' }
    if ($null -ne $next -and ($i % 2) -eq 0) {
        try { $next.GetCurrentPattern([System.Windows.Automation.InvokePattern]::Pattern).Invoke() } catch { $next = $null }
    }
    Start-Sleep -Milliseconds 700
}
if ($null -eq $box) { Fail 'the switch did not appear: the Programs view was not reached' }

$toggle = $box.GetCurrentPattern([System.Windows.Automation.TogglePattern]::Pattern)
if ($toggle.Current.ToggleState -ne [System.Windows.Automation.ToggleState]::Off) { Fail 'the switch was already ticked' }

# 4. The person ticks it; the assistant keeps it.
$toggle.Toggle()

$kept = $false
for ($i = 0; $i -lt 60 -and -not $kept; $i++) {
    Start-Sleep -Milliseconds 500
    $kept = (Switch-State) -eq $true
}
if (-not $kept) {
    $note = Find-ById $window 'outside-note'
    $text = if ($note) { $note.Current.Name } else { '' }
    Fail "the switch ticked in the window was not kept; the page says: $text"
}

Write-Output 'ticked in the window: the assistant keeps it'
Write-Output 'desk window check OK: the window took its key, a program without it was refused'
exit 0
