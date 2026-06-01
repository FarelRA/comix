# Comix — Comic Creator Studio

## Overview

Comix is a single-binary tool that converts text-based novels (markdown) into sequential comic panels using generative AI. It runs as both an HTTP server and a CLI tool, orchestrating a 6-phase pipeline:

1. **Ingestion** — Read and validate cover + chapter markdown files
2. **Pass One (Characters)** — LLM extracts characters into a running Character Note
3. **Pass Two (Scenes)** — LLM extracts sequential scenes using the complete Character Note
4. **Base Model Sheets** — gpt-image-2 generates 3×2 character reference grids (6 angles)
5. **Dynamic Poses** — gpt-image-2 image-to-image generates 5×5 pose grids from base sheets
6. **Sequential Rendering** — Each scene is rendered using the previous panel as reference for visual continuity

## Original

I am building an automated Comic Creator Studio. The system uses generative AI to convert text-based novels into sequential comic panels. Here is the required workflow:

**Phase 1: Ingestion**
The system receives markdown uploads for a novel. This includes a single cover markdown file and sequential chapter markdown files.

**Phase 2: Pass One - Character Extraction**
Run the text through an LLM. Feed it the cover markdown and one chapter at a time. The LLM must find all characters, extract their physical descriptions, and map their character traits using a predefined JSON schema. Save this data to a running "Character Note" state. For each subsequent chapter, pass the same Note back into the LLM so it can append new characters or update existing ones without losing the accumulated context.

**Phase 3: Pass Two - Scene Extraction**
Once the Character Note is fully populated for the uploaded chapters, run a second LLM pass. Feed it the cover, the chapter text, and the complete Character Note. The AI must extract sequential scene descriptions and attach the specific characters that appear in each scene using a predefined JSON format.

**Phase 4: Base Model Sheet Generation (3x2 Grid)**
Using the physical descriptions from the Character Note, trigger an image generation API. Generate a base reference model sheet for each character formatted as a strict 3x2 image grid. The grid must feature the character in a neutral pose from exactly 6 specific angles: front, back, right profile, left profile, top-down (up), and bottom-up. Every slot must be filled with one of these specific angles to prevent generation artifacts.

**Phase 5: Dynamic Pose Generation (5x5 Grid)**
Take the generated 3x2 base reference image and feed it back into an image-to-image generative model. Use it to generate a massive 5x5 grid containing 25 distinct dynamic poses for that specific character, ensuring strict visual consistency with the 3x2 sheet.

**Phase 6: Sequential Scene Rendering**
Render the comic scenes in order. For the first scene, generate the image using the scene description and the corresponding character reference sheets. For every subsequent scene, the prompt must include the new scene description, the required characters, AND the generated image from the previous scene to maintain visual flow and spatial continuity.

---

## Technology Stack

| Layer | Choice | Rationale |
|---|---|---|
| Language | **Go 1.26+** | Single static binary, excellent stdlib (HTTP, JSON, image), goroutine concurrency, fast compilation |
| CLI Framework | **Cobra + Viper** | Industry standard for Go CLI; composable subcommands; config merging (file + env + flags) |
| HTTP Router | **`go-chi/chi/v5`** | Lightweight, idiomatic, stdlib-compatible, middleware-friendly |
| LLM Backend | **OpenAI GPT-4o** (Chat Completions API + `response_format: json_object`) | Reliable structured JSON output for character/scene extraction |
| Image Backend | **OpenAI gpt-image-2** (`v1/images/generations` + `v1/images/edits`) | Text-to-image + image-to-image editing + strong layout reasoning |
| State Storage | **File-based JSON + YAML** | No database; pure filesystem. Human-readable, diff-able, checkpointable |
| Output | **Individual PNG panels** | One PNG per scene, saved to organized directory tree |

### Why Go

- **Single static binary**: No JVM, no Python runtime, no node_modules. `go build` produces one file.
- **Rich stdlib**: `net/http`, `encoding/json`, `image/png`, `io/fs`, `context` — all built-in.
- **Concurrency**: Goroutines map naturally to parallel API calls (multiple character sheets, batch poses).
- **Cross-compilation**: Build for linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64 from one machine.

---

## Project Structure

