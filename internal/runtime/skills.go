package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/etchebarne/openbot/internal/skills"
	"github.com/etchebarne/openbot/internal/store"
)

// Skills are reusable instructions for a kind of job, shared by every agent (see package
// skills). The prompt lists their names and descriptions; an agent reads one with use_skill when
// a job calls for it, and proposes new ones with save_skill, which the user approves: a skill
// steers every agent from then on, so one written under the influence of something the agent
// read must not slip in unnoticed.

const (
	toolUseSkill  = "use_skill"
	toolSaveSkill = "save_skill"
)

var useSkillTool = function(toolUseSkill,
	"Read a skill's instructions (and the list of files that come with it) before doing the job it "+
		"covers, then follow them. Skills are listed in your instructions under Skills.",
	`{
		"type": "object",
		"properties": {"name": {"type": "string"}},
		"required": ["name"],
		"additionalProperties": false
	}`)

var saveSkillTool = function(toolSaveSkill,
	"Create or rewrite a skill: instructions any agent follows for a kind of job (a procedure, a "+
		"format, the steps of a recurring task). Use it when the user asks, or offer it when you've "+
		"worked out a multi-step job worth repeating. The user approves it. It's saved as "+
		"/shared/skills/<name>/SKILL.md; put scripts or templates it uses in that folder with "+
		"write_file and refer to them by path.",
	`{
		"type": "object",
		"properties": {
			"name": {"type": "string", "description": "Lowercase letters, digits and hyphens, e.g. weekly-report. An existing name rewrites that skill."},
			"description": {"type": "string", "description": "What it does and when to use it, in a sentence or two: it's how agents decide to use it."},
			"instructions": {"type": "string", "description": "The full instructions, in Markdown: steps, rules, examples, files to use."}
		},
		"required": ["name", "description", "instructions"],
		"additionalProperties": false
	}`)

func (m *Manager) skillsAvailable() bool { return m.Skills != nil }

func (l *loop) useSkill(raw []byte) (string, bool) {
	var a struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &a); err != nil {
		return toolError("invalid arguments: %v", err), false
	}
	s, err := l.m.Skills.Get(strings.TrimSpace(a.Name))
	if errors.Is(err, skills.ErrNotFound) {
		return toolError("there's no skill called %q (see Skills in your instructions)", a.Name), false
	}
	if err != nil {
		return toolError("couldn't read the skill: %v", err), false
	}
	if s.Problem != "" {
		return toolError("the %s skill can't be used: %s", s.Name, s.Problem), false
	}
	out := map[string]any{"name": s.Name, "folder": skills.Path(s.Name), "instructions": s.Instructions}
	if len(s.Files) > 0 {
		files := make([]string, len(s.Files))
		for i, f := range s.Files {
			files[i] = skills.Path(s.Name) + "/" + f
		}
		out["files"] = files
	}
	return toolOK(out), true
}

func (l *loop) saveSkill(raw []byte) (string, bool) {
	var a struct {
		Name         string `json:"name"`
		Description  string `json:"description"`
		Instructions string `json:"instructions"`
	}
	if err := json.Unmarshal(raw, &a); err != nil {
		return toolError("invalid arguments: %v", err), false
	}
	s, err := l.m.Skills.Save(strings.TrimSpace(a.Name), a.Description, a.Instructions)
	if err != nil {
		return toolError("%v", err), false
	}
	return toolOK(map[string]any{"saved": s.Name, "path": skills.Path(s.Name) + "/" + skills.File}), true
}

// describeSaveSkill is the approval card for save_skill.
func describeSaveSkill(args json.RawMessage) (string, *store.ActionPreview, error) {
	var a struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return "", nil, fmt.Errorf("invalid arguments: %w", err)
	}
	fields, body := previewFields(args, map[string]string{"name": "Name", "description": "When to use it"}, "instructions")
	const note = "Every agent can follow it from now on."
	return fmt.Sprintf("Save the skill %q? %s", a.Name, note),
		&store.ActionPreview{Title: "Save skill", Verb: "Save", Note: note, Fields: fields, Body: body}, nil
}

// writeSkills lists the skills in the system prompt.
func (l *loop) writeSkills(_ context.Context, b *strings.Builder) {
	if !l.m.skillsAvailable() {
		return
	}
	list, err := l.m.Skills.List()
	if err != nil {
		logger(l.agentID).Warn("list skills", "err", err)
		return
	}
	b.WriteString("# Skills\n")
	b.WriteString("Shared instructions for kinds of jobs, in /shared/skills. When a job matches one, read it with use_skill " +
		"first and follow it. To capture a procedure worth repeating, propose one with save_skill (the user approves it).\n")
	n := 0
	for _, s := range list {
		if s.Problem != "" {
			continue
		}
		fmt.Fprintf(b, "- %s: %s\n", s.Name, truncate(strings.Join(strings.Fields(s.Description), " "), 300))
		n++
	}
	if n == 0 {
		b.WriteString("(none yet)\n")
	}
	b.WriteString("\n")
}
