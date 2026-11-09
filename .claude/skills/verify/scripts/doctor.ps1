# Read-only health check for a running saundo instance. Changes nothing.
# Usage from the repo root (Git Bash):
#   powershell -NoProfile -ExecutionPolicy RemoteSigned -File .claude/skills/verify/scripts/doctor.ps1
# Exit code 0 = instance is up and worth driving; 1 = nothing to drive.
param(
    # Override the data dir (default: %APPDATA%\saundo). Use when the
    # instance under test was launched with a redirected APPDATA.
    [string]$DataDir = ""
)

$ErrorActionPreference = "SilentlyContinue"
$fail = $false

function Line($tag, $msg) { Write-Output ("[{0}] {1}" -f $tag, $msg) }

# --- 1. Process -------------------------------------------------------------
$procs = @(Get-Process -Name saundo | Where-Object { $_.Path })
if ($procs.Count -eq 0) {
    Line "FAIL" "no saundo.exe process is running"
    exit 1
}
$p = $procs[0]
if ($procs.Count -gt 1) {
    Line "WARN" ("{0} saundo.exe processes found; single-instance lock should prevent this" -f $procs.Count)
}
Line "OK" ("process up: pid {0}, started {1}" -f $p.Id, $p.StartTime.ToString("yyyy-MM-dd HH:mm:ss"))
Line "OK" ("binary: {0}" -f $p.Path)
$ver = (Get-Item $p.Path).VersionInfo.ProductVersion
Line "OK" ("version: {0}" -f $ver)
if ($p.Path -notlike "*\bin\saundo.exe" -and $p.Path -notlike "*\saundo.exe") {
    Line "WARN" "unexpected binary path"
}

# --- 2. Data dir ------------------------------------------------------------
if (-not $DataDir) {
    $DataDir = Join-Path $env:APPDATA "saundo"
}
Line "OK" ("data dir: {0}" -f $DataDir)
foreach ($name in @("settings.json", "receivers.json", "credentials.dat")) {
    $f = Get-Item (Join-Path $DataDir $name)
    if ($f) {
        Line "OK" ("  {0} ({1} bytes, {2})" -f $name, $f.Length, $f.LastWriteTime.ToString("yyyy-MM-dd HH:mm"))
    } else {
        Line "INFO" ("  {0} absent (normal for a fresh dir; receivers.json appears with the first registry write)" -f $name)
    }
}
# Never print the contents of credentials.dat: DPAPI-protected secrets.

# --- 3. Log -----------------------------------------------------------------
$log = Join-Path $DataDir "saundo.log"
if (Test-Path $log) {
    Line "OK" ("log: {0}" -f $log)
    $tail = Get-Content $log -Tail 12
    foreach ($l in $tail) { Line "LOG" $l }
    $fatal = Get-Content $log -Tail 60 | Select-String -Pattern "fatal:"
    if ($fatal) { Line "FAIL" "log contains a fatal: line" ; $fail = $true }
} else {
    Line "WARN" ("{0} missing on disk. The app may hold a deleted handle: the live log is the 诊断日志 row on the settings 关于 page (its 复制 button puts it on the clipboard)." -f $log)
}

# --- 4. Windows owned by the pid --------------------------------------------
Add-Type @"
using System;
using System.Text;
using System.Collections.Generic;
using System.Runtime.InteropServices;
public class WinEnum {
    [DllImport("user32.dll")] static extern bool EnumWindows(EnumWindowsProc cb, IntPtr lp);
    [DllImport("user32.dll")] static extern uint GetWindowThreadProcessId(IntPtr h, out uint pid);
    [DllImport("user32.dll")] static extern int GetClassName(IntPtr h, StringBuilder sb, int max);
    [DllImport("user32.dll")] static extern int GetWindowText(IntPtr h, StringBuilder sb, int max);
    [DllImport("user32.dll")] static extern bool IsWindowVisible(IntPtr h);
    delegate bool EnumWindowsProc(IntPtr h, IntPtr lp);
    public static List<string> Of(uint pid) {
        var found = new List<string>();
        EnumWindows((h, lp) => {
            uint wpid; GetWindowThreadProcessId(h, out wpid);
            if (wpid != pid) return true;
            var cls = new StringBuilder(256); GetClassName(h, cls, 256);
            var txt = new StringBuilder(256); GetWindowText(h, txt, 256);
            found.Add(string.Format("{0}|{1}|{2}", cls, txt, IsWindowVisible(h) ? "visible" : "hidden"));
            return true;
        }, IntPtr.Zero);
        return found;
    }
}
"@
$wins = [WinEnum]::Of([uint32]$p.Id)
$flyout = $wins | Where-Object { $_ -like "MyGoFlyoutShadow|*" }
$settings = $wins | Where-Object { $_ -like "MyGoWindow|*saundo*" }
if ($flyout) {
    Line "OK" ("flyout window: {0}" -f ($flyout -join "; "))
} else {
    Line "OK" "flyout window absent from the window list (panel hidden, app in tray — normal after Escape or --tray)"
}
if ($settings) {
    Line "OK" ("settings window: {0}" -f ($settings -join "; "))
} else {
    Line "INFO" "settings window not open"
}

# --- Verdict ----------------------------------------------------------------
if ($fail) { Line "FAIL" "doctor found problems; read the FAIL lines above"; exit 1 }
Line "OK" "doctor passed: instance is up and worth driving"
exit 0
