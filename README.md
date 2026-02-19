# CVPS CLI

Command-line interface for managing ClaudeVPS sandboxes.

## Installation

### macOS (Homebrew)

```bash
# Requires public tap repo: Achronon/homebrew-tap
brew tap Achronon/tap
brew install cvps
```

### Quick Install Script (macOS/Linux)

```bash
curl -fsSL https://raw.githubusercontent.com/Achronon/cvps/main/scripts/install.sh | sh
```

### Linux

```bash
# AMD64
curl -L https://github.com/Achronon/cvps/releases/latest/download/cvps-linux-amd64 -o /usr/local/bin/cvps
chmod +x /usr/local/bin/cvps

# ARM64
curl -L https://github.com/Achronon/cvps/releases/latest/download/cvps-linux-arm64 -o /usr/local/bin/cvps
chmod +x /usr/local/bin/cvps
```

### Windows

Download from [GitHub Releases](https://github.com/Achronon/cvps/releases)
or use winget:

```powershell
# Coming soon
# winget install achronon.cvps
```

### Distribution Setup (Maintainers)

See `docs/distribution.md` for public Homebrew tap and release automation setup.

### From Source

```bash
go install github.com/achronon/cvps/cmd/cvps@latest
```

## Quick Start

```bash
# Login
cvps login

# Create sandbox
cvps up --name my-project

# Check status of current context sandbox
cvps status

# Connect
cvps connect

# Sync files
cvps sync

# Terminate
cvps down
```

## Troubleshooting

### `cvps status` says no sandbox/context found

If you see:

`no sandbox specified and no context found: no sandbox context...`

Use one of these:

```bash
# Create a sandbox and save local context
cvps up

# Or inspect all your sandboxes
cvps status --all

# Or query a specific sandbox by ID
cvps status sbx-abc123
```

## Commands

| Command | Description |
|---------|-------------|
| `cvps login` | Authenticate with ClaudeVPS |
| `cvps logout` | Log out |
| `cvps up` | Provision new sandbox |
| `cvps down` | Terminate sandbox |
| `cvps status` | Show sandbox status |
| `cvps connect` | Open terminal to sandbox |
| `cvps sync` | Start file synchronization |
| `cvps migrate` | Upload local workspace |
| `cvps config` | Manage configuration |

## Configuration

Config file: `~/.cvps/config.yaml`

```yaml
api_key: cvps_xxx
api_base_url: https://api.claudevps.com

defaults:
  cpu_cores: 2
  memory_gb: 4
  storage_gb: 20
```

## Environment Variables

| Variable | Description |
|----------|-------------|
| `CVPS_API_KEY` | API key (overrides config) |
| `CVPS_API_URL` | API URL (overrides config) |

## Development

### Build

```bash
make build
```

### Test

```bash
make test
```

### Lint

```bash
make lint
```

### Clean

```bash
make clean
```

## License

Proprietary - ClaudeVPS SaaS
