; The Windows installer.
;
; Per-user rather than for the whole machine: it asks for no administrator
; password, it installs into the account that runs it, and the assistant it
; installs keeps its brain in that account's own folder anyway. An installer
; that demands elevation to put one file in Program Files is asking for more
; than it needs.
;
; Nothing here is signed. Windows will say so — SmartScreen shows "unknown
; publisher" and takes two clicks to pass — and that is the honest state of
; things rather than something to hide: a signing certificate costs money
; every year and this program does not have one. The documentation says the
; same.

#ifndef Version
  #define Version "0.0.0"
#endif

#ifndef SourceDir
  #define SourceDir "..\..\build\windows\PN-Scripts-Assistant"
#endif

[Setup]
AppId={{7F3B6A1E-6C2C-4E0B-9E5F-2B7A6F0C9D41}
AppName=PN Scripts Assistant
AppVersion={#Version}
AppVerName=PN Scripts Assistant {#Version}
AppPublisher=Petar Nikolov
AppPublisherURL=https://github.com/pnscripts/pn-scripts-assistant
AppSupportURL=https://github.com/pnscripts/pn-scripts-assistant/issues
DefaultDirName={autopf}\PN Scripts Assistant
DefaultGroupName=PN Scripts Assistant
DisableProgramGroupPage=yes
PrivilegesRequired=lowest
PrivilegesRequiredOverridesAllowed=dialog
OutputBaseFilename=pn-scripts-assistant-{#Version}-windows-setup
Compression=lzma2
SolidCompression=yes
WizardStyle=modern
; x64 rather than x64compatible, which reads better and is newer than the
; Inno Setup on some machines that will compile this. 6.3 renamed it and kept
; x64 working; 6.2 does not know the new name at all and refuses to compile.
; The older spelling builds everywhere, and means the same thing here.
ArchitecturesAllowed=x64
ArchitecturesInstallIn64BitMode=x64
UninstallDisplayName=PN Scripts Assistant
UninstallDisplayIcon={app}\pn-scripts-assistant.exe

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"

[Tasks]
Name: "desktopicon"; Description: "Put a shortcut on the desktop"; GroupDescription: "Shortcuts:"

[Files]
Source: "{#SourceDir}\pn-scripts-assistant.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#SourceDir}\README.txt"; DestDir: "{app}"; Flags: ignoreversion skipifsourcedoesntexist

[Icons]
Name: "{group}\PN Scripts Assistant"; Filename: "{app}\pn-scripts-assistant.exe"
Name: "{group}\Set up again"; Filename: "{app}\pn-scripts-assistant.exe"; Parameters: "setup"
Name: "{autodesktop}\PN Scripts Assistant"; Filename: "{app}\pn-scripts-assistant.exe"; Tasks: desktopicon

[Run]
Filename: "{app}\pn-scripts-assistant.exe"; Description: "Open it now"; Flags: nowait postinstall skipifsilent

; Nothing is removed from the user's own folders on uninstall. The brain — the
; conversations, everything learned — lives in their profile, and an
; uninstaller that deletes years of somebody's memory because they removed a
; program is an uninstaller nobody asked for. The documentation says where it
; is and how to remove it deliberately.
