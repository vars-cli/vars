# vars

One encrypted store for all your environment variables, shared across any number of projects.

---

If you work across multiple projects, the problems are familiar: secrets duplicated across `.env` files, rotations that miss half the repos, one accidental commit away from a leak — and now AI coding assistants that can read everything in your working directory.

`vars` keeps everything in a single encrypted personal store and resolves secrets as environment variables on demand. No server, no account, no cloud. **Opt-in and non-breaking** — teammates who don't use it are unaffected.

`vars` loads env vars into your session. What you do with them is up to you.

---

## Install

```sh
# macOS (Apple Silicon)
curl -L https://github.com/vars-cli/vars/releases/download/v0.1.0/vars_0.1.0_darwin_arm64.tar.gz | tar xz
sudo mv vars /usr/local/bin/

# Linux / WSL (amd64)
curl -L https://github.com/vars-cli/vars/releases/download/v0.1.0/vars_0.1.0_linux_amd64.tar.gz | tar xz
sudo mv vars /usr/local/bin/
```

---

## The basics — your personal store

Run `vars` to get started. If no store exists yet, it walks you through creating one and choosing a passphrase (optional).

Store values:

```sh
vars set DB_PASSWORD              # prompts for the value (keeps it out of shell history)
vars set API_TOKEN "abc123"
vars set RPC_URL "https://rpc.example.com"
```

Read them back:

```sh
vars get RPC_URL                  # prints the value
vars ls                           # lists all keys
```

One place for all your keys, encrypted at rest, accessible from any terminal.

The first time you run a command you'll be asked for your passphrase. After that, the store stays unlocked in the background for 8 hours — no re-entering it between commands or across terminal sessions.

---

## Using in a project

Add `.vars.yaml` to your project declaring the env var names it needs. Commit it :

```yaml
# .vars.yaml
keys:
  - DB_URL
  - API_TOKEN
  - PRIVATE_KEY
```

Then resolve them into your shell, on demand:

```sh
eval "$(vars resolve)"          # bash/zsh
echo $API_TOKEN                 # loaded as env vars
```

`resolve` reads the project manifest, looks up each key in your store, and prints shell-ready `export` statements. Nothing is written to disk.

Each developer uses their own store. The committed manifest is the shared contract; the secrets are personal.

That's the core workflow. Everything from here is optional — adopt it as your needs grow.

---

## Scoped keys

As your store grows, you may have multiple variants of the same key — a `prod` private key, a `staging` key, etc. A naming convention keeps them organised: prefix keys with a scope, separated by `/`.

```sh
vars set prod/PRIVATE_KEY "0xPRODKEY"
vars set hoodi/PRIVATE_KEY "0xTESTKEY"
vars set hoodi/dev/PRIVATE_KEY "0xTESTKEY2"    # nested scope
vars set SERVER_API_KEY "abc123"            # shared, no scope needed
```

List keys by scope:

```sh
vars ls                   # top level keys only
vars ls prod              # keys under prod/, prefix stripped from output
vars ls -a                # keys from all scopes
vars scope ls             # list all scope prefixes present in the store
```

---

## Profiles — resolving the right scope

If you have scoped variables, you will need `resolve` to know which scope to use for each run. That's what `profiles` are for: a list of `env var → store key` mappings, declared in `.vars.yaml`.

```yaml
# .vars.yaml
keys:
  - PRIVATE_KEY
  - RPC_URL
  - SERVER_API_KEY

profiles:
  default:
    PRIVATE_KEY: dev/PRIVATE_KEY
    RPC_URL: sepolia/RPC_URL
  mainnet:
    PRIVATE_KEY: prod/PRIVATE_KEY
    RPC_URL: mainnet/RPC_URL
```

```sh
vars resolve              # uses the "default" profile automatically
vars resolve -p mainnet   # mappings from the mainnet profile
```

