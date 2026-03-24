# secrets

One encrypted store for all your environment variables, shared across any number of projects.

---

If you work across many repositories — each with its own `.env` file full of private keys, RPC URLs, and API tokens — you've felt the pain: secrets duplicated everywhere, rotations that miss half the repos, files that accidentally get committed or read by an agent.

`secrets` keeps all your secrets in one age-encrypted store and exports them as environment variables on demand, making `.env` files entirely optional. It is **opt-in and non-breaking**: collaborators that don't use it keep operating as before. Projects that do, commit a `.secrets.yaml` listing the variable names they need and developers can resolve them from their own personal store.

It is **local-first and offline friendly** — no server, no cloud dependency, no account. It scales from a solo developer to a team using optional scopes and profiles. The store is a single encrypted file that you can back up, sync, or leave alone.

`secrets` loads env vars on your session. What you do with them is up to you.

---

## Install

```sh
# macOS (Apple Silicon)
curl -L https://github.com/brickpop/secrets/releases/latest/download/secrets_darwin_arm64.tar.gz | tar xz
sudo mv secrets /usr/local/bin/

# Linux / WSL (amd64)
curl -L https://github.com/brickpop/secrets/releases/latest/download/secrets_linux_amd64.tar.gz | tar xz
sudo mv secrets /usr/local/bin/
```

---

## The basics — your personal store

Start by creating an encrypted store. An encryption passphrase can be chosen — press enter for none.

```sh
secrets init
```

Store values:

```sh
secrets set PRIVATE_KEY              # prompts for the value (keeps it out of shell history)
secrets set RPC_URL "https://rpc.example.com"
secrets set ETHERSCAN_API_KEY "abc123"
```

Read them back:

```sh
secrets get RPC_URL                  # prints the value
secrets ls                           # lists all keys (global scope)
```

One place for all your keys, encrypted at rest, accessible from any terminal.

---

## Using in a project

Add `.secrets.yaml` to your project declaring the env var names it needs. Commit it :

```yaml
# .secrets.yaml
keys:
  - RPC_URL
  - PRIVATE_KEY
  - ETHERSCAN_API_KEY
```

Then resolve them into your shell, on demand:

```sh
eval "$(secrets resolve)"          # bash/zsh
echo $ETHERSCAN_API_KEY            # Loaded as env vars
```

`resolve` reads the manifest, looks up each key in your store, and prints shell-ready `export` statements. Nothing is written to disk.

Each developer uses their own store. The committed manifest is the shared contract; the secrets are personal.

---

## Scoped keys

As your store grows, you may have multiple variants of the same key — a `prod` private key, a `staging` key, etc. A naming convention keeps them organised: prefix keys with a scope, separated by `/`.

```sh
secrets set prod/PRIVATE_KEY "0xPRODKEY"
secrets set hoodi/PRIVATE_KEY "0xTESTKEY"
secrets set arbitrum/dev/PRIVATE_KEY "0xARBKEY"   # nested scope
secrets set ETHERSCAN_API_KEY "abc123"            # shared, no scope needed
```

List keys by scope:

```sh
secrets ls                   # top level keys only
secrets ls prod              # keys under prod/, prefix stripped from output
secrets ls -a                # keys from all scopes
secrets scope ls             # list all scope prefixes present in the store
```

---

## Profiles — resolving the right scope

If you have scoped variables, you will need `resolve` to know which scope to use for each run. That's what `profiles` are for: a list of `env var → store key` mappings, declared in `.secrets.yaml`.

```yaml
# .secrets.yaml
keys:
  - PRIVATE_KEY
  - RPC_URL
  - ETHERSCAN_API_KEY

profiles:
  default:
    PRIVATE_KEY: dev/PRIVATE_KEY
    RPC_URL: sepolia/RPC_URL
  mainnet:
    PRIVATE_KEY: prod/PRIVATE_KEY
    RPC_URL: mainnet/RPC_URL
```

```sh
secrets resolve              # uses the "default" profile automatically
secrets resolve -p mainnet   # mappings from the mainnet profile
```

`PRIVATE_KEY` will resolve to either `sepolia/PRIVATE_KEY` (by default) or to `prod/PRIVATE_KEY` when `-p mainnet` is passed. 

