package pipeline

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"

	"github.com/FarelRA/comix/internal/model"
	"github.com/FarelRA/comix/internal/state"
	"github.com/FarelRA/comix/internal/storage"
)

type discoveredChapter struct {
	ID        string
	Filename  string
	Title     string
	Content   string
	WordCount int
}

func (p *Pipeline) Ingest(ctx context.Context, source IngestSource) (*model.ProjectManifest, error) {
	if source.ProjectName != "" {
		if err := storage.ValidateName(source.ProjectName); err != nil {
			return nil, fmt.Errorf("validating project name: %w", err)
		}
	}
	chapters, coverContent, err := p.discoverFiles(source)
	if err != nil {
		return nil, fmt.Errorf("discovering files: %w", err)
	}

	if len(chapters) == 0 {
		return nil, fmt.Errorf("no chapter files found")
	}

	manifest, err := p.buildManifest(source, chapters)
	if err != nil {
		return nil, fmt.Errorf("building manifest: %w", err)
	}

	outputDir := p.cfg.Pipeline.OutputDir
	projectName := manifest.Project.Name
	exists, err := state.ManifestExists(outputDir, projectName)
	if err != nil {
		return nil, fmt.Errorf("checking existing project: %w", err)
	}
	if exists && !source.AllowExisting {
		return nil, fmt.Errorf("project %q already exists; use --resume to continue an existing project", projectName)
	}

	rawDir := storage.RawDir(outputDir, projectName)
	if err := storage.EnsureDir(rawDir); err != nil {
		return nil, fmt.Errorf("creating raw dir: %w", err)
	}

	if coverContent != "" {
		coverPath := filepath.Join(rawDir, p.cfg.Pipeline.CoverFilename)
		if err := os.WriteFile(coverPath, []byte(coverContent), 0644); err != nil {
			return nil, fmt.Errorf("writing cover file: %w", err)
		}
	}

	for _, ch := range chapters {
		chPath := filepath.Join(rawDir, ch.Filename)
		if err := os.WriteFile(chPath, []byte(ch.Content), 0644); err != nil {
			return nil, fmt.Errorf("writing chapter %s: %w", ch.ID, err)
		}
	}

	if err := state.SaveManifest(outputDir, projectName, manifest); err != nil {
		return nil, fmt.Errorf("saving manifest: %w", err)
	}

	return manifest, nil
}

func (p *Pipeline) discoverFiles(source IngestSource) ([]discoveredChapter, string, error) {
	if source.BookDir != "" {
		return p.discoverFromDir(source.BookDir)
	}
	return p.discoverFromExplicit(source.Cover, source.Chapters)
}

func (p *Pipeline) discoverFromDir(bookDir string) ([]discoveredChapter, string, error) {
	info, err := os.Stat(bookDir)
	if err != nil {
		return nil, "", fmt.Errorf("accessing book directory %q: %w", bookDir, err)
	}
	if !info.IsDir() {
		return nil, "", fmt.Errorf("%q is not a directory", bookDir)
	}

	coverFilename := p.cfg.Pipeline.CoverFilename
	chapterPattern := p.cfg.Pipeline.ChapterPattern

	coverPath := filepath.Join(bookDir, coverFilename)
	var coverContent string
	if _, err := os.Stat(coverPath); err == nil {
		content, err := storage.ReadMarkdown(coverPath)
		if err != nil {
			return nil, "", fmt.Errorf("reading cover file: %w", err)
		}
		coverContent = content
	}

	globPattern := filepath.Join(bookDir, chapterPattern)
	matches, err := filepath.Glob(globPattern)
	if err != nil {
		return nil, "", fmt.Errorf("globbing chapters with pattern %q: %w", globPattern, err)
	}

	if len(matches) == 0 {
		return nil, "", fmt.Errorf("no chapters found matching %q in %q", chapterPattern, bookDir)
	}

	sort.Strings(matches)

	var chapters []discoveredChapter
	for _, match := range matches {
		content, err := storage.ReadMarkdown(match)
		if err != nil {
			return nil, "", fmt.Errorf("reading %s: %w", match, err)
		}

		filename := filepath.Base(match)
		id := storage.SlugName(chapterIDFromFilename(filename))
		title := chapterTitleFromContent(content, id)

		chapters = append(chapters, discoveredChapter{
			ID:        id,
			Filename:  filename,
			Title:     title,
			Content:   content,
			WordCount: wordCount(content),
		})
	}

	return chapters, coverContent, nil
}

