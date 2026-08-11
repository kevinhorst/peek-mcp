#ifndef AppVersion
  #define AppVersion "0.0.0"
#endif

[Setup]
AppId={{7E1F7C34-9D3B-4A46-B4F2-2A7C0E6D5A11}
AppName=peek-mcp
AppVersion={#AppVersion}
AppPublisher=Kevin Horst
AppPublisherURL=https://github.com/kevinhorst/peek-mcp
DefaultDirName={localappdata}\Programs\peek-mcp
DisableProgramGroupPage=yes
PrivilegesRequired=lowest
ArchitecturesInstallIn64BitMode=x64compatible and arm64
ChangesEnvironment=yes
OutputDir=..\..\dist
OutputBaseFilename=peek-mcp-setup
SolidCompression=yes

[Tasks]
Name: "claude"; Description: "Configure Claude Code (%USERPROFILE%\.claude.json)"
Name: "codex"; Description: "Configure Codex CLI (%USERPROFILE%\.codex\config.toml)"
Name: "controlserver"; Description: "Enable the control server dashboard (http://127.0.0.1:42442)"
Name: "addtopath"; Description: "Add peek-mcp to PATH"

[Files]
Source: "..\..\dist\peek-mcp-windows-amd64.exe"; DestDir: "{app}"; DestName: "peek-mcp.exe"; Check: not IsArm64
Source: "..\..\dist\peek-mcp-windows-arm64.exe"; DestDir: "{app}"; DestName: "peek-mcp.exe"; Check: IsArm64

[Registry]
Root: HKCU; Subkey: "Environment"; ValueType: expandsz; ValueName: "Path"; ValueData: "{olddata};{app}"; Tasks: addtopath; Check: NeedsAddPath(ExpandConstant('{app}'))

[Run]
Filename: "{app}\peek-mcp.exe"; Parameters: "{code:SetupParams}"; StatusMsg: "Writing agent configuration..."; Flags: runhidden waituntilterminated; Check: WizardIsTaskSelected('claude') or WizardIsTaskSelected('codex')
Filename: "{app}\peek-mcp.exe"; Parameters: "{code:StartParams}"; Description: "Start peek-mcp now (keeps a console window open)"; Flags: nowait postinstall skipifsilent
Filename: "http://127.0.0.1:42442"; Description: "Open the control server dashboard"; Flags: shellexec postinstall skipifsilent; Check: WizardIsTaskSelected('controlserver')

[Code]
function StartParams(Param: string): string;
begin
  Result := 'start';
  if not WizardIsTaskSelected('controlserver') then
    Result := Result + ' --control-port=0';
end;

function SetupParams(Param: string): string;
begin
  Result := 'setup';
  if WizardIsTaskSelected('claude') then
    Result := Result + ' --claude';
  if WizardIsTaskSelected('codex') then
    Result := Result + ' --codex';
  if not WizardIsTaskSelected('controlserver') then
    Result := Result + ' --control-server=false';
end;

function NeedsAddPath(Param: string): boolean;
var
  OrigPath: string;
begin
  if not RegQueryStringValue(HKCU, 'Environment', 'Path', OrigPath) then
  begin
    Result := True;
    exit;
  end;
  Result := Pos(';' + Uppercase(Param) + ';', ';' + Uppercase(OrigPath) + ';') = 0;
end;