```
comix/
│
├── cmd/
│   └── comix/
│       └── main.go                  # Entry point (~30 lines): parse mode flag, delegate to server or CLI
│
├── internal/
│   ├── config/
│   │   └── config.go                # Viper-based config loading (config.yaml + env vars + CLI flags)
│   │
│   ├── server/
│   │   ├── server.go                # HTTP server: chi router setup, listen, graceful shutdown
│   │   ├── handlers.go              # REST handlers for all endpoints
│   │   └── middleware.go            # Logging, CORS, request ID, rate-limit awareness
│   │
│   ├── cli/
│   │   ├── root.go                  # Root cobra command with global flags (--config, --output, --verbose)
│   │   ├── run.go                   # `comix run` — full pipeline orchestration
│   │   ├── serve.go                 # `comix serve` — start HTTP server
│   │   ├── ingest.go                # `comix ingest` — Phase 1 only
│   │   ├── extract.go               # `comix extract characters|scenes` — Phase 2 / 3
│   │   ├── generate.go              # `comix generate sheets|poses` — Phase 4 / 5
│   │   └── render.go                # `comix render` — Phase 6
│   │
│   ├── pipeline/
│   │   ├── pipeline.go              # Orchestrator: state machine that coordinates phases 1-6
│   │   ├── ingest.go                # Phase 1: read markdown, infer chapter order, validate structure
│   │   ├── characters.go            # Phase 2: iterative LLM character extraction with context accumulation
│   │   ├── scenes.go                # Phase 3: LLM scene extraction using complete CharacterNote
│   │   ├── sheets.go                # Phase 4: 3×2 base model sheet generation via gpt-image-2
│   │   ├── poses.go                 # Phase 5: 5×5 dynamic pose generation via img2img editing
│   │   └── render.go                # Phase 6: sequential scene rendering with visual continuity
│   │
│   ├── llm/
│   │   ├── client.go                # OpenAI Chat Completions wrapper (JSON mode, message construction, retries)
│   │   └── prompts.go               # All system prompts + few-shot examples for character/scene extraction
│   │
│   ├── imagegen/
│   │   ├── client.go                # gpt-image-2 client: text-to-image + image-to-image edit, retry, rate-limit
│   │   └── prompts.go               # Image generation prompt templates for sheets, poses, scenes
│   │
│   ├── model/
│   │   ├── character.go             # Character, CharacterNote structs with JSON tags + validation
│   │   ├── scene.go                 # Scene, SceneList structs with JSON tags + validation
│   │   └── project.go               # Project manifest, Chapter metadata, PipelineProgress structs
│   │
│   ├── state/
│   │   └── state.go                 # Load/save utility for JSON state files, checkpoint/resume logic
│   │
│   └── storage/
│       ├── storage.go               # File I/O abstraction: read markdown, save images, list directories
│       └── paths.go                 # Path generation helpers for output directory structure
│
├── config.yaml                      # Default configuration file (checked into repo)
├── .env.example                     # Environment variable template
├── go.mod
├── go.sum
├── Makefile                         # build, test, lint, clean, cross-compile targets
└── .goreleaser.yaml                 # Automated multi-platform builds
```

---

## Data Models

### Character Note (`internal/model/character.go`)

```json
{
  "$schema": "comix/character-note/v1",
  "characters": [
    {
      "id": "alice",
      "name": "Alice",
      "physical_description": "Young girl approximately 7 years old, shoulder-length blonde hair tied back with a black ribbon, bright blue eyes, fair complexion, wearing a blue Victorian-style dress with a white pinafore apron and black patent leather shoes",
      "personality_traits": ["curious", "brave", "imaginative", "polite", "determined"],
      "first_chapter": "chapter_01",
      "chapters_seen": ["chapter_01", "chapter_02", "chapter_03"],
      "aliases": [],
      "notable_actions": ["follows the White Rabbit down the hole", "grows and shrinks after eating/drinking"],
      "relationships": {
        "white_rabbit": "pursuer",
        "cheshire_cat": "guide"
      }
    }
  ],
  "version": 1,
  "last_updated_chapter": "chapter_03"
}
```

### Scene List (`internal/model/scene.go`)

```json
{
  "$schema": "comix/scene-list/v1",
  "project_id": "alice_in_wonderland",
  "scenes": [
    {
      "id": "scene_001",
      "chapter": "chapter_01",
      "chapter_sequence": 1,
      "global_sequence": 1,
      "description": "Alice sits on a riverbank beside her sister, who is reading a book with no pictures. Alice fidgets, glances at the book, and sighs dramatically, clearly bored.",
      "characters_present": ["alice", "alices_sister"],
      "location": "riverside",
      "mood": "bored_restless",
      "visual_cues": ["sunny afternoon", "green grass", "wildflowers", "lazy river", "oak tree"],
      "panel_count": 1,
      "dialogue": [
        {
          "speaker": "alice",
          "text": "What is the use of a book without pictures or conversations?"
        }
      ]
    },
    {
      "id": "scene_002",
      "chapter": "chapter_01",
      "chapter_sequence": 2,
      "global_sequence": 2,
      "description": "A white rabbit with pink eyes runs past, frantically checking a pocket watch and exclaiming about being late. Alice's eyes widen with curiosity.",
      "characters_present": ["alice", "white_rabbit"],
      "location": "riverside",
      "mood": "sudden_excitement",
      "visual_cues": ["rabbit hole nearby", "pocket watch chain", "waistcoat", "panic"],
      "panel_count": 2,
      "dialogue": [
        {
          "speaker": "white_rabbit",
          "text": "Oh dear! Oh dear! I shall be too late!"
        }
      ]
    }
  ]
}
```

### Project Manifest (`internal/model/project.go`)

```yaml
# <output_dir>/project.yaml
project:
  name: alice_in_wonderland
  created_at: "2026-06-01T10:00:00Z"
  source:
    type: directory
    path: ./novels/alice/
  chapters:
    - id: chapter_01
      filename: chapter_01.md
      title: "Down the Rabbit-Hole"
      word_count: 2145
    - id: chapter_02
      filename: chapter_02.md
      title: "The Pool of Tears"
      word_count: 1897

pipeline:
  status: in_progress  # idle | in_progress | completed | failed
  current_phase: 4     # 0=ingest, 2=characters, 3=scenes, 4=sheets, 5=poses, 6=render
  phases:
    ingest:      { status: completed,   completed_at: "2026-06-01T10:01:00Z" }
    characters:  { status: completed,   completed_at: "2026-06-01T10:15:00Z" }
    scenes:      { status: completed,   completed_at: "2026-06-01T10:30:00Z" }
    sheets:      { status: in_progress, started_at:  "2026-06-01T10:31:00Z" }
    poses:       { status: pending }
    render:      { status: pending }
  errors: []
```

