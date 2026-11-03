# Capture a saundo window to a PNG via PrintWindow (works on the layered
# flyout, which a plain screen grab misses when overlapped).
# Usage from the repo root:
#   powershell -NoProfile -ExecutionPolicy Bypass -File .claude/skills/verify/scripts/capture-window.ps1 -Out /tmp/flyout.png
#   ... -Class MyGoWindow -TitleLike "saundo"   # the settings window
#   ... -ProcId 12345                           # restrict to one process
# Exit code 0 = PNG written; 1 = no matching window.
param(
    [Parameter(Mandatory)][string]$Out,
    [string]$Class = "MyGoFlyoutShadow",
    [string]$TitleLike = "",
    [int]$ProcId = 0
)

$ErrorActionPreference = "Stop"
Add-Type -AssemblyName System.Drawing
Add-Type @"
using System;
using System.Text;
using System.Collections.Generic;
using System.Runtime.InteropServices;
public class WinCap {
    [StructLayout(LayoutKind.Sequential)]
    public struct RECT { public int L, T, R, B; }
    [DllImport("user32.dll")] static extern bool EnumWindows(EnumWindowsProc cb, IntPtr lp);
    [DllImport("user32.dll")] static extern uint GetWindowThreadProcessId(IntPtr h, out uint pid);
    [DllImport("user32.dll")] static extern int GetClassName(IntPtr h, StringBuilder sb, int max);
    [DllImport("user32.dll")] static extern int GetWindowText(IntPtr h, StringBuilder sb, int max);
    [DllImport("user32.dll")] static extern bool IsWindowVisible(IntPtr h);
    [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h, out RECT r);
    [DllImport("user32.dll")] public static extern bool PrintWindow(IntPtr h, IntPtr dc, uint flags);
    delegate bool EnumWindowsProc(IntPtr h, IntPtr lp);
    public static IntPtr Found = IntPtr.Zero;
    public static string Desc = "";
    public static IntPtr Find(string cls, string titleLike, uint pid) {
        Found = IntPtr.Zero;
        EnumWindows((h, lp) => {
            if (!IsWindowVisible(h)) return true;
            uint wpid; GetWindowThreadProcessId(h, out wpid);
            if (pid != 0 && wpid != pid) return true;
            var c = new StringBuilder(256); GetClassName(h, c, 256);
            if (cls != "" && !c.ToString().StartsWith(cls, StringComparison.OrdinalIgnoreCase)) return true;
            var t = new StringBuilder(256); GetWindowText(h, t, 256);
            if (titleLike != "" && !t.ToString().Contains(titleLike)) return true;
            Found = h; Desc = string.Format("pid {0} class {1} title {2}", wpid, c, t);
            return false;
        }, IntPtr.Zero);
        return Found;
    }
}
"@

$hwnd = [WinCap]::Find($Class, $TitleLike, [uint32]$ProcId)
if ($hwnd -eq [IntPtr]::Zero) {
    Write-Output ("no visible window matches class={0} title~={1} pid={2}" -f $Class, $TitleLike, $ProcId)
    exit 1
}
$r = New-Object WinCap+RECT
[WinCap]::GetWindowRect($hwnd, [ref]$r) | Out-Null
$w = $r.R - $r.L; $h = $r.B - $r.T
if ($w -le 0 -or $h -le 0) { Write-Output "window has zero size"; exit 1 }

$bmp = New-Object System.Drawing.Bitmap($w, $h)
$g = [System.Drawing.Graphics]::FromImage($bmp)
$dc = $g.GetHdc()
# PW_RENDERFULLCONTENT (2) picks up DWM-composited content, which the
# frameless shadowed flyout needs.
[WinCap]::PrintWindow($hwnd, $dc, 2) | Out-Null
$g.ReleaseHdc($dc); $g.Dispose()
$full = $ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath($Out)
$dir = Split-Path $full -Parent
if ($dir -and -not (Test-Path $dir)) { New-Item -ItemType Directory -Path $dir -Force | Out-Null }
$bmp.Save($full, [System.Drawing.Imaging.ImageFormat]::Png)
$bmp.Dispose()
Write-Output ("captured {0} ({1}x{2}px) -> {3}" -f ([WinCap]::Desc), $w, $h, $full)
exit 0
