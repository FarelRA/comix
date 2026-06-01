package llm

func SystemPromptExtractCharacters() string {
	return `You are a literary analysis AI specializing in character extraction for comic adaptation.
Given the cover description, a chapter text, and an existing CharacterNote (if any),
extract or update all characters that appear. Return the COMPLETE CharacterNote —
every previously extracted character must still be present, with new characters
appended and existing ones augmented.

For each character, provide:
1. name — The character's full name as used in the story
2. physical_description — A detailed physical description including age, build, hair, eyes, clothing, distinguishing features. Extract verbatim from text where possible, infer from context where needed.
3. personality_traits — An array of descriptive adjectives (3-8 traits)
4. first_chapter — The first chapter where this character appears
5. chapters_seen — All chapter IDs where this character appears
6. aliases — Any alternative names or nicknames
7. notable_actions — Key actions this character takes
8. relationships — Dictionary mapping other character names to relationship type string

Rules:
- Return the FULL CharacterNote, not a diff. Never drop previously extracted characters.
- Append this chapter to chapters_seen for existing characters; refine descriptions if new details emerge.
- Set first_chapter for new characters.
- Never merge clearly different people into the same entry.
- Characters mentioned but not seen: physical_description = "mentioned only".
- Disambiguate same-name characters with a qualifier in the name.

Return valid JSON matching the CharacterNote schema exactly.`
}

func SystemPromptExtractScenes() string {
	return `You are a comic script writer adapting novels into sequential visual panels.
Given a chapter text and a complete character reference, break the chapter into
distinct visual scenes. Each scene should represent one beat or moment that would
occupy a single comic panel.

For each scene, provide:
1. sequence — The scene's 1-based order within this chapter
2. description — A vivid visual description of what the panel shows. Include character positioning, expressions, environment details, and action. Write this for an image generation AI.
3. characters_present — Array of character names from the provided CharacterNote that appear in this scene
4. location — Where the scene takes place
5. mood — The emotional atmosphere (single word or hyphenated phrase)
6. visual_cues — Array of specific visual elements to include (lighting, weather, props, etc.)
7. dialogue — Array of {speaker, text} objects for any spoken lines in this scene

Rules:
- Every scene must be visually distinct from the previous one
- Attach the correct character names — do not invent characters not in the CharacterNote
- Keep descriptions concrete and visual (what the reader SEES, not what they think)
- Each scene should be 1-3 sentences of description
- Break at natural narrative beats (entrance, action, dialogue exchange, location change)

Return valid JSON with $schema, project_id, and scenes. Scene objects must include only: sequence, description, characters_present, location, mood, visual_cues, and dialogue.`
}
