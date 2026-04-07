# Safe Vars

Secure environment variables for your projects with UNIX-like composability. 

One encrypted personal store, shared across all your repos. No more scattered `.env` files, accidental leaks, secrets out of sync or even read by a coding agent.

---

## Quick start

**1. Install**

```sh
# macOS (Apple Silicon)
curl -L https://github.com/vars-cli/vars/releases/latest/download/vars_darwin_arm64.tar.gz | tar xz
sudo mv vars /usr/local/bin/

# Linux / WSL (amd64)
curl -L https://github.com/vars-cli/vars/releases/latest/download/vars_linux_amd64.tar.gz | tar xz
sudo mv vars /usr/local/bin/
```

**2. Store a secret**

```sh
vars set RPC_URL "https://rpc.example.com"
vars set PRIVATE_KEY          # prompts for the value (keeps it out of shell history)
```

**3. Use it in a project**

```sh
vars init                     # creates .vars.yaml — commit this
# edit .vars.yaml to list your keys
eval "$(vars resolve)"        # bash/zsh: loads keys as env vars
```

That's the core loop. The rest is optional.

---

## What this solves

- **No more `.env` leaks** — secrets never touch your project repo
- **One place for all keys** — personal store shared across projects and scopes
- **Team-friendly** — commit `.vars.yaml` (what variables the project needs); each developer has their own store
- **Gradual adoption** — pipe an existing `.env` in; move keys to the store when you can
- **Multi-environment** — scoped keys (`prod/KEY`, `dev/KEY`, `KEY`) with profiles for different contexts

---

## The basics — your personal store

Run `vars` with no arguments to get started. If no store exists yet, it walks you through creating one and choosing a passphrase (optional).

```sh
vars set DB_URL "http://user@server/db"
vars set API_TOKEN               # prompts for the value
vars set PRIVATE_KEY
vars get DB_URL                  # print the value
vars ls                          # list the keys
```

The first time you run a command you'll be asked for your passphrase. After that, the store stays unlocked in your session for 8 hours — no re-entering it between commands or across terminal sessions.

---

## Using in a project

Add `.vars.yaml` to your project declaring the env var names it needs. Commit it:

```sh
vars init                         # scaffolds .vars.yaml with examples
```

```yaml
# .vars.yaml
keys:
  - DB_URL
  - API_TOKEN
  - PRIVATE_KEY
```

Then resolve them into your shell, on demand:

```sh
eval "$(vars resolve)"            # bash/zsh
vars resolve --fish | source      # fish
```

`resolve` reads the manifest, looks up each key in your store, and prints shell-ready `export` statements with the right values to load into your shell. Nothing is written to disk.

Each developer uses their own store. The `.env.vars` manifest is the shared contract; the secrets are personal.

---

## Scoped keys

As your store grows, you may have multiple variants of the same key — a `prod` key, a `dev` key, etc. A naming convention keeps them organised: prefix with a scope, separated by `/`.

```sh
vars set prod/PRIVATE_KEY "0xPROD_KEY"
vars set dev/PRIVATE_KEY "0xDEV_KEY"
vars set dev/temp/PRIVATE_KEY "0xTEMP_KEY"     # nested scope
vars set SERVER_API_KEY "abc123"               # shared, no scope needed
```

```sh
vars ls                   # top-level keys only
vars ls prod              # keys under prod/
vars ls -a                # all keys from all scopes
vars scope ls             # list all scope prefixes in the store
```

---

## Profiles — resolving the right scope

Profiles are named sets of `env var → store key` mappings, declared in `.vars.yaml`. They tell `resolve` which scope/key to use for each run.

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
vars resolve              # "default" profile applied automatically
vars resolve -p mainnet   # use the mainnet profile
```

### Scope fallback in resolve

When resolving a key, `resolve` searches from most specific to least — stripping one scope level at a time:

`dev/temp/RPC_URL`  →  `dev/RPC_URL`  →  `RPC_URL`  →  not found

This lets profiles reference specific scoped keys even if you've only stored the base key.

### The `global:` profile

`global:` is a reserved profile name that is always applied as a fallback, regardless of which profile is active. Use it for base aliases that apply to every context:

```yaml
profiles:
  global:
    SERVER_API_KEY: SERVER_API_KEY_v2    # applies in every profile
  default:
    PRIVATE_KEY: dev/PRIVATE_KEY
  mainnet:
    PRIVATE_KEY: prod/PRIVATE_KEY        # overrides global: if it were there too