---

## Pipeline Architecture

### Phase 1 — Ingestion

**Input**: Markdown files (cover + chapters), delivered via CLI argument or HTTP upload.

**CLI**:
```
comix ingest \
  --cover ./novels/cover.md \
  --chapters ./novels/chapter_01.md,./novels/chapter_02.md \
  --project alice \
  --output ./comix-output
```

```
comix ingest \
  --book-dir ./novels/alice/ \
  --project alice \
  --output ./comix-output
```

**HTTP**: `POST /api/projects/:id/ingest` (multipart/form-data with file fields, or JSON body with paths)

**Logic**:
1. Discover files: explicit paths, or scan `--book-dir` for `cover.md` and `chapter_*.md` (sorted by filename)
2. Validate each file is valid Markdown (parse frontmatter if present)
3. Extract chapter title from filename or frontmatter `title:` field
4. Build chapter manifest with ordering
5. Copy raw markdown into `<output>/<project>/raw/`
6. Write initial `<output>/<project>/project.yaml` with manifest and pipeline status = `idle`

**Checkpoint**: `project.yaml` written after successful ingestion.

---

### Phase 2 — Pass One: Character Extraction

**Input**: Cover markdown + all chapter markdown files.

**Output**: Complete `characters.json` (CharacterNote).

**Algorithm**:
```
CharacterNote = empty
FOR EACH chapter in chapters (in order):
    messages = [
        SystemPrompt("extract_characters"),
        UserMessage("Cover:\n" + cover_md + "\n\nChapter:\n" + chapter_text),
        (IF CharacterNote not empty) UserMessage("Existing CharacterNote (return this COMPLETE with your updates appended):\n" + json(CharacterNote))
    ]
    response = OpenAI.ChatCompletions({
        model: "gpt-4o",
        messages: messages,
        response_format: { type: "json_object" },
        temperature: 0.1
    })
    CharacterNote = json.Unmarshal(response.choices[0].message.content)
    SaveCheckpoint(CharacterNote)
```

The LLM handles all merging itself. Each call receives the full existing CharacterNote and must return the **complete updated CharacterNote** — not just deltas. This means:
- New characters from the current chapter are appended to the `characters` array
- Existing characters get their `chapters_seen` extended, `physical_description` refined, and `personality_traits`/`notable_actions`/`relationships` augmented
- No character is ever duplicated or dropped
- `version` is incremented and `last_updated_chapter` reflects the current chapter

**System prompt** (`internal/llm/prompts.go`):
```
You are a literary analysis AI specializing in character extraction for comic adaptation.
Given the cover description, a chapter text, and an existing CharacterNote (if any),
extract or update all characters that appear. Return the COMPLETE CharacterNote —
every previously extracted character must still be present, with new characters
appended and existing ones augmented.

For each character, provide:
1. id — A unique lowercase_snake_case identifier (e.g., "white_rabbit")
2. name — The character's full name as used in the story
3. physical_description — A detailed physical description including age, build, hair, eyes, clothing, distinguishing features. Extract verbatim from text where possible, infer from context where needed.
4. personality_traits — An array of descriptive adjectives (3-8 traits)
5. aliases — Any alternative names or nicknames
6. notable_actions — Key actions this character takes
7. relationships — Dictionary mapping other character IDs to relationship type string

Rules:
- Return the FULL CharacterNote, not a diff. Never drop previously extracted characters.
- Append this chapter to chapters_seen for existing characters; refine descriptions if new details emerge.
- Set first_chapter for new characters; increment version and update last_updated_chapter.
- Never merge clearly different people into the same entry.
- Characters mentioned but not seen: physical_description = "mentioned only".
- Disambiguate same-name characters with a qualifier in the id.

Return valid JSON matching the CharacterNote schema exactly.
```

**Edge cases handled**:
- Characters mentioned but not physically present → include with `physical_description: "mentioned only"`
- Multiple characters with the same name → use context to disambiguate, append qualifier to ID
- Empty chapter with no characters → return empty array, preserve existing note
- API failure → retry with exponential backoff (1s, 2s, 4s, 8s, 16s max)

**Checkpoint**: `characters.json` updated after each chapter. On resume, skip chapters whose characters are already processed.

---

### Phase 3 — Pass Two: Scene Extraction

**Input**: Cover markdown + all chapter markdown files + complete CharacterNote.

**Output**: `scenes.json` (SceneList) with ordered scenes.

**Algorithm**:
```
FOR EACH chapter in chapters (in order):
    messages = [
        SystemPrompt("extract_scenes"),
        UserMessage("Cover: " + cover_md +
                    "\n\nChapter: " + chapter_text +
                    "\n\nCharacter Reference:\n" + json(CharacterNote))
    ]
    response = OpenAI.ChatCompletions({
        model: "gpt-4o",
        messages: messages,
        response_format: { type: "json_object" },
        temperature: 0.2
    })
    chapter_scenes = json.Unmarshal(response.choices[0].message.content)
    for scene in chapter_scenes:
        ValidateCharacterRefs(scene, CharacterNote)  # Warn if character ID not in note
        scene.global_sequence = ++counter
    AppendToSceneList(scene_list, chapter_scenes)
    SaveCheckpoint(scene_list)
```

