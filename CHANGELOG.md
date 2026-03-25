# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/).

## [0.1.0] - Unreleased

### Added
- Encrypted secret store using age/scrypt (`vars init`, `vars set`, `vars get`, `vars ls`, `vars rm`)
- Passphrase management (`vars passwd`) with empty passphrase support
- Per-project manifests (`.vars.yaml`) with export to posix, fish, and dotenv formats
- Per-developer remapping via `.vars-map.yaml`
- `--partial` flag for export: emit empty values for missing keys instead of erroring
- Background agent (`vars agent`) holding decrypted store in memory with configurable TTL
- Agent is read-only: serves get/list over Unix domain socket
- Trial-decrypt for empty passphrases (no marker files, like OpenSSH)
- Pluggable `crypto.Backend` interface for future Yubikey/SSH agent support
- Atomic writes (temp file + rename) for crash safety
- Memory zeroing of decrypted secrets on close
- Permission checking with actionable fix commands
- XDG-compliant store location (`~/.local/share/vars/`)
- `VARS_STORE_DIR` environment variable override
- GitHub Actions CI (vet, test, cross-compile) and release workflows
- goreleaser configuration for 5-target builds
- Comprehensive test suite: 70+ unit tests, 22 integration tests, smoke test