`PRIVATE_KEY` will resolve to either `sepolia/PRIVATE_KEY` (by default) or to `prod/PRIVATE_KEY` when `-p mainnet` is passed. 

Profiles are opt-in. Any keys (`SERVER_API_KEY`) not listed in a profile continue to work as usual.

### Scope fallback in resolve

When resolving a key, `resolve` searches from most specific to least — stripping one scope level at a time until a match is found:

`hoodi/dev/RPC_URL`  →  `hoodi/RPC_URL`  →  `RPC_URL`  →  Not found.

This means that your profile can reference a specific scoped key even if you've only stored the base key. Teams can share a common `RPC_URL` while individuals or environments override it at any specificity level, without changing the manifest.

### Team-wide key aliases

Use `mappings:` to rename store keys for everyone on the team, regardless of the profile:

```yaml
mappings:
  SERVER_API_KEY: SERVER_API_KEY_v2    # applies to all profiles
```

---

## Personal overrides

Profiles and mappings in `.vars.yaml` are committed — they are the team's shared convention. But you may have personal variations: a wallet key, a different URL, a local backup.

Add `.vars.local.yaml` (git-ignored, never commit it) alongside `.vars.yaml`:

```yaml
# .vars.local.yaml
mappings:
  PRIVATE_KEY: prod/PRIVATE_KEY_alice_hw    # override for all profiles

profiles:
  mainnet:
    RPC_URL: prod/RPC_URL_quicknode         # only override this one key in mainnet
```

Local overrides take priority over `.vars.yaml`, per key. Everyone has the same manifest; only personal overrides differ.

---

## Mixing with a .env file

You can pipe an existing `.env` into `vars resolve`. Store values take priority for manifest keys; the dotenv acts as a fallback for keys not yet in the store; anything not declared in the manifest passes through unchanged.

```sh
cat .env | vars resolve             # error if a manifest key is missing from both sources
cat .env | vars resolve --partial   # skip keys missing from both, pass the rest through
```

This makes migration gradual: keep your `.env` in place and move keys into the store one by one. As soon as a key is in the store, `vars` uses the store value automatically — no changes to your workflow.

---

## More use cases

### Scripts and task runners

Using `just` or `make`:

```sh
# justfile
deploy:
    #!/usr/bin/env bash
    eval "$(vars resolve -p mainnet)"
    forge script script/Deploy.s.sol --broadcast

test:
    #!/usr/bin/env bash
    eval "$(vars resolve)"
    forge test
```

### Docker containers

Pass secrets to a container without writing anything to disk, using bash process substitution:

```sh
docker run --env-file <(vars resolve --dotenv) my-image
```

`--env-file` reads from the file descriptor provided by `<(...)` — the output of `vars resolve` never touches the filesystem.

### Migrating from `.env` files

```sh
vars import .env                    # import all keys
vars import my-project/dev .env     # import with a scope prefix → my-project/dev/KEY
```

Conflicts are handled interactively (skip, overwrite). Use `--skip` or `--force` for non-interactive imports.

### Integrating with external vaults

`vars` resolves to plain env vars, so it composes with anything. If your team uses 1Password, you can sync keys from it using the `op` CLI:

```sh
# justfile — sync secrets from 1Password (nop if values didn't change)
sync-from-op:
    #!/usr/bin/env bash
    vars set dev/RPC_URL        "$(op read 'op://dev/rpc/url')"
    vars set dev/PRIVATE_KEY    "$(op read 'op://dev/wallet/private-key')"
    vars set SERVER_API_KEY  "$(op read 'op://server/api-key')"
```

Run the recipe during onboarding or after a rotation. Keys already present are left untouched (`--skip`); use `--force` to overwrite.

The same pattern works for HashiCorp Vault (`vault kv get`), AWS Secrets Manager (`aws secretsmanager get-secret-value`), or any CLI that prints a secret to stdout.

### Renaming and removing keys

```sh
vars mv OLD_KEY NEW_KEY    # atomic rename
vars rm OLD_KEY            # delete key and its history
```

