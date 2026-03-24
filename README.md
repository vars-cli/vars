# secrets

One encrypted store for all your environment variable secrets, shared across any number of projects.

---

If you work across many repositories — each with its own `.env` file full of private keys, RPC URLs, and API tokens — you've felt the pain: secrets duplicated everywhere, rotations that miss half the repos, files that accidentally get committed.

`secrets` keeps all your secrets in one age-encrypted store and exports them as environment variables on demand. It is **opt-in and non-breaking**: projects that don't use it keep working exactly as before. Projects that do opt in commit a `.secrets.yaml` listing the variable names they need — no values, just names — and each developer resolves them from their own personal store.

It exports env vars. What you do with them is up to you.

---

## Install

**From source:**

```sh
go install github.com/brickpop/secrets@latest
```

**From GitHub releases:**

```sh
# macOS (Apple Silicon)
curl -L https://github.com/brickpop/secrets/releases/latest/download/secrets_darwin_arm64.tar.gz | tar xz
sudo mv secrets /usr/local/bin/

# Linux (amd64)
curl -L https://github.com/brickpop/secrets/releases/latest/download/secrets_linux_amd64.tar.gz | tar xz
sudo mv secrets /usr/local/bin/
```

---

## The basics — your personal store

Start by creating an encrypted store. A passphrase is optional — press enter for none.

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
eval "$(secrets resolve)"                  # bash/zsh
echo $ETHERSCAN_API_KEY
```

`resolve` reads the manifest, looks up each key in your store, and prints shell-ready `export` statements. Nothing is written to disk.

Each developer uses their own store. The committed manifest is the shared contract; the secrets are personal.

---

## Scoped keys

As your store grows, you may have multiple variants of the same key — a prod private key, a testnet key, a backup. A naming convention keeps them organised: prefix keys with a scope, separated by `/`.

```sh
secrets set prod/PRIVATE_KEY "0xPRODKEY"
secrets set sepolia/PRIVATE_KEY "0xTESTKEY"
secrets set ETHERSCAN_API_KEY "abc123"       # shared, no scope needed
```

Scopes are just a naming convention. They can be nested (`arbitrum/dev/PRIVATE_KEY`) or use hyphens for compound names at any level (`arbitrum-dev/PRIVATE_KEY`).

List keys by scope:

```sh
secrets ls                   # top level keys only
secrets ls prod              # keys under prod/, prefix stripped from output
secrets ls -a                # all keys, full names
secrets scope ls             # list all scope prefixes present in the store
```

---

## Profiles — resolving the right scope

Once your keys are scoped, you need a way for `resolve` to know which scope to use for each run. That's what profiles are for: a list of `env var → store key` mappings, declared in `.secrets.yaml`.

```yaml
# .secrets.yaml
keys:
  - PRIVATE_KEY
  - RPC_URL
  - ETHERSCAN_API_KEY

profiles:
  default:
    PRIVATE_KEY: sepolia/PRIVATE_KEY
    RPC_URL: sepolia/RPC_URL
  mainnet:
    PRIVATE_KEY: prod/PRIVATE_KEY
    RPC_URL: prod/RPC_URL
```

```sh
secrets resolve              # uses "default" profile automatically
secrets resolve -p mainnet   # prod/ keys
```

`ETHERSCAN_API_KEY` isn't listed in either profile, so it falls back to a bare store lookup — the key name itself. Shared keys need no mapping.

`PRIVATE_KEY` will resolve to either `sepolia/PRIVATE_KEY` (by default) or to `prod/PRIVATE_KEY` when `-p mainnet` is passed. 

### Scope fallback in resolve

When resolving a key, `resolve` searches from most specific to least — stripping one scope level at a time until a match is found:

```
hoodi/dev/RPC_URL  →  hoodi/RPC_URL  →  RPC_URL
```

This means that your profile can reference a specific scoped key even if you've only stored the base key. Teams can share a common `RPC_URL` while individuals or environments override it at any specificity level, without changing the manifest.

### Team-wide key aliases

Use `mappings:` to rename store keys for everyone on the team, regardless of the profile:

```yaml
mappings:
  ETHERSCAN_API_KEY: ETHERSCAN_API_KEY_v2    # applies to all profiles
```

---

## Personal overrides

Profiles and mappings in `.secrets.yaml` are committed — they're the team's shared convention. But you may have personal variations: a wallet key, a different URL, a backup.

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

### Migrating from `.env` files

```sh
secrets import .env                  # import all keys
secrets import sepolia/dev .env      # import with a scope prefix → sepolia/dev/KEY
```

Conflicts are handled interactively (skip, overwrite). Use `--skip` or `--overwrite` for non-interactive imports.

### Renaming keys

```sh
secrets mv OLD_KEY NEW_KEY
```

Atomic — old key deleted and new key written in one operation.

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
| `secrets rm <key> [key...]` | Delete one or more keys |
| `secrets import [scope] <file>` | Import keys from a `.env` file |
| `secrets dump` | Dump all secrets (debugging / migration) |
| `secrets passwd` | Change the store passphrase |
| `secrets agent [--ttl N]` | Adjust the agent lifetime |
| `secrets agent stop` | Wipe memory and stop the agent |

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

The store lives at `~/.secrets/` by default. Override with `SECRETS_STORE_DIR`.

---

## Development

Requires Go 1.22+, [just](https://github.com/casey/just), and `protoc` (only for proto regeneration).

```sh
just setup       # check/install dev toolchain
just check       # vet + lint + test
just test-all    # unit + integration tests
just smoke       # quick end-to-end smoke test
just build       # build binary
just proto       # regenerate agent.pb.go (then commit)
```
