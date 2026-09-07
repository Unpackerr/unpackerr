If your app is used on windows, change this icon. You should also set the app name in the manifest.xml file.
I used this website to make a 64x64 icon: https://icoconvert.com
This icon belongs to the application (file), not the tray icon.

GoReleaser embeds icon, manifest, and VERSIONINFO via goversioninfo
(`rsrc_windows_amd64.syso`). Explorer File version is the `x.y.z` prefix of
the release version plus `REVISION` (same split as Darwin
CFBundleShortVersionString; fourth number matches CFBundleVersion).
Product version is the release version string (nightly includes `-REVISION`).
Do not use GoReleaser `.RawVersion` here: on `--nightly` it stays on the last
tag while `.Version` is already incpatched.