**System prompt**:
```
You are a comic script writer adapting novels into sequential visual panels.
Given a chapter text and a complete character reference, break the chapter into
distinct visual scenes. Each scene should represent one beat or moment that would
occupy a single comic panel.

For each scene, provide:
1. id — Unique identifier (e.g., "scene_001")
2. description — A vivid visual description of what the panel shows. Include character positioning, expressions, environment details, and action. Write this for an image generation AI.
3. characters_present — Array of character IDs from the provided CharacterNote that appear in this scene
4. location — Where the scene takes place
5. mood — The emotional atmosphere (single word or hyphenated phrase)
6. visual_cues — Array of specific visual elements to include (lighting, weather, props, etc.)
7. dialogue — Array of {speaker, text} objects for any spoken lines in this scene

Rules:
- Every scene must be visually distinct from the previous one
- Attach the correct character IDs — do not invent characters not in the CharacterNote
- Keep descriptions concrete and visual (what the reader SEES, not what they think)
- Each scene should be 1-3 sentences of description
- Break at natural narrative beats (entrance, action, dialogue exchange, location change)

Return valid JSON matching the SceneList schema.
```

**Validation**: After extraction, validate every `characters_present` reference exists in CharacterNote. Log warnings for missing refs but don't fail — the image generator will receive the character description anyway.

**Checkpoint**: `scenes.json` updated after each chapter.

---

### Phase 4 — Base Model Sheets (3×2 Grid)

**Input**: CharacterNote with physical descriptions.

**Output**: One `sheets/<character_id>_3x2.png` per character.

**Strategy**: Generate the entire 3×2 grid as a single gpt-image-2 image.

**Prompt template**:
```
Create a character reference sheet for [character_name].

Physical description: [physical_description]

The sheet must be a strict 3x2 grid layout with exactly 6 panels:

Top row (left to right):
  Panel 1: FRONT view — character facing the viewer, neutral expression, standing straight
  Panel 2: BACK view — character seen from behind, showing back of hair and clothing
  Panel 3: RIGHT PROFILE — character facing the viewer's right, in profile

Bottom row (left to right):
  Panel 4: LEFT PROFILE — character facing the viewer's left, in profile
  Panel 5: TOP-DOWN / OVERHEAD — character seen from directly above
  Panel 6: BOTTOM-UP / LOW ANGLE — character seen from below, looking up

Requirements:
- Neutral pose throughout (standing, arms at sides, no action poses)
- Plain white or light gray background
- Consistent lighting across all 6 panels
- All 6 slots MUST be filled — no empty panels
- Grid lines or clear borders between each panel
- Full-body view in every panel
- Accurate to the physical description above
```

**Image params**:
```go
model: "gpt-image-2"
size: "1536x1024"     // 3:2 aspect ratio matches grid layout
quality: "high"       // maximum fidelity for reference sheets
thinking: "medium"    // ensure layout reasoning
n: 1
```

**Rate limiting**: gpt-image-2 Tier 1 = 5 images/min. For a book with 10 characters, this phase takes ~2 minutes. Run character sheets concurrently with semaphore limiting to 5 concurrent requests.

**Checkpoint**: Each sheet saved individually. `project.yaml` updated after all sheets complete.

---

### Phase 5 — Dynamic Pose Generation (5×5 Grid)

**Input**: 3×2 base reference sheets.

**Output**: One `poses/<character_id>_5x5.png` per character.

**Strategy**: Use gpt-image-2 edit endpoint with the 3×2 sheet as reference image.

```
POST /v1/images/edits
Content-Type: multipart/form-data

model: gpt-image-2
image: <3x2_sheet.png>
prompt: "Using this character reference sheet for style and appearance guidance, generate a massive 5x5 grid containing exactly 25 distinct dynamic action poses of this character. Each of the 25 slots must show a different pose: running, jumping, crouching, reaching upward, ducking, spinning, pointing, sitting, kneeling, leaping, crawling, stretching, dodging, balancing, throwing, catching, climbing, pushing, pulling, bowing, waving, hiding, flying/falling, standing heroically, and thinking/pondering. Every slot must be filled with a unique pose. Maintain strict visual consistency with the reference — same face, same clothing, same proportions. White background. Grid lines between panels."
size: "1536x1024"     // or custom 5:2 ratio: "2560x1024"
quality: "medium"
n: 1
```

**Checkpoint**: Each pose sheet saved individually.

---

### Phase 6 — Sequential Scene Rendering

**Input**: Scene list, pose sheets, character descriptions.

**Output**: Individual `panels/scene_<id>.png` files.

**Algorithm**:
```
previous_panel = nil
FOR EACH scene in scenes (in order of global_sequence):
    character_refs = BuildCharacterPromptText(scene.characters_present, CharacterNote, pose_sheets)
    
    IF previous_panel == nil:
        // First scene — text-to-image
        prompt = "Comic panel: " + scene.description +
                 "\n\nCharacters: " + character_refs +
                 "\n\nMood: " + scene.mood +
                 "\n\nVisual cues: " + join(scene.visual_cues)
        response = ImageGen.Generate(prompt, size, quality)
    ELSE:
        // Subsequent scenes — image-to-image edit from previous panel
        prompt = "Continue the visual story. Render the next comic panel based on this previous panel.\n\n" +
                 "New scene: " + scene.description +
                 "\n\nCharacters present: " + character_refs +
                 "\n\nIMPORTANT: Maintain visual continuity — same character appearances, same art style, consistent lighting direction, coherent spatial layout. The characters should look like the same individuals from the previous panel."
        response = ImageGen.Edit(previous_panel, prompt, size, quality)
    
    SavePanel(scene.id, response.image)
    previous_panel = response.image   // Feed back for next scene
```

