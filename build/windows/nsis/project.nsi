Unicode true

####
## Please note: Template replacements don't work in this file. They are provided with default defines like
## mentioned underneath.
## If the keyword is not defined, "wails_tools.nsh" will populate them.
## If they are defined here, "wails_tools.nsh" will not touch them. This allows you to use this project.nsi manually
## from outside of Wails for debugging and development of the installer.
## 
## For development first make a wails nsis build to populate the "wails_tools.nsh":
## > wails build --target windows/amd64 --nsis
## Then you can call makensis on this file with specifying the path to your binary:
## For a AMD64 only installer:
## > makensis -DARG_WAILS_AMD64_BINARY=..\..\bin\app.exe
## For a ARM64 only installer:
## > makensis -DARG_WAILS_ARM64_BINARY=..\..\bin\app.exe
## For a installer with both architectures:
## > makensis -DARG_WAILS_AMD64_BINARY=..\..\bin\app-amd64.exe -DARG_WAILS_ARM64_BINARY=..\..\bin\app-arm64.exe
####
## The following information is taken from the wails_tools.nsh file, but they can be overwritten here.
####
## !define INFO_PROJECTNAME    "my-project" # Default "ArkServerLink"
## !define INFO_COMPANYNAME    "My Company" # Default "arkfgf.com"
## !define INFO_PRODUCTNAME    "My Product Name" # Default "ARKFGF 联机助手"
## !define INFO_PRODUCTVERSION "1.0.0"     # Default "0.1.0"
## !define INFO_COPYRIGHT      "(c) Now, My Company" # Default "© 2026, My Company"
###
## !define PRODUCT_EXECUTABLE  "Application.exe"      # Default "${INFO_PROJECTNAME}.exe"
## !define UNINST_KEY_NAME     "UninstKeyInRegistry"  # Default "${INFO_COMPANYNAME}${INFO_PRODUCTNAME}"
####
## !define REQUEST_EXECUTION_LEVEL "admin"            # Default "admin"  see also https://nsis.sourceforge.io/Docs/Chapter4.html
## !define WAILS_INSTALL_SCOPE     "user"             # Default "machine" - set to "user" for per-user install ($LOCALAPPDATA) without UAC prompt
####
## Include the wails tools
####
!include "wails_tools.nsh"

# The version information for this two must consist of 4 parts
VIProductVersion "${INFO_PRODUCTVERSION}.0"
VIFileVersion    "${INFO_PRODUCTVERSION}.0"

VIAddVersionKey "CompanyName"     "${INFO_COMPANYNAME}"
VIAddVersionKey "FileDescription" "${INFO_PRODUCTNAME} Installer"
VIAddVersionKey "ProductVersion"  "${INFO_PRODUCTVERSION}"
VIAddVersionKey "FileVersion"     "${INFO_PRODUCTVERSION}"
VIAddVersionKey "LegalCopyright"  "${INFO_COPYRIGHT}"
VIAddVersionKey "ProductName"     "${INFO_PRODUCTNAME}"

# Enable HiDPI support. https://nsis.sourceforge.io/Reference/ManifestDPIAware
ManifestDPIAware true

!include "MUI.nsh"
!include "LogicLib.nsh"

!define MUI_ICON "..\icon.ico"
!define MUI_UNICON "..\icon.ico"
# !define MUI_WELCOMEFINISHPAGE_BITMAP "resources\leftimage.bmp" #Include this to add a bitmap on the left side of the Welcome Page. Must be a size of 164x314
!define MUI_FINISHPAGE_NOAUTOCLOSE # Wait on the INSTFILES page so the user can take a look into the details of the installation steps
!define MUI_ABORTWARNING # This will warn the user if they exit from the installer.

!insertmacro MUI_PAGE_WELCOME # Welcome to the installer page.
# !insertmacro MUI_PAGE_LICENSE "resources\eula.txt" # Adds a EULA page to the installer
!insertmacro MUI_PAGE_DIRECTORY # In which folder install page.
!insertmacro MUI_PAGE_INSTFILES # Installing page.
!insertmacro MUI_PAGE_FINISH # Finished installation page.

!insertmacro MUI_UNPAGE_INSTFILES # Uninstalling page

# 第一个是默认语言；同时插入多种时 NSIS 按系统语言自动选择
!insertmacro MUI_LANGUAGE "SimpChinese"
!insertmacro MUI_LANGUAGE "English"

## The following two statements can be used to sign the installer and the uninstaller. The path to the binaries are provided in %1
#!uninstfinalize 'signtool --file "%1"'
#!finalize 'signtool --file "%1"'

Name "${INFO_PRODUCTNAME}"
OutFile "..\..\..\bin\${INFO_PROJECTNAME}-${ARCH}-installer.exe" # Name of the installer's file.
# 安装目录名不用中文产品名，避免中文路径
!define INSTALL_DIRNAME "ArkServerLink"
!if "${WAILS_INSTALL_SCOPE}" == "user"
    !define DEFAULT_INSTDIR "$LOCALAPPDATA\Programs\${INSTALL_DIRNAME}"
    !define UNINST_ROOT HKCU
