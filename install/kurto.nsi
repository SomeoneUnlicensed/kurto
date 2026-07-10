!define PRODUCT_NAME "Kurto"
!define PRODUCT_VERSION "0.1.0"
!define PRODUCT_PUBLISHER "Vladimir Letunovskiy"

Name "${PRODUCT_NAME} ${PRODUCT_VERSION}"
OutFile "kurto-setup-${PRODUCT_VERSION}.exe"
InstallDir "$PROGRAMFILES64\Kurto"
RequestExecutionLevel admin

Section "Install"
  SetOutPath "$INSTDIR"
  File "kurto-windows-amd64.exe"
  Rename "$INSTDIR\kurto-windows-amd64.exe" "$INSTDIR\kurto.exe"

  WriteUninstaller "$INSTDIR\uninstall.exe"

  CreateDirectory "$SMPROGRAMS\Kurto"
  CreateShortCut "$SMPROGRAMS\Kurto\Kurto Shell.lnk" "$INSTDIR\kurto.exe" ""
  CreateShortCut "$SMPROGRAMS\Kurto\Uninstall.lnk" "$INSTDIR\uninstall.exe"

  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\Kurto" "DisplayName" "Kurto ${PRODUCT_VERSION}"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\Kurto" "UninstallString" "$INSTDIR\uninstall.exe"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\Kurto" "DisplayVersion" "${PRODUCT_VERSION}"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\Kurto" "Publisher" "${PRODUCT_PUBLISHER}"

  ExecWait 'setx PATH "$INSTDIR;%PATH%" /M'
SectionEnd

Section "Uninstall"
  Delete "$INSTDIR\kurto.exe"
  Delete "$INSTDIR\uninstall.exe"
  RMDir "$INSTDIR"

  Delete "$SMPROGRAMS\Kurto\Kurto Shell.lnk"
  Delete "$SMPROGRAMS\Kurto\Uninstall.lnk"
  RMDir "$SMPROGRAMS\Kurto"

  DeleteRegKey HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\Kurto"
SectionEnd
