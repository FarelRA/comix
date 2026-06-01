package imagegen

import "fmt"

func PromptBaseSheet(charName, physicalDesc string) string {
	return fmt.Sprintf(`Create a character reference sheet for %s.

Physical description: %s

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
- Accurate to the physical description above`, charName, physicalDesc)
}

func PromptPoseSheet(charName string) string {
	return fmt.Sprintf(`Using this character reference sheet for style and appearance guidance, generate a massive 5x5 grid containing exactly 25 distinct dynamic action poses of %s. Each of the 25 slots must show a different pose: running, jumping, crouching, reaching upward, ducking, spinning, pointing, sitting, kneeling, leaping, crawling, stretching, dodging, balancing, throwing, catching, climbing, pushing, pulling, bowing, waving, hiding, flying/falling, standing heroically, and thinking/pondering. Every slot must be filled with a unique pose. Maintain strict visual consistency with the reference — same face, same clothing, same proportions. White background. Grid lines between panels.`, charName)
}

func PromptFirstScene(sceneDesc, charRefs string) string {
	return fmt.Sprintf(`Comic panel: %s

Characters: %s

IMPORTANT: Render this as a comic panel with clear visual storytelling.`, sceneDesc, charRefs)
}

func PromptNextScene(prevDesc string, sceneDesc, charRefs string) string {
	return fmt.Sprintf(`Continue the visual story. Render the next comic panel based on this previous panel.

Previous panel: %s

New scene: %s

Characters present: %s

IMPORTANT: Maintain visual continuity — same character appearances, same art style, consistent lighting direction, coherent spatial layout. The characters should look like the same individuals from the previous panel.`, prevDesc, sceneDesc, charRefs)
}