```

The active profile always takes precedence over `global:`. `global:` fills in what the active profile doesn't cover.

All keys used in a profile need to be listed in `keys:`. Profiles only provide mappings; `keys:` is what gets resolved.

### Inline literals and defaults

Profile values support two special prefixes:

| Syntax | Behaviour |
|--------|-----------|
| `= value` | Emit this literal value — no store lookup |
| `?= value` | Use an available value if present and non-empty; otherwise emit this default |

```yaml
profiles:
  global:
    LOG_LEVEL: = info                    # emits "info", no store lookup
    RPC_URL: ?= http://localhost:8545    # store wins; falls back to localhost if missing
  ci:
    DRY_RUN: = true
    API_URL: ?= http://localhost:8080
```

---

## Local overrides

Profiles in `.vars.yaml` are committed: they are the team's shared convention. For local overrides you can add `.vars.local.yaml` alongside `.vars.yaml`. It should be git-ignored; never commit it.

`.vars.local.yaml` has the same structure as `.vars.yaml`, minus `keys:`:

```yaml
# .vars.local.yaml
profiles:
  global:
    PRIVATE_KEY: prod/PRIVATE_KEY_alice_hw    # override for all profiles
  mainnet:
    RPC_URL: prod/RPC_URL_quicknode           # only override this one key in mainnet
```

Local overrides take priority over `.vars.yaml`, per key, per profile.

---

## Working with existing env vars

You can pipe existing `.env` files into `vars resolve`. Store values take priority for manifest keys; the dotenv acts as a fallback for keys not yet in the store. Any keys not declared in the manifest pass through unchanged.

```sh
cat .env | vars resolve             # error if a manifest key is missing from both sources
cat .env | vars resolve --partial   # ignore keys missing from both, pass the rest through
```

If a manifest key isn't found in the store or a piped `.env`, then `vars resolve` checks the current shell as a last fallback. If the variable already exists, no export is emitted (the value is already there). Otherwise, an error is thrown.

Use `--origin` to see where each value came from:

```sh
$ cat .env | vars resolve --partial --origin
export DB_URL='postgres://...'  # vars
export API_TOKEN='xyz'          # .env
export LOG_LEVEL='info'         # manifest
export RPC_URL='http://...'     # manifest
# PRIVATE_KEY  shell
# STRIPE_SECRET  missing
```

| Origin | Meaning |
|--------|---------|
| `vars` | Value from the encrypted store |
| `.env` | Value from piped stdin (dotenv file) |
| `manifest` | Inline literal (`= value`) or inline default (`?= value`) |
| `shell` | Already in the calling shell (no export emitted) |
| `missing` | Not found anywhere (only appears with `--partial`) |

---

## More use cases

### Scripts and task runners

```just
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

`--dotenv` outputs bare `KEY=value` (no quoting), compatible with `docker --env-file`. The `<(...)` process substitution means the output never touches the filesystem.

### Migrating from `.env` files

```sh
vars import .env                    # import keys
vars import my-project/dev .env     # import with a scope prefix → my-project/dev/KEY_NAME
rm .env
```

Conflicts are handled interactively (replace, new name, skip). Use `--replace` or `--skip` for non-interactive imports.

### Integrating with external vaults

`vars` resolves to plain env vars, so it composes with anything.

```just
# justfile
sync:
    vars set dev/RPC_URL      "$(op read 'op://dev/rpc/url')"             # 1Password
    vars set dev/API_KEY      "$(vault kv get -field=value dev/api_key)"  # HashiCorp Vault
    vars set dev/SECRET       "$(aws secretsmanager get-secret-value --secret-id dev/secret --query SecretString --output text)"
```

Run this during onboarding or after a key rotation. As before, `--skip` leaves existing keys untouched; `--replace` replaces them.

### Renaming and removing keys

```sh
vars mv OLD_KEY NEW_KEY           # atomic rename — prompts for confirmation
vars mv OLD_KEY NEW_KEY --force   # skip prompt (scripts / CI)
vars rm OLD_KEY                   # delete key and its history — prompts for confirmation
vars rm OLD_KEY --force           # skip prompt
```

## Checking older values

```sh
vars history RPC_URL
# RPC_URL~3:	https://rpc-v2.example.com   (most recent)
# RPC_URL~2:	https://rpc-v1.example.com
# RPC_URL~1:	https://rpc-old.example.com
vars get RPC_URL~2          # retrieve a specific backup
```