**Character reference text builder**:
```
For each character in scene:
  - [character_name]: [physical_description]
    Reference pose sheet: poses/[character_id]_5x5.png
    Use the pose sheet for consistent appearance.
```

**Edge cases**:
- Scene with no characters → omit character section, generate from description alone
- Multiple scenes at same location → edit prompt includes "same location as previous panel, different moment"
- Character appears for first time → include full physical description in prompt
- API rate limit hit → queue remaining scenes, retry with backoff

**Image params for rendering**:
```go
model: "gpt-image-2"
size: "1024x1536"  // portrait — good for comic panels
quality: "medium"  // balance between quality and cost/speed
thinking: "medium" // layout reasoning for scene composition
```

**Checkpoint**: Each panel saved immediately. On resume, scenes with existing panel files are skipped.

---

## HTTP API Reference

### Endpoints

```
METHOD  PATH                                DESCRIPTION
------  ----                                -----------
POST    /api/projects                       Create a new project
POST    /api/projects/:id/ingest            Upload markdown files (multipart)
POST    /api/projects/:id/run               Execute full pipeline (phases 1-6)
POST    /api/projects/:id/run/characters    Run Phase 2 only
POST    /api/projects/:id/run/scenes        Run Phase 3 only
POST    /api/projects/:id/run/sheets        Run Phase 4 only
POST    /api/projects/:id/run/poses         Run Phase 5 only
POST    /api/projects/:id/run/render        Run Phase 6 only
GET     /api/projects/:id/status            Current pipeline status and progress
GET     /api/projects/:id/output            List all output artifacts
GET     /api/projects/:id/output/:path      Download specific file
DELETE  /api/projects/:id                   Delete project and all artifacts
GET     /api/projects                       List all projects
GET     /api/health                         Health check
```

### Response Format

All responses follow a consistent envelope:

```json
{
  "success": true,
  "data": { ... },
  "error": null,
  "meta": {
    "request_id": "req_abc123",
    "timestamp": "2026-06-01T10:00:00Z"
  }
}
```

Error responses:

```json
{
  "success": false,
  "data": null,
  "error": {
    "code": "INVALID_CHAPTER_REFERENCE",
    "message": "Scene scene_005 references character 'mad_hatter' not found in CharacterNote",
    "details": { "scene_id": "scene_005", "missing_character": "mad_hatter" }
  },
  "meta": { "request_id": "req_def456", "timestamp": "2026-06-01T10:00:00Z" }
}
```

### Key Request Bodies

**Create project**:
```json
{
  "name": "alice_in_wonderland",
  "source_type": "directory",
  "source_path": "/path/to/novels/alice/"
}
```

**Ingest files** (multipart/form-data):
```
Content-Type: multipart/form-data

Fields:
  cover:       (file) cover.md
  chapters[]:  (files) chapter_01.md, chapter_02.md, ...
  -- OR --
  file_count:  3
  files:       (files array)
```

**Run pipeline**:
```json
{
  "phases": ["ingest", "characters", "scenes", "sheets", "poses", "render"],
  "resume": false,
  "force": false
}
```

---

## CLI Reference

### Global Flags

```
--config <path>     Config file path (default: ./config.yaml)
--output <dir>      Output directory (default: ./comix-output)
--verbose           Enable verbose logging
--log-format        text | json (default: text)
```

### Commands

```bash
# Run entire pipeline
comix run \
  --book-dir ./novels/alice/ \
  --project alice \
  --output ./comix-output

# Run with explicit file paths
comix run \
  --cover ./novels/alice/cover.md \
  --chapters ./novels/alice/chapter_01.md,./novels/alice/chapter_02.md \
  --project alice \
  --output ./comix-output

# Resume from last checkpoint
comix run --book-dir ./novels/alice/ --project alice --resume

# Start HTTP server
comix serve --port 8080 --host 0.0.0.0

# Individual phases
comix ingest --book-dir ./novels/alice/ --project alice
comix extract characters --project alice
comix extract scenes --project alice
comix generate sheets --project alice
comix generate poses --project alice
comix render --project alice

# List projects
comix list

# Show project status
comix status --project alice
```

---

## Configuration

### `config.yaml`

```yaml
# Comix Configuration

openai:
  api_key: "${OPENAI_API_KEY}"          # Reads from env var (recommended)
  # OR set directly:
  # api_key: "sk-..."                   # Not recommended for production

  llm:
    model: "gpt-4o"                     # Model for character/scene extraction
    temperature: 0.1                    # Lower = more consistent extraction
    max_retries: 5                      # Retry on API failure
    retry_base_delay: 1s                # Exponential backoff starting delay

  image:
    model: "gpt-image-2"                # Image generation model
    quality: "medium"                   # low | medium | high
    size: "1024x1024"                   # Default size
    thinking: "medium"                  # off | low | medium | high
    max_retries: 5
    retry_base_delay: 2s
    rate_limit_rpm: 5                   # Requests per minute (Tier 1 default)

pipeline:
  output_dir: "./comix-output"          # All artifacts root
  chapter_pattern: "chapter_*.md"       # Glob for chapter discovery
  cover_filename: "cover.md"            # Cover markdown filename
  max_concurrent_sheets: 3              # Parallel sheet generation
  max_concurrent_poses: 2               # Parallel pose generation

server:
  host: "0.0.0.0"                      # Listen address
  port: 8080                           # Listen port
  read_timeout: 30s                    # HTTP read timeout
  write_timeout: 60s                   # HTTP write timeout
  shutdown_timeout: 15s                # Graceful shutdown deadline

logging:
  level: "info"                        # debug | info | warn | error
  format: "text"                       # text | json
```

