If your app is used on windows, change this icon. You should also set the app name in the manifest.xml file.
I used this website to make a 64x64 icon: https://icoconvert.com
This icon belongs to the application (file), not the tray icon.

GoReleaser embeds icon, manifest, and VERSIONINFO via goversioninfo
(`rsrc_windows_amd64.syso`). Explorer File version is
`Major.Minor.Patch.REVISION` (same fourth number as Darwin CFBundleVersion).
Product version is the release version string (nightly includes `-REVISION`).
