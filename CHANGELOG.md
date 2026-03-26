# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/).

## [0.3.0] UNRELEASED

- `vars` with no arguments triggers a first-run setup wizard when no store exists: explains store location, prompts for passphrase, creates the store, starts the agent, and prints next steps
- `VARS_AGENT_TTL` environment variable sets the default agent lifetime (e.g. `export VARS_AGENT_TTL=4h` in your shell profile); falls back to 8 hours if unset
- `vars resolve --origins` appends an inline `# vars`, `# stdin`, or `# KEY  not set` comment to each output line — eval-safe across all output formats, useful for auditing which source each value came from

## [0.2.0]

### Added
- `vars resolve` merges stdin dotenv as a fallback source — store values take priority for manifest keys; dotenv acts as fallback for keys not yet in the store; non-manifest keys pass through unchanged
- Agent is now the exclusive write gateway — all writes (`set`, `rm`, `mv`, `import`, `passwd`) go through the agent and are persisted to disk immediately

### Changed
- Project renamed from `secrets` to `vars` (binary name, env vars `VARS_STORE_DIR` / `VARS_AGENT_SOCK`, store path `~/.local/share/vars/`, manifest files `.vars.yaml` / `.vars.local.yaml`)
- `vars init` removed — the first command that needs the store creates it transparently with a passphrase prompt
- `--overwrite` flag renamed to `--force` on `set` and `import`, consistent with `rm`
- `vars passwd` now prompts for the current passphrase first, then the new passphrase (previously prompted new passphrase first)
- `vars history <key>` now errors if the key does not exist, instead of printing nothing
- Error messages standardised: lowercase, no trailing period
- Batch Set and Delete RPCs — `import` and multi-key `rm` run a single scrypt encryption call regardless of how many keys are affected, significantly reducing write latency

---

## [0.1.0]

### Added
- Encrypted secret store using age/scrypt (`vars init`, `vars set`, `vars get`, `vars ls`, `vars rm`)
- Passphrase management (`vars passwd`) with empty passphrase support
- Per-project manifests (`.vars.yaml`) with export to posix, fish, and dotenv formats
- Per-developer remapping via `.vars-map.yaml`
- `--partial` flag for resolve: skip missing keys instead of erroring
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