### Environment Variables

```bash
# .env
OPENAI_API_KEY=sk-...                  # Required
COMIX_CONFIG=./config.yaml             # Config file path
COMIX_OUTPUT=./comix-output            # Output directory
COMIX_LOG_LEVEL=info                   # Log level
COMIX_LOG_FORMAT=text                  # Log format
```

CLI flags > Env vars > Config file > Defaults (Viper precedence).

---

## Output Directory Layout

```
comix-output/
└── <project_name>/
    ├── project.yaml                   # Project manifest + pipeline progress
    ├── raw/
    │   ├── cover.md
    │   ├── chapter_01.md
    │   ├── chapter_02.md
    │   └── ...
    ├── state/
    │   ├── characters.json            # Phase 2 output — CharacterNote
    │   └── scenes.json                # Phase 3 output — SceneList
    ├── sheets/
    │   ├── alice_3x2.png
    │   ├── white_rabbit_3x2.png
    │   ├── cheshire_cat_3x2.png
    │   └── ...
    ├── poses/
    │   ├── alice_5x5.png
    │   ├── white_rabbit_5x5.png
    │   ├── cheshire_cat_5x5.png
    │   └── ...
    └── panels/
        ├── scene_001.png
        ├── scene_002.png
        ├── scene_003.png
        └── ...
```

---

## Implementation Phases

### Phase 0 — Project Scaffold

**Goal**: Working `go build` with config loading, empty CLI skeleton.

**Files**:
- `go.mod` — Module init with `github.com/yourusername/comix`
- `cmd/comix/main.go` — Parse `--mode` flag, dispatch to server or CLI
- `internal/config/config.go` — Viper config struct with all fields, LoadConfig() function
- `cli/root.go` — Root cobra command with global flags
- `cli/serve.go` — `serve` subcommand (starts placeholder server)
- `cli/run.go` — `run` subcommand (starts placeholder pipeline)
- `config.yaml` — Default config with all sections
- `.env.example` — Template env file
- `Makefile` — `build`, `test`, `lint`, `clean`, `run` targets

**Verification**: `make build` produces a binary. `./comix` shows help. `./comix serve` starts a server. `./comix run --help` shows flags.

---

### Phase 1 — Data Models & Storage

**Goal**: All data types defined, file I/O working, no pipeline logic yet.

**Files**:
- `internal/model/character.go` — `Character`, `CharacterNote` structs with JSON tags, validation methods (Validate())
- `internal/model/scene.go` — `Scene`, `SceneList`, `DialogueLine` structs, validation methods
- `internal/model/project.go` — `ProjectManifest`, `ChapterMeta`, `PhaseStatus`, `PipelineProgress` structs
- `internal/storage/storage.go` — `ReadMarkdown(path)`, `SaveJSON(path, data)`, `LoadJSON(path, dest)`, `SavePNG(path, img)`, `EnsureDir(path)`, `DirectoryExists(path)`
- `internal/storage/paths.go` — Output path generation functions: `ProjectDir(root, project)`, `RawDir(root, project)`, `StateDir(root, project)`, `SheetsDir(root, project)`, `PosesDir(root, project)`, `PanelsDir(root, project)`, `CharactersPath(root, project)`, `ScenesPath(root, project)`, `ManifestPath(root, project)`

**Verification**: Unit tests that create temp directories, write/read JSON models, validate structs. `go test ./...` passes.

---

### Phase 2 — LLM Client

**Goal**: OpenAI Chat Completions integration with JSON mode, retries, error handling.

**Files**:
- `internal/llm/client.go`:
  - `NewClient(apiKey, model string) *Client`
  - `(*Client).Chat(messages []Message, schema any) error` — Sends chat request with `response_format: {type: "json_object"}`, unmarshals into provided schema pointer
  - Retry with exponential backoff on 429, 500, 503
  - Context cancellation support
- `internal/llm/prompts.go`:
  - `SystemPromptExtractCharacters()` — Returns the character extraction system prompt
  - `SystemPromptExtractScenes()` — Returns the scene extraction system prompt

**Verification**: Integration test with a real API key (optional, gated behind build tag `integration`). Unit tests for retry logic and JSON parsing. Mock HTTP server for unit tests.

---

### Phase 3 — Image Generation Client

**Goal**: gpt-image-2 text-to-image and image-to-image editing integration.

**Files**:
- `internal/imagegen/client.go`:
  - `NewClient(apiKey, model string, quality, size, thinking string) *Client`
  - `(*Client).Generate(prompt string) (*ImageResult, error)` — Text-to-image via `POST /v1/images/generations`
  - `(*Client).Edit(inputImage image.Image, prompt string) (*ImageResult, error)` — Image-to-image via `POST /v1/images/edits`
  - `ImageResult` struct with `Image image.Image`, `RevisedPrompt string`, `Usage TokenUsage`
  - Rate-limit aware semaphore (tokens/minute)
  - Retry with backoff