!else
    !define DEFAULT_INSTDIR "$PROGRAMFILES64\${INFO_COMPANYNAME}\${INSTALL_DIRNAME}"
    !define UNINST_ROOT HKLM
!endif
InstallDir "${DEFAULT_INSTDIR}"
ShowInstDetails show # This will always show the installation details.

Function .onInit
    !insertmacro wails.checkArchitecture

    # 已经安装过时沿用原来的目录（旧版本默认装在中文名目录下），覆盖安装不会装出两份。
    # 命令行用 /D= 指定了目录（自动更新）时 $INSTDIR 不是默认值，不改。
    ${If} "$INSTDIR" == "${DEFAULT_INSTDIR}"
        SetRegView 64
        ReadRegStr $R0 ${UNINST_ROOT} "${UNINST_KEY}" "UninstallString"
        SetRegView lastused
        # UninstallString 形如 "C:\...\uninstall.exe"（带引号）
        ${If} $R0 != ""
            StrCpy $R0 $R0 "" 1
            StrCpy $R0 $R0 -15
            ${If} ${FileExists} "$R0\uninstall.exe"
                StrCpy $INSTDIR $R0
            ${EndIf}
        ${EndIf}
    ${EndIf}
FunctionEnd

# 文件被运行中的进程占用时以写方式打开会失败，用来判断客户端是否还在运行
!macro IsLocked FILE RESULT
    StrCpy ${RESULT} 0
    ${If} ${FileExists} "${FILE}"
        ClearErrors
        FileOpen $R9 "${FILE}" a
        ${If} ${Errors}
            StrCpy ${RESULT} 1
        ${Else}
            FileClose $R9
        ${EndIf}
    ${EndIf}
!macroend

# 覆盖安装前等客户端和 easytier-core 退出。
# 自动更新（静默）时客户端启动安装包后马上退出，最多等 30 秒；手动安装时等 2 秒后提示玩家先退出。
Function WaitForAppExit
    StrCpy $R1 0
    ${If} ${Silent}
        StrCpy $R3 60
    ${Else}
        StrCpy $R3 4
    ${EndIf}
    loop:
        !insertmacro IsLocked "$INSTDIR\${PRODUCT_EXECUTABLE}" $R0
        ${If} $R0 == 0
            !insertmacro IsLocked "$INSTDIR\easytier\easytier-core.exe" $R0
        ${EndIf}
        ${If} $R0 == 0
            Return
        ${EndIf}
        ${If} $R1 < $R3
            IntOp $R1 $R1 + 1
            Sleep 500
            Goto loop
        ${EndIf}
        ${If} ${Silent}
            Abort
        ${EndIf}
        MessageBox MB_RETRYCANCEL|MB_ICONEXCLAMATION "${INFO_PRODUCTNAME} 正在运行，请先关闭后点「重试」。" IDRETRY retry
        Abort
    retry:
        StrCpy $R1 0
        Goto loop
FunctionEnd

Section
    !insertmacro wails.setShellContext

    !insertmacro wails.webview2runtime

    Call WaitForAppExit

    SetOutPath $INSTDIR
    
    !insertmacro wails.files

    # EasyTier（由 easytier:fetch 下载到 bin/easytier），程序在 $INSTDIR\easytier 下查找
    SetOutPath $INSTDIR\easytier
    File "..\..\..\bin\easytier\easytier-core.exe"
    File "..\..\..\bin\easytier\easytier-cli.exe"
    File "..\..\..\bin\easytier\Packet.dll"
    SetOutPath $INSTDIR

    CreateShortcut "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"
    CreateShortCut "$DESKTOP\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"

    !insertmacro wails.associateFiles
    !insertmacro wails.associateCustomProtocols
    
    !insertmacro wails.writeUninstaller

    # 客户端自动更新时带 /RELAUNCH（见 internal/update），装完重新打开客户端
    ${GetParameters} $R0
    ClearErrors
    ${GetOptions} $R0 "/RELAUNCH" $R1
    ${IfNot} ${Errors}
        Exec '"$INSTDIR\${PRODUCT_EXECUTABLE}"'
    ${EndIf}
SectionEnd

Section "uninstall" 
    !insertmacro wails.setShellContext

    RMDir /r "$AppData\${PRODUCT_EXECUTABLE}" # Remove the WebView2 DataPath

    RMDir /r $INSTDIR

    Delete "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk"
    Delete "$DESKTOP\${INFO_PRODUCTNAME}.lnk"

    !insertmacro wails.unassociateFiles
    !insertmacro wails.unassociateCustomProtocols

    !insertmacro wails.deleteUninstaller
SectionEnd