Profiles are opt-in. Any keys (`ETHERSCAN_API_KEY`) not listed in a profile continue to work as usual.

### Scope fallback in resolve

When resolving a key, `resolve` searches from most specific to least — stripping one scope level at a time until a match is found:

`hoodi/dev/RPC_URL`  →  `hoodi/RPC_URL`  →  `RPC_URL`  →  Not found.

This means that your profile can reference a specific scoped key even if you've only stored the base key. Teams can share a common `RPC_URL` while individuals or environments override it at any specificity level, without changing the manifest.

### Team-wide key aliases

Use `mappings:` to rename store keys for everyone on the team, regardless of the profile:

```yaml
mappings:
  ETHERSCAN_API_KEY: ETHERSCAN_API_KEY_v2    # applies to all profiles
```

---

## Personal overrides

Profiles and mappings in `.secrets.yaml` are committed — they are the team's shared convention. But you may have personal variations: a wallet key, a different URL, a local backup.

Add `.secrets.local.yaml` (git-ignored, never commit it) alongside `.secrets.yaml`:

```yaml
# .secrets.local.yaml
mappings:
  PRIVATE_KEY: prod/PRIVATE_KEY_alice_hw    # override for all profiles

profiles:
  mainnet:
    RPC_URL: prod/RPC_URL_quicknode         # only override this one key in mainnet
```

Local overrides take priority over `.secrets.yaml`, per key. Everyone has the same manifest; only personal overrides differ.

---

## More use cases

### Scripts and task runners

Using `just` or `make`:

```sh
# justfile
deploy:
    #!/usr/bin/env bash
    eval "$(secrets resolve -p mainnet)"
    forge script script/Deploy.s.sol --broadcast

test:
    #!/usr/bin/env bash
    eval "$(secrets resolve)"
    forge test
```

### Docker containers

Pass secrets to a container without writing anything to disk, using bash process substitution:

```sh
docker run --env-file <(secrets resolve --format dotenv) my-image
```

`--env-file` reads from the file descriptor provided by `<(...)` — the output of `secrets resolve` never touches the filesystem.

### Migrating from `.env` files

```sh
secrets import .env                    # import all keys
secrets import my-project/dev .env     # import with a scope prefix → my-project/dev/KEY
```

Conflicts are handled interactively (skip, overwrite). Use `--skip` or `--overwrite` for non-interactive imports.

### Integrating with external vaults

`secrets` resolves to plain env vars, so it composes with anything. If your team uses 1Password, you can sync keys from it using the `op` CLI:

```sh
# justfile — sync secrets from 1Password (skip if values haven't changed)
sync-from-op:
    #!/usr/bin/env bash
    secrets set --skip dev/RPC_URL        "$(op read 'op://dev/rpc/url')"
    secrets set --skip dev/PRIVATE_KEY    "$(op read 'op://dev/wallet/private-key')"
    secrets set --skip ETHERSCAN_API_KEY  "$(op read 'op://etherscan/api-key')"
```

Run the recipe during onboarding or after a rotation. Keys already present are left untouched (`--skip`); use `--overwrite` to force an update.

The same pattern works for HashiCorp Vault (`vault kv get`), AWS Secrets Manager (`aws secretsmanager get-secret-value`), or any CLI that prints a secret to stdout.

### Renaming and removing keys

```sh
secrets mv OLD_KEY NEW_KEY    # atomic rename
secrets rm OLD_KEY            # delete key and its history
```

Every time you overwrite a key, the current value is saved as a backup. Retrieve it if you need it:

```sh
secrets history RPC_URL
# RPC_URL~3:	https://rpc-v2.example.com
# RPC_URL~2:	https://rpc-v1.example.com
# RPC_URL~1:	https://rpc-old.example.com
```

The label matches the actual key stored. In the example, `RPC_URL~3` is the most recent backup, `RPC_URL~1` is the oldest.

---

## Command reference