- `internal/imagegen/prompts.go`:
  - `PromptBaseSheet(charName, physicalDesc string) string`
  - `PromptPoseSheet(charName string) string`
  - `PromptFirstScene(scene SceneDesc, charRefs string) string`
  - `PromptNextScene(prevDesc string, scene SceneDesc, charRefs string) string`

**Verification**: Unit test grid compositing with synthetic images. Integration test against gpt-image-2 (optional). Mock HTTP server for rate-limit and retry tests.

---

### Phase 4 — Pipeline Core (Phases 1-3)

**Goal**: Ingestion, character extraction, and scene extraction working end-to-end.

**Files**:
- `internal/pipeline/pipeline.go`:
  - `Pipeline` struct with config, client references, state manager
  - `(*Pipeline).Run(ctx, manifest, phases []string, resume bool) error` — State machine that dispatches to phase implementations
  - Progress tracking and error collection
  - Checkpoint save/load for resume
- `internal/pipeline/ingest.go`:
  - `(*Pipeline).Ingest(ctx, source IngestSource) (*ProjectManifest, error)` — Phase 1
  - Discovers markdown files, validates, copies to output dir
- `internal/pipeline/characters.go`:
  - `(*Pipeline).ExtractCharacters(ctx, manifest, resume bool) (*CharacterNote, error)` — Phase 2
  - Iterates chapters, calls LLM, merges, checkpoints
- `internal/pipeline/scenes.go`:
  - `(*Pipeline).ExtractScenes(ctx, manifest, note, resume bool) (*SceneList, error)` — Phase 3
  - Iterates chapters, calls LLM, validates character refs, checkpoints
- `internal/state/state.go`:
  - `LoadCharacterNote(root, project string) (*CharacterNote, error)`
  - `SaveCharacterNote(root, project string, note *CharacterNote) error`
  - `LoadSceneList(root, project string) (*SceneList, error)`
  - `SaveSceneList(root, project string, scenes *SceneList) error`
  - `UpdateManifestPhase(root, project, phase string, status PhaseStatus) error`

**Verification**: `comix run --book-dir ./testdata/alice/ --project test --output /tmp/comix-test`. Check output dir for raw/, state/ with characters.json and scenes.json.

---

### Phase 5 — Image Generation Pipeline (Phases 4-6)

**Goal**: Full pipeline with image generation working.

**Files**:
- `internal/pipeline/sheets.go`:
  - `(*Pipeline).GenerateSheets(ctx, manifest, note) error` — Phase 4
  - For each character, call imagegen to generate 3×2 sheet in a single gpt-image-2 call
  - Parallel with semaphore limiting concurrency
- `internal/pipeline/poses.go`:
  - `(*Pipeline).GeneratePoses(ctx, manifest, note) error` — Phase 5
  - For each character, load 3×2 sheet, use as reference via gpt-image-2 edit for 5×5 generation
  - Single call per character — gpt-image-2 produces the full 25-pose grid
- `internal/pipeline/render.go`:
  - `(*Pipeline).RenderScenes(ctx, manifest, note, scenes) error` — Phase 6
  - Iterate scenes in order, generate/edit based on previous panel
  - Save each panel immediately

**Verification**: Run full pipeline on a small novel (2 chapters, 3 characters). Verify output dir contains all 3×2 sheets, 5×5 pose sheets, and panel PNGs.

---

### Phase 6 — HTTP Server

**Goal**: Full REST API for project management and pipeline execution.

**Files**:
- `internal/server/server.go`:
  - `NewServer(cfg, pipeline) *Server` — Creates chi router, registers middleware and routes
  - `(*Server).Start() error` — Listens and serves HTTP, handles graceful shutdown
- `internal/server/handlers.go`:
  - `handleCreateProject` — `POST /api/projects`
  - `handleIngest` — `POST /api/projects/:id/ingest` (multipart file upload)
  - `handleRunPipeline` — `POST /api/projects/:id/run` (async)
  - `handleRunPhase` — `POST /api/projects/:id/run/:phase` (async)
  - `handleGetStatus` — `GET /api/projects/:id/status`
  - `handleListOutputs` — `GET /api/projects/:id/output`
  - `handleGetOutput` — `GET /api/projects/:id/output/:path`
  - `handleDeleteProject` — `DELETE /api/projects/:id`
  - `handleListProjects` — `GET /api/projects`
  - `handleHealth` — `GET /api/health`
- `internal/server/middleware.go`:
  - `RequestLogger` — Structured request logging
  - `CORS` — Configurable CORS headers
  - `RequestID` — UUID per request
  - `Recoverer` — Panic recovery

**Async execution**: Long-running pipeline operations start a goroutine and return immediately with status `{"status": "started", "project_id": "..."}`. The client polls `GET /api/projects/:id/status` until `status` becomes `completed` or `failed`.

**Verification**: Start server, create project via curl, upload files via multipart, run pipeline, poll status, download output.

---

### Phase 7 — CLI Completion

**Goal**: All CLI subcommands fully functional.

**Files**:
- `internal/cli/ingest.go` — `comix ingest --book-dir|--cover+--chapters --project --output`
- `internal/cli/extract.go` — `comix extract characters|scenes --project`
- `internal/cli/generate.go` — `comix generate sheets|poses --project`
- `internal/cli/render.go` — `comix render --project`
- `internal/cli/root.go` — Extend with `list`, `status` commands

All subcommands construct a `pipeline.Pipeline` and call the appropriate method, streaming progress to stdout.

