# Comix — Comic Creator Studio

Comix converts text-based novels (markdown) into sequential comic panels using generative AI. It runs as both an HTTP server and a CLI tool.

## Quickstart

### 1. Install

```bash
go install github.com/comix/comix/cmd/comix@latest
```

Or build from source:

```bash
git clone https://github.com/comix/comix.git
cd comix
make build
```

### 2. Set your OpenAI API key

```bash
export OPENAI_API_KEY=sk-...
```

### 3. Prepare your novel

Create a directory with your novel as markdown files:

```
novels/alice/
├── cover.md
├── chapter_01.md
├── chapter_02.md
└── ...
```

### 4. Run the full pipeline

```bash
comix run --book-dir ./novels/alice/ --project alice
```

### 5. View output

```
comix-output/alice/
├── project.yaml
├── raw/
├── state/
│   ├── characters.json
│   └── scenes.json
├── sheets/
│   └── ..._3x2.png
├── poses/
│   └── ..._5x5.png
└── panels/
    └── scene_*.png
```

## CLI Reference

### Global Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--config` | `./config.yaml` | Config file path |
| `-o, --output` | `./comix-output` | Output directory |
| `-v, --verbose` | `false` | Enable verbose debug logging |
| `--log-format` | `text` | Log format: `text` or `json` |

### Commands

#### `comix run` — Full pipeline

```bash
# From book directory
comix run --book-dir ./novels/alice/ --project alice

# With explicit file paths
comix run --cover ./cover.md --chapters ch1.md,ch2.md --project alice

# Resume from checkpoint
comix run --book-dir ./novels/alice/ --project alice --resume
```

#### `comix serve` — HTTP server

```bash
comix serve --port 8080 --host 0.0.0.0
```

#### Individual phases

```bash
comix ingest --book-dir ./novels/alice/ --project alice
comix extract characters --project alice
comix extract scenes --project alice
comix generate sheets --project alice
comix generate poses --project alice
comix render --project alice
```

#### Project management

```bash
comix list                    # List all projects
comix status --project alice  # Show pipeline status
```

## HTTP API

Start the server:

```bash
comix serve
```

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/projects` | Create project |
| POST | `/api/projects/:id/ingest` | Upload markdown files |
| POST | `/api/projects/:id/run` | Execute full pipeline |
| POST | `/api/projects/:id/run/:phase` | Execute single phase |
| GET | `/api/projects/:id/status` | Pipeline status |
| GET | `/api/projects/:id/output` | List artifacts |
| GET | `/api/projects/:id/output/*` | Download file |
| DELETE | `/api/projects/:id` | Delete project |
| GET | `/api/projects` | List projects |
| GET | `/api/health` | Health check |

## Configuration

Comix uses a `config.yaml` file with environment variable overrides. See `config.yaml` for all options.

Key settings:

| Config Key | Env Var | Default | Description |
|------------|---------|---------|-------------|
| `openai.api_key` | `OPENAI_API_KEY` | — | OpenAI API key |
| `openai.llm.model` | — | `gpt-4o` | LLM model for extraction |
| `openai.image.model` | — | `gpt-image-2` | Image generation model |
| `openai.image.quality` | — | `medium` | Image quality (low/medium/high) |
| `pipeline.output_dir` | `COMIX_OUTPUT` | `./comix-output` | Output directory |
| `server.port` | — | `8080` | HTTP server port |
| `logging.level` | — | `info` | Log level (debug/info/warn/error) |
| `logging.format` | — | `text` | Log format (text/json) |

## Pipeline Phases

| Phase | Description | Output |
|-------|-------------|--------|
| 1. Ingestion | Read & validate markdown files | `raw/*.md`, `project.yaml` |
| 2. Characters | LLM extracts characters | `state/characters.json` |
| 3. Scenes | LLM extracts scene descriptions | `state/scenes.json` |
| 4. Base Sheets | Generate 3×2 character reference grids | `sheets/*_3x2.png` |
| 5. Dynamic Poses | Generate 5×5 pose grids | `poses/*_5x5.png` |
| 6. Rendering | Render sequential comic panels | `panels/scene_*.png` |

## Troubleshooting

### "OPENAI_API_KEY is not set"

Set your key: `export OPENAI_API_KEY=sk-...` or add it to `config.yaml`:
```yaml
openai:
  api_key: sk-...
```

### Pipeline fails with rate limit errors

gpt-image-2 has rate limits (default 5 images/min). Adjust in config:
```yaml
openai:
  image:
    rate_limit_rpm: 3
```

### Resume after interruption

```bash
comix run --book-dir ./novels/alice/ --project alice --resume
```

The pipeline checks for existing checkpoints and skips completed phases.

### Enable debug logging

```bash
comix run --book-dir ./novels/alice/ --project alice --verbose
```

Or in config:
```yaml
logging:
  level: debug
  format: json
```