| Command | Description |
|---------|-------------|
| `secrets init` | Create the encrypted store |
| `secrets set <key> [value]` | Add or update a secret |
| `secrets get <key>` | Print a secret to stdout |
| `secrets resolve` | Resolve project secrets and print as shell exports |
| `secrets ls [scope]` | List keys (unscoped, or filtered by scope prefix) |
| `secrets ls -a` | List all keys with full names |
| `secrets scope ls` | List all scope prefixes in the store |
| `secrets mv <from> <to>` | Rename a key |
| `secrets rm <key> [key...]` | Delete one or more keys (and their history) |
| `secrets history <key>` | Show prior values for a key (newest first) |
| `secrets import [scope] <file>` | Import keys from a `.env` file |
| `secrets dump` | Dump all secrets (debugging / migration) |
| `secrets passwd` | Change the store passphrase |
| `secrets agent [--ttl N]` | Adjust the agent lifetime |
| `secrets agent stop` | Wipe memory and stop the agent |

### `rm` flags

| Flag | Description |
|------|-------------|
| `-f`, `--force` | Skip the confirmation prompt |

### `set` flags

| Flag | Description |
|------|-------------|
| `--overwrite` | Replace existing key without prompting |
| `--skip` | Do nothing if key already exists |

When a key exists with a different value and neither flag is given, `set` prompts interactively: `[o]verwrite / [r]ename / [s]kip`. Setting the same value is a no-op.

### `import` flags

| Flag | Description |
|------|-------------|
| `--overwrite` | Replace all conflicting keys without prompting |
| `--skip` | Keep all existing keys, only import new ones |

### `resolve` flags

| Flag | Default | Description |
|------|---------|-------------|
| `-f`, `--file` | `.secrets.yaml` | Path to manifest file |
| `-p`, `--profile` | — | Active profile (auto-applies `default` if present) |
| `--format` | `posix` | Output format: `posix` (default), `fish`, `dotenv` |
| `--partial` | `false` | Export empty string for missing keys instead of erroring |

### Output formats

| Format | Example output | Shell usage |
|--------|----------------|-------------|
| `posix` | `export KEY='value'` | `eval "$(secrets resolve)"` |
| `fish` | `set -x KEY 'value'` | `secrets resolve --format fish \| source` |
| `dotenv` | `KEY="value"` | Pipe to files or other tools |

---

## The agent

The first command that needs the store auto-starts an agent: it decrypts the store into memory and serves all subsequent requests from there. You type your passphrase once and it stays unlocked for 8 hours.

You only need to interact with it directly if you want to change the lifetime:

```sh
secrets agent --ttl 4h    # restart with a shorter lifetime
secrets agent --ttl 0     # never expire
secrets agent stop        # wipe memory and exit immediately
```

The agent communicates over a Unix domain socket. It never writes decrypted data to disk.

---

## Security

- **Encryption**: [age](https://age-encryption.org) with scrypt key derivation (`filippo.io/age`)
- **No plaintext on disk**: secrets are never written unencrypted
- **Memory zeroing**: decrypted buffers are zeroed when the agent exits
- **Permissions**: store directory `0700`, file `0600`
- **Atomic writes**: temp file + rename prevents partial writes on crash
- **Empty passphrase**: fully supported — same model as unprotected SSH keys

The store lives at `~/.local/share/secrets/` by default (XDG). Override with `SECRETS_STORE_DIR`.

---

## Development

Requires Go 1.22+, [just](https://github.com/casey/just), and `protoc` (only for proto regeneration).

```sh
$ just
Available recipes:
    help             # Default recipe: show help

    [dev]
    setup            # Check and install dev toolchain dependencies
    proto            # Regenerate protobuf Go code from agent.proto (commit the result)
    fmt              # Format Go source code
    vet              # Run go vet
    lint             # Run staticcheck linter
    check            # Pre-commit quality gate: vet + lint + test

    [test]
    test             # Run unit tests
    test-v           # Run unit tests with verbose output
    test-integration # Run integration tests (requires built binary)
    test-race        # Run unit tests with race detector
    test-all         # Run all tests (unit + integration)
    coverage         # Generate test coverage report
    smoke            # Quick end-to-end smoke test against a temp store

    [build]
    build            # Build the binary
    install          # Install to GOPATH/bin
    cross-compile    # Cross-compile for all supported platforms

    [release]
    release-dry      # Dry-run goreleaser

```