**Verification**: Every CLI command produces correct output. `comix run` completes end-to-end.

---

### Phase 8 — Polish & Hardening

**Goal**: Production readiness.

**Activities**:
- Comprehensive error messages with actionable remediation
- `--verbose` mode with structured debug logging (JSON format for log aggregation)
- Graceful SIGINT/SIGTERM handling — save checkpoint on interrupt, allow resume
- Config validation on startup (required fields, valid ranges, API key format check)
- Linting: `golangci-lint` with `go vet`, `staticcheck`
- Documentation: README with quickstart, full examples, troubleshooting
- `.goreleaser.yaml` for multi-platform builds (linux/darwin/windows, amd64/arm64)
- `Makefile` targets: `build`, `test`, `lint`, `clean`, `release`, `cross-build`

---

### Phase 9 — Testing

**Goal**: Reliable, maintainable codebase.

**Test categories**:

| Category | Scope | Tools |
|---|---|---|
| Unit tests | All internal packages, mock external APIs | `testing` stdlib, `httptest` |
| Integration | LLM + image gen (gated behind build tags) | `go test -tags=integration`, real API key |
| End-to-end | Full pipeline with test data | Small known novel, deterministic output check |

| State persistence | Save/load/resume across all phases | Temp directory, compare loaded state to original |
| Rate limiting | Semaphore behavior, backoff | Mock slow HTTP server, verify call timing |

**Test data**: A small test novel (`testdata/mini/`) with:
- `cover.md` — 3 paragraphs
- `chapter_01.md` — ~500 words, 2 characters, 3 scenes
- `chapter_02.md` — ~500 words, 1 new character, 2 returning characters, 4 scenes

---

## Error Handling Strategy

| Error Category | Handling | User Impact |
|---|---|---|
| OpenAI API 429 (rate limit) | Exponential backoff, semaphore waiting | Transient delay |
| OpenAI API 500/503 (server error) | Retry up to 5x | Brief delay, auto-recovered |
| OpenAI API 400 (invalid request) | Return error with prompt debug info | User must fix config/prompt |
| Invalid markdown (Phase 1) | Return error with file + line info | User fixes markdown |
| Character ref not found (Phase 3) | Warning in log, continue with best effort | Minor quality impact |
| Image generation empty/malformed | Retry 3x with different seed | Brief delay or warning |
| Disk full / write error | Return error with path + disk usage | User frees space |
| Context cancelled (SIGINT) | Save checkpoint, return partial results | User resumes later |

All errors are written to `project.yaml` under `pipeline.errors[]` with `phase`, `timestamp`, `message`, and `recoverable` flags.

---

## Rate Limit & Cost Management

### gpt-image-2 Tiers

| Tier | Requirement | Images/min |
|---|---|---|
| 1 | Default | 5 |
| 2 | $50 spent | 20 |
| 3 | $100 spent, 7d account age | 50 |
| 4 | $250 spent, 14d account age | 100 |
| 5 | $1,000 spent, 30d account age | 250 |

### Cost Estimation (1024×1024)

| Quality | Cost/image | 10 characters × 1 sheet | 10 characters × 1 pose | 50 scenes | Total |
|---|---|---|---|---|---|
| Low | $0.006 | $0.06 | $0.06 | $0.30 | $0.42 |
| Medium | $0.053 | $0.53 | $0.53 | $2.65 | $3.71 |
| High | $0.211 | $2.11 | $2.11 | $10.55 | $14.77 |

### LLM Cost Estimation

| Phase | Tokens/call | Calls | Cost (GPT-4o) |
|---|---|---|---|
| Character extraction | ~4K input, ~1K output | 10 chapters | ~$0.25 |
| Scene extraction | ~6K input, ~2K output | 10 chapters | ~$0.50 |

### Optimization Strategies

- Use `quality: "low"` for initial runs, `medium` for final renders
- Use `config.yaml` `image.quality` setting to control cost globally
- Rate limit semaphore prevents 429 errors — configurable via `image.rate_limit_rpm`
- Resume support prevents re-processing completed phases
- Parallel sheet/pose generation with concurrency limited by semaphore

---

## Design Decisions Log

| # | Decision | Rationale | Date |
|---|---|---|---|
| 1 | Go over Rust/Python | Single static binary; best stdlib for HTTP/JSON/image; goroutines for pipeline concurrency | 2026-06-01 |
| 2 | File-based state (JSON+YAML) over DB | User requested pure files; human-readable; no infra; git-friendly | 2026-06-01 |
| 3 | gpt-image-2 for all image tasks | Single model for text-to-image + img2img + editing; strong layout reasoning; high-fidelity input preservation | 2026-06-01 |
| 4 | Previous-panel-as-input for continuity | gpt-image-2's high-fidelity edit preserves character appearance, lighting, and spatial layout across scenes | 2026-06-01 |
| 5 | Single-image grids via gpt-image-2 (no fallback compositing) | gpt-image-2 produces 3×2 and 5×5 grid layouts directly; thinking mode ensures correct spatial arrangement | 2026-06-01 |
| 6 | Cobra + Viper for CLI/config | Industry standard; flexible config merging; composable subcommands | 2026-06-01 |
| 7 | chi router over stdlib mux | Middleware chaining; route params; context-based handlers; used in production | 2026-06-01 |
| 8 | Async HTTP pipeline execution | Long-running image generation (potentially hours); polling pattern is simple and reliable | 2026-06-01 |
