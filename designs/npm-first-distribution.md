# Purpose
Ship `aproc` as an npm-installed CLI where npm is the primary install channel, users do not need Go, and the execution core remains a prebuilt Go binary. Installation must work in Node-based agent environments without creating workspaces, runtime directories, or project scaffolding.

# Approach
Publish one npm package that contains only:
- `bin/aproc.js`: runtime launcher
- `scripts/install.js`: install-time downloader/verifier
- `bin/native/`: destination for the downloaded Go binary only

Install-time responsibilities:
- Read the npm package version from `package.json`.
- Detect `process.platform` and `process.arch`.
- Map that pair to a supported Go target.
- Build release URLs for the exact same version as the npm package.
- Download the platform archive and checksum file.
- Verify the archive checksum before extraction.
- Extract `aproc` or `aproc.exe` into `bin/native/`.
- Set executable permission on Unix.
- Fail with a non-zero install error on unsupported platform, download failure, extract failure, or checksum failure.
- Remove partial files on any failure.
- Create no runtime state and no workspace scaffolding.

Runtime responsibilities:
- Resolve `bin/native/aproc` or `bin/native/aproc.exe`.
- Spawn the Go binary with inherited stdio.
- Forward exit code and signals.
- Perform no download, install, update, or migration logic.
- Leave all runtime state creation to the Go binary itself.

# Decisions
- npm package name: `aproc`.
- If `aproc` is unavailable on npm, use scoped name `@agentos/aproc`.
- CLI command name remains `aproc` in either package.
- Install destination inside the npm package: `bin/native/aproc` on Unix and `bin/native/aproc.exe` on Windows.
- Binary download source pattern: `https://github.com/<org>/<repo>/releases/download/v${version}/`.
- Release artifact naming:
  - Unix: `aproc_${version}_${goos}_${goarch}.tar.gz`
  - Windows: `aproc_${version}_${goos}_${goarch}.zip`
- Checksum file naming: `SHA256SUMS` at the same release path.
- Checksum verification: installer downloads `SHA256SUMS`, finds the exact artifact filename entry, computes SHA-256 of the downloaded archive, and aborts if the entry is missing, malformed, or mismatched.
- Platform mapping:
  - `linux` + `x64` -> `linux/amd64` -> `aproc_${version}_linux_amd64.tar.gz`
  - `linux` + `arm64` -> `linux/arm64` -> `aproc_${version}_linux_arm64.tar.gz`
  - `darwin` + `x64` -> `darwin/amd64` -> `aproc_${version}_darwin_amd64.tar.gz`
  - `darwin` + `arm64` -> `darwin/arm64` -> `aproc_${version}_darwin_arm64.tar.gz`
  - `win32` + `x64` -> `windows/amd64` -> `aproc_${version}_windows_amd64.zip`
  - `win32` + `arm64` -> `windows/arm64` -> `aproc_${version}_windows_arm64.zip`
- Unsupported-platform behavior: `postinstall` exits non-zero, prints detected platform and supported mappings, and leaves no binary in `bin/native/`.
- Failed-download behavior: installer retries are out of scope; any network, HTTP, checksum, or extraction failure exits non-zero and removes partial downloads and extracted files.
- Version alignment: npm package version `X.Y.Z` must download release tag `vX.Y.Z`, artifact `aproc_X.Y.Z_*`, and matching `SHA256SUMS`. Publishing npm `X.Y.Z` is blocked unless those release assets already exist.
- Upgrade behavior: users upgrade through npm only (`npm install -g aproc@latest` or equivalent). npm replaces package contents and reruns `postinstall` for the new version. The Go binary does not self-update. A failed upgrade leaves npm reporting failure rather than accepting a mismatched binary.

# Done Condition
- A maintainer can implement `scripts/install.js`, `bin/aproc.js`, and release automation directly from this document.
- npm is the primary install path and no Go toolchain is required on user machines.
- Install-time and runtime responsibilities are explicitly separate and non-overlapping.
- Package naming, download URL pattern, install destination, artifact naming, checksum handling, platform mapping, unsupported-platform behavior, failed-download behavior, version alignment, and upgrade behavior are fully specified.
