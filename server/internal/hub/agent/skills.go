package agent

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// SkillDescriptor describes one discovered skill for an agent provider.
type SkillDescriptor struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// ListProviderSkills returns discovered skills for a provider name in cwd context.
func ListProviderSkills(ctx context.Context, providerName, cwd string) ([]SkillDescriptor, error) {
	preset, ok := providerPresetByName(providerName)
	if !ok {
		return nil, nil
	}
	return listSkillsForPreset(ctx, preset, cwd)
}

func providerPresetByName(name string) (ACPProviderPreset, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case CodexACPProviderPreset.Name:
		return CodexACPProviderPreset, true
	case ClaudeACPProviderPreset.Name:
		return ClaudeACPProviderPreset, true
	case CopilotACPProviderPreset.Name:
		return CopilotACPProviderPreset, true
	case CodeflickerACPProviderPreset.Name:
		return CodeflickerACPProviderPreset, true
	case OpenCodeACPProviderPreset.Name:
		return OpenCodeACPProviderPreset, true
	case CodeBuddyACPProviderPreset.Name:
		return CodeBuddyACPProviderPreset, true
	default:
		return ACPProviderPreset{}, false
	}
}

func listSkillsForPreset(_ context.Context, preset ACPProviderPreset, cwd string) ([]SkillDescriptor, error) {
	roots := skillScanRoots(preset, cwd)
	if len(roots) == 0 {
		return nil, nil
	}

	seenPaths := map[string]SkillDescriptor{}
	for _, root := range roots {
		_ = walkSkillRoot(root, func(skill SkillDescriptor) {
			key := strings.ToLower(strings.TrimSpace(skill.Path))
			if key == "" {
				return
			}
			if _, exists := seenPaths[key]; exists {
				return
			}
			seenPaths[key] = skill
		})
	}

	if len(seenPaths) == 0 {
		return nil, nil
	}

	out := make([]SkillDescriptor, 0, len(seenPaths))
	for _, skill := range seenPaths {
		out = append(out, skill)
	}
	sort.Slice(out, func(i, j int) bool {
		left := strings.ToLower(strings.TrimSpace(out[i].Name))
		right := strings.ToLower(strings.TrimSpace(out[j].Name))
		if left == right {
			return out[i].Path < out[j].Path
		}
		return left < right
	})
	return out, nil
}

func skillScanRoots(preset ACPProviderPreset, cwd string) []string {
	roots := make([]string, 0, 16)
	seen := map[string]struct{}{}
	appendUnique := func(path string) {
		path = strings.TrimSpace(path)
		if path == "" {
			return
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			return
		}
		key := strings.ToLower(abs)
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		roots = append(roots, abs)
	}

	cwd = strings.TrimSpace(cwd)
	if cwd != "" {
		for _, dir := range preset.SkillProjectDirs {
			appendUnique(filepath.Join(cwd, filepath.FromSlash(strings.TrimSpace(dir))))
		}
		if len(preset.SkillProjectParentDirs) > 0 {
			for _, parent := range parentDirs(cwd) {
				for _, dir := range preset.SkillProjectParentDirs {
					appendUnique(filepath.Join(parent, filepath.FromSlash(strings.TrimSpace(dir))))
				}
			}
		}
	}

	for _, dir := range preset.SkillUserDirs {
		appendUnique(expandHomePath(dir))
	}
	if preset.SkillExtraDirsEnv != "" {
		for _, extra := range splitSkillPathList(os.Getenv(preset.SkillExtraDirsEnv)) {
			appendUnique(expandHomePath(extra))
		}
	}
	for _, pattern := range preset.SkillPluginDirGlobs {
		for _, matched := range expandSkillGlob(pattern) {
			appendUnique(matched)
		}
	}
	return roots
}

func parentDirs(path string) []string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil
	}
	out := make([]string, 0, 8)
	for {
		next := filepath.Dir(abs)
		if next == abs {
			break
		}
		out = append(out, next)
		abs = next
	}
	return out
}

func expandHomePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, "~\\") {
		home, err := os.UserHomeDir()
		if err != nil || strings.TrimSpace(home) == "" {
			return ""
		}
		rest := strings.TrimPrefix(strings.TrimPrefix(path, "~/"), "~\\")
		return filepath.Join(home, filepath.FromSlash(strings.ReplaceAll(rest, "\\", "/")))
	}
	if path == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		return home
	}
	return path
}

func splitSkillPathList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == rune(os.PathListSeparator)
	})
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func expandSkillGlob(pattern string) []string {
	pattern = expandHomePath(pattern)
	if pattern == "" {
		return nil
	}
	matches, err := filepath.Glob(filepath.FromSlash(pattern))
	if err != nil {
		return nil
	}
	return matches
}

func walkSkillRoot(root string, emit func(skill SkillDescriptor)) error {
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return nil
	}

	return filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d == nil || d.IsDir() {
			return nil
		}
		if !strings.EqualFold(d.Name(), "SKILL.md") {
			return nil
		}
		name := skillNameFromRelativePath(root, path)
		if name == "" {
			name = strings.TrimSpace(filepath.Base(filepath.Dir(path)))
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			abs = path
		}
		emit(SkillDescriptor{Name: name, Path: abs})
		return nil
	})
}

func skillNameFromRelativePath(root, skillFile string) string {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return ""
	}
	fileAbs, err := filepath.Abs(skillFile)
	if err != nil {
		return ""
	}
	relDir, err := filepath.Rel(rootAbs, filepath.Dir(fileAbs))
	if err != nil {
		return ""
	}
	relDir = strings.TrimSpace(filepath.ToSlash(relDir))
	if relDir == "" || relDir == "." || relDir == ".." || strings.HasPrefix(relDir, "../") {
		return ""
	}
	parts := strings.Split(relDir, "/")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || part == "." {
			continue
		}
		out = append(out, part)
	}
	if len(out) == 0 {
		return ""
	}
	return out[len(out)-1]
}

