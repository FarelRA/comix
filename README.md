# Comix

Comix turns text-based novels into sequential comic panels using AI. Give it your story as markdown files, and it handles character extraction, scene breakdown, character sheet generation, and panel rendering — all in one command.

## Quick start

```bash
export OPENAI_API_KEY=sk-...
comix run --book-dir ./novels/alice/ --project alice
# Output: comic panels saved to comix-output/alice/panels/
```

## Features

- **Full pipeline automation** — goes from raw text to comic panels in one command
- **Character consistency** — generates reference sheets from 6 angles and dynamic pose grids so each character looks the same in every panel
- **Scene-aware rendering** — each panel is rendered with knowledge of the previous one for visual continuity
- **Checkpoint resume** — pick up where you left off if a run is interrupted
- **CLI + HTTP server** — run locally or deploy as a web service

## Installation

Requires **Go 1.26+** and an **OpenAI API key** with access to GPT-4o and gpt-image-2.

```bash
go install github.com/comix/comix/cmd/comix@latest
```

Or build from source:

```bash
git clone https://github.com/FarelRA/comix.git
cd comix
make build
```

## Usage

### Run the full pipeline

```bash
export OPENAI_API_KEY=sk-...
comix run --book-dir ./novels/alice/ --project alice
```

Comix processes your novel in 6 phases:

| Phase | What happens |
|-------|-------------|
| 1. Ingestion | Reads cover and chapter markdown files |
| 2. Characters | AI identifies characters and their descriptions |
| 3. Scenes | AI breaks chapters into visual scenes |
| 4. Sheets | Generates 3×2 character reference grids from 6 angles |
| 5. Poses | Creates 5×5 dynamic pose sheets for each character |
| 6. Render | Produces sequential comic panels |

### Start the HTTP server

```bash
comix serve --port 8080
```

### Resume an interrupted run

```bash
comix run --book-dir ./novels/alice/ --project alice --resume
```

### Run individual phases

```bash
comix ingest --book-dir ./novels/alice/ --project alice
comix extract characters --project alice
comix extract scenes --project alice
comix generate sheets --project alice
comix generate poses --project alice
comix render --project alice
```

### Output structure

```
comix-output/alice/
├── raw/       # your original markdown files
├── state/     # extracted characters and scene descriptions
├── sheets/    # 3×2 character reference grids
├── poses/     # 5×5 dynamic pose sheets
└── panels/    # the final comic panels
```

## Configuration

Your OpenAI API key can be set via environment variable or in `config.yaml`:

```bash
export OPENAI_API_KEY=sk-...
```

See `config.yaml` for all available options (LLM model, image quality, server port, logging, etc.).

## Contributing

Pull requests are welcome. For major changes, please open an issue first to discuss what you would like to change.

Please make sure to update tests as appropriate and run `make lint` before submitting.

## License

[GNU General Public License v3.0](LICENSE)