Every time you overwrite a key, the current value is saved as a backup. Retrieve it if you need it:

```sh
vars history RPC_URL
# RPC_URL~3:	https://rpc-v2.example.com
# RPC_URL~2:	https://rpc-v1.example.com
# RPC_URL~1:	https://rpc-old.example.com
```

In the example, `RPC_URL~3` is the most recent backup, `RPC_URL~1` is the oldest.

The label matches the key internally stored. Backups are hidden when using `ls`, `resolve` or `dump`, but they can be accessed if needed:

```sh
vars get RPC_URL~2
# https://rpc-v1.example.com
```

---

## Command reference

| Command | Description |
|---------|-------------|
| `vars set <key> [value]` | Add or update a secret |
| `vars get <key>` | Print a secret to stdout |
| `vars resolve` | Resolve project variables and print as shell exports |
| `vars ls [scope]` | List keys (unscoped, or filtered by scope prefix) |
| `vars ls -a` | List all keys with full names |
| `vars scope ls` | List all scope prefixes in the store |
| `vars mv <from> <to>` | Rename a key |
| `vars rm <key> [key...]` | Delete one or more keys (and their history) |
| `vars history <key>` | Show prior values for a key (newest first) |
| `vars import [scope] <file>` | Import keys from a `.env` file |
| `vars dump` | Dump all variables (debugging / migration) |
| `vars passwd` | Change the store passphrase |
| `vars agent [--ttl N]` | Adjust the agent lifetime |
| `vars agent stop` | Wipe memory and stop the agent |

### `rm` flags

| Flag | Description |
|------|-------------|
| `-f`, `--force` | Skip the confirmation prompt |

### `set` flags

| Flag | Description |
|------|-------------|
| `-f`, `--force` | Overwrite existing key without prompting |
| `--skip` | Do nothing if key already exists |

When a key exists with a different value and neither flag is given, `set` prompts interactively: `[o]verwrite / [r]ename / [s]kip`. Setting the same value is a no-op.

### `import` flags

| Flag | Description |
|------|-------------|
| `-f`, `--force` | Overwrite all conflicting keys without prompting |
| `--skip` | Keep all existing keys, only import new ones |

### `resolve` flags

| Flag | Default | Description |
|------|---------|-------------|
| `-f`, `--file` | `.vars.yaml` | Path to manifest file |
| `-p`, `--profile` | — | Active profile (auto-applies `default` if present) |
| `--dotenv` | — | Output as `KEY=value` |
| `--fish` | — | Output in fish shell format |
| `--partial` | — | Skip missing keys instead of erroring |
| `--origins` | — | Annotate each output line with its source as an inline comment (`# vars`, `# stdin`, `# not set`) |

Default output is `export KEY='value'`, which you can pipe into `eval "$(vars resolve)"` in bash/zsh.

`--origins` annotates each line with where the value came from. Useful for auditing or debugging which source won. The comments are eval-safe (shell ignores `#`):

```sh
$ cat .env | vars resolve --partial --origins
export DB_URL='postgres://...'  # vars
export API_TOKEN='xyz'          # stdin
# STRIPE_SECRET  not set
```

---

## The agent

The first command that needs the store auto-starts an agent: it decrypts the store into memory and serves all subsequent requests from there. You type your passphrase once and it stays unlocked for 8 hours.

You only need to interact with it directly if you want to change the lifetime:

```sh
vars agent --ttl 4h    # restart with a shorter lifetime
vars agent --ttl 0     # never expire
vars agent stop        # wipe memory and exit immediately
```

To set a persistent default, add `VARS_AGENT_TTL` to your shell profile:

```sh
export VARS_AGENT_TTL=4h   # e.g. 30m, 4h, 12h, 1d, 0 for unlimited
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

The store lives at `~/.local/share/vars/` by default (XDG). Override with `VARS_STORE_DIR`.

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