---

## Command reference

### Store management

```sh
vars set <key> [value]        # Set a value (prompts if omitted)
vars get <key>                # Print a value to stdout
vars ls [scope]               # List keys (filtered to scope if given)
vars ls -a                    # List all keys with full names
vars scope ls                 # List all scope prefixes in the store
vars mv <old> <new>           # Rename a key (history moves with it)
vars rm <key>...              # Delete one or more keys and their history
vars history <key>            # Show value history (newest first)
vars import [scope] <file>    # Import from a .env file
vars dump                     # Dump all store keys and values
vars passwd                   # Change the store passphrase
```

### Project integration

```sh
vars init                     # Scaffold .vars.yaml in the current directory
vars resolve [flags]          # Resolve manifest keys as shell exports
```

### Agent

```sh
vars agent [--ttl N]          # Adjust daemon lifetime (default: 8h)
vars agent --stdin            # Start agent, read passphrase from stdin (CI)
vars agent stop               # Wipe memory and stop immediately
```

### `set` flags

| Flag | Description |
|------|-------------|
| `--replace` | Replace existing key without prompting |
| `--skip` | Do nothing if key already exists |

When a key exists with a different value and neither flag is given, `set` prompts: `[r]eplace  [n]ew name  [s]kip`. Same value is a no-op.

### `import` flags

| Flag | Description |
|------|-------------|
| `--replace` | Replace all conflicting keys |
| `--skip` | Keep existing keys, only import new ones |

### `resolve` flags

| Flag | Default | Description |
|------|---------|-------------|
| `-f`, `--file` | `.vars.yaml` | Path to manifest file |
| `-p`, `--profile` | — | Active profile (`default` auto-applied if present) |
| `--dotenv` | — | Output as bare `KEY=value` (compatible with `docker --env-file`) |
| `--fish` | — | Output in fish shell format |
| `--partial` | — | Skip missing keys instead of erroring |
| `--origin` | — | Annotate each line with its source |

### `mv` flags

| Flag | Description |
|------|-------------|
| `-f`, `--force` | Skip confirmation prompt |

### `rm` flags

| Flag | Description |
|------|-------------|
| `-f`, `--force` | Skip confirmation prompt |

---

## How it works

### Resolution priority

When resolving a key with `--profile mainnet`:

1. `mainnet` profile, `.vars.local.yaml`
2. `mainnet` profile, `.vars.yaml`
3. `global:` profile, `.vars.local.yaml`
4. `global:` profile, `.vars.yaml`
5. Bare key (identity — key name equals store key)

### Scope fallback

When looking up a store key, `vars` tries progressively less specific:

`prod/dev/RPC_URL`  →  `prod/RPC_URL`  →  `RPC_URL`  →  not found

---

## The agent

The first command that needs the store auto-starts an agent: it decrypts the store into memory and serves all subsequent requests from there. You type your passphrase once and it stays unlocked for 8 hours.

You only need to interact with it directly to change the lifetime:

```sh
vars agent --ttl 4h    # restart with a shorter lifetime
vars agent --ttl 0     # never expire
vars agent stop        # wipe memory and exit immediately
```

To set a persistent default, add `VARS_AGENT_TTL` to your shell profile:

```sh
export VARS_AGENT_TTL=4h   # e.g. 15, 60s, 30m, 4h, 12h, 1d, 0 for unlimited
```

### CI and non-interactive use

Pre-start the agent with the passphrase before running other commands (use an empty string if no passphrase was set):

```sh
echo "$STORE_PASSPHRASE" | vars agent --stdin
cat .env | vars resolve --partial
```

All destructive commands support flags for non-interactive flows. Use these in scripts to skip confirmation prompts:

```sh
vars set KEY value --replace          # replace without prompt
vars import .env --replace            # import, replacing conflicts
vars mv OLD_KEY NEW_KEY --force       # rename without prompt
vars rm KEY --force                   # delete without prompt
```

---

## Security

- **Encryption**: [age](https://age-encryption.org) with scrypt key derivation (`filippo.io/age`)
- **No plaintext on disk**: secrets are never written unencrypted
- **Memory zeroing**: decrypted buffers are zeroed when the agent exits
- **Permissions**: store directory `0700`, file `0600`
- **Atomic writes**: temp file + rename prevents partial writes on crash
- **Empty passphrase**: supported: same model as unprotected SSH keys

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