func (p *Pipeline) discoverFromExplicit(coverPath string, chapterPaths []string) ([]discoveredChapter, string, error) {
	var coverContent string
	if coverPath != "" {
		content, err := storage.ReadMarkdown(coverPath)
		if err != nil {
			return nil, "", fmt.Errorf("reading cover file %q: %w", coverPath, err)
		}
		coverContent = content
	}

	if len(chapterPaths) == 0 {
		return nil, "", fmt.Errorf("no chapter files provided")
	}

	var chapters []discoveredChapter
	for _, chPath := range chapterPaths {
		chPath = strings.TrimSpace(chPath)
		if chPath == "" {
			continue
		}

		content, err := storage.ReadMarkdown(chPath)
		if err != nil {
			return nil, "", fmt.Errorf("reading %q: %w", chPath, err)
		}

		filename := filepath.Base(chPath)
		id := storage.SlugName(chapterIDFromFilename(filename))
		title := chapterTitleFromContent(content, id)

		chapters = append(chapters, discoveredChapter{
			ID:        id,
			Filename:  filename,
			Title:     title,
			Content:   content,
			WordCount: wordCount(content),
		})
	}

	return chapters, coverContent, nil
}

func (p *Pipeline) buildManifest(source IngestSource, chapters []discoveredChapter) (*model.ProjectManifest, error) {
	var sourceType, sourcePath string
	switch {
	case source.BookDir != "":
		sourceType = "directory"
		sourcePath = source.BookDir
	default:
		sourceType = "explicit"
		sourcePath = ""
	}

	projectName := p.extractProjectName(source, chapters)

	var chapterMetas []model.ChapterMeta
	for _, ch := range chapters {
		if ch.ID == "" {
			return nil, fmt.Errorf("chapter file %q has empty ID", ch.Filename)
		}
		chapterMetas = append(chapterMetas, model.ChapterMeta{
			ID:        ch.ID,
			Filename:  ch.Filename,
			Title:     ch.Title,
			WordCount: ch.WordCount,
		})
	}

	manifest := model.NewProjectManifest(projectName, sourceType, sourcePath, chapterMetas)
	return manifest, nil
}

func (p *Pipeline) extractProjectName(source IngestSource, chapters []discoveredChapter) string {
	if source.ProjectName != "" {
		return source.ProjectName
	}
	if source.BookDir != "" {
		return filepath.Base(source.BookDir)
	}
	return "project"
}

func chapterIDFromFilename(filename string) string {
	ext := filepath.Ext(filename)
	base := strings.TrimSuffix(filename, ext)
	base = strings.ReplaceAll(base, " ", "_")
	base = strings.ToLower(base)
	return base
}

func chapterTitleFromContent(content string, fallbackID string) string {
	source := []byte(content)
	doc := goldmark.DefaultParser().Parse(text.NewReader(source))

	var title string
	ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if heading, ok := n.(*ast.Heading); ok && heading.Level == 1 && title == "" {
			lines := n.Lines()
			if lines.Len() > 0 {
				seg := lines.At(0)
				title = string(seg.Value(source))
			}
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})

	if title != "" {
		return strings.TrimSpace(title)
	}

	lines := strings.SplitN(content, "\n", 3)
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "title:") {
			t := strings.TrimSpace(strings.TrimPrefix(trimmed, "title:"))
			t = strings.Trim(t, "\"'")
			if t != "" {
				return t
			}
		}
	}

	return fallbackID
}

func wordCount(s string) int {
	words := strings.Fields(s)
	return len(words)
}
