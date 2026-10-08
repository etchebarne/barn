// Package skills reads and writes the skill library: reusable instructions agents follow for a
// kind of job, one folder each under /shared/skills (so every agent's computer sees them), in
// the Agent Skills format: a SKILL.md with YAML frontmatter (name, description) and Markdown
// instructions, optionally next to scripts and other files the instructions refer to.
package skills

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

// File is the name of a skill's instructions file.
const File = "SKILL.md"

const (
	MaxDescription  = 1024
	MaxInstructions = 64 << 10
	maxFiles        = 200 // listed per skill
)

var nameRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// ErrNotFound means there's no skill by that name.
var ErrNotFound = errors.New("no such skill")

// Skill is one skill in the library.
type Skill struct {
	Name        string
	Description string
	// Instructions is SKILL.md without its frontmatter (only filled in by Get).
	Instructions string
	// Files are the other files in its folder, relative to it (only filled in by Get).
	Files     []string
	UpdatedAt time.Time
	// Problem says why the folder isn't a usable skill (no SKILL.md, bad frontmatter…).
	Problem string
}

// Library is the skills folder on the host (<data>/shared/skills).
type Library struct{ Dir string }

// Path is where a skill lives as agents see it.
func Path(name string) string { return "/shared/skills/" + name }

// ValidName reports whether name can be a skill's name: lowercase letters, digits and hyphens.
func ValidName(name string) error {
	if len(name) == 0 || len(name) > 64 || !nameRe.MatchString(name) {
		return fmt.Errorf("a skill name is 1–64 lowercase letters, digits and hyphens (like weekly-report), got %q", name)
	}
	return nil
}

// List returns every skill, by name. Folders that aren't valid skills are listed with a Problem.
func (l Library) List() ([]Skill, error) {
	entries, err := os.ReadDir(l.Dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Skill
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		s, err := l.read(e.Name(), false)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

// Get returns a skill with its instructions and files.
func (l Library) Get(name string) (Skill, error) {
	if ValidName(name) != nil {
		return Skill{}, ErrNotFound
	}
	if fi, err := os.Stat(filepath.Join(l.Dir, name)); err != nil || !fi.IsDir() {
		return Skill{}, ErrNotFound
	}
	return l.read(name, true)
}

func (l Library) read(folder string, full bool) (Skill, error) {
	s := Skill{Name: folder}
	dir := filepath.Join(l.Dir, folder)
	path := filepath.Join(dir, File)
	if fi, err := os.Stat(dir); err == nil {
		s.UpdatedAt = fi.ModTime()
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		s.Problem = "it has no " + File
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if fi, err := os.Stat(path); err == nil && fi.ModTime().After(s.UpdatedAt) {
		s.UpdatedAt = fi.ModTime()
	}
	meta, body, err := parse(b)
	switch {
	case err != nil:
		s.Problem = err.Error()
	case ValidName(folder) != nil:
		s.Problem = "its folder name isn't a valid skill name (lowercase letters, digits and hyphens)"
	case meta.Name != "" && meta.Name != folder:
		s.Problem = fmt.Sprintf("its name (%q) doesn't match its folder", meta.Name)
	case strings.TrimSpace(meta.Description) == "":
		s.Problem = "its description is missing"
	}
	s.Description = strings.TrimSpace(meta.Description)
	if full {
		s.Instructions = body
		s.Files = l.files(dir)
	}
	return s, nil
}

// files lists a skill folder's files other than SKILL.md.
func (l Library) files(dir string) []string {
	var out []string
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || len(out) >= maxFiles {
			return filepath.SkipDir
		}
		if d.IsDir() {
			if p != dir && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(dir, p)
		if rel != File {
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	})
	slices.Sort(out)
	return out
}

type meta struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

// parse splits SKILL.md into its frontmatter and the instructions after it.
func parse(b []byte) (meta, string, error) {
	var m meta
	text := strings.TrimPrefix(string(b), "\ufeff")
	text = strings.ReplaceAll(text, "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return m, text, errors.New("it doesn't start with frontmatter (--- name, description ---)")
	}
	rest := text[4:]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return m, text, errors.New("its frontmatter isn't closed with ---")
	}
	if err := yaml.Unmarshal([]byte(rest[:end]), &m); err != nil {
		return m, text, fmt.Errorf("its frontmatter isn't valid YAML: %v", err)
	}
	body := rest[end+4:]
	if i := strings.IndexByte(body, '\n'); i >= 0 {
		body = body[i+1:]
	} else {
		body = ""
	}
	return m, strings.TrimLeft(body, "\n"), nil
}

// Render writes a SKILL.md.
func Render(name, description, instructions string) ([]byte, error) {
	front, err := yaml.Marshal(meta{Name: name, Description: description})
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	b.WriteString("---\n")
	b.Write(front)
	b.WriteString("---\n\n")
	b.WriteString(strings.TrimSpace(instructions))
	b.WriteString("\n")
	return b.Bytes(), nil
}

// Validate checks a skill's fields before saving.
func Validate(name, description, instructions string) error {
	if err := ValidName(name); err != nil {
		return err
	}
	d := strings.TrimSpace(description)
	if d == "" || utf8.RuneCountInString(d) > MaxDescription {
		return fmt.Errorf("the description must be 1–%d characters: what the skill does and when to use it", MaxDescription)
	}
	if strings.TrimSpace(instructions) == "" || len(instructions) > MaxInstructions {
		return fmt.Errorf("the instructions must be 1–%d bytes", MaxInstructions)
	}
	return nil
}

// Save creates or replaces a skill's SKILL.md (other files in its folder stay).
func (l Library) Save(name, description, instructions string) (Skill, error) {
	if err := Validate(name, description, instructions); err != nil {
		return Skill{}, err
	}
	b, err := Render(name, strings.TrimSpace(description), instructions)
	if err != nil {
		return Skill{}, err
	}
	dir := filepath.Join(l.Dir, name)
	// World-writable like the rest of /shared: agents' sandboxes run as root, but the user's
	// computer access and other tools may not.
	if err := os.MkdirAll(dir, 0o777); err != nil {
		return Skill{}, err
	}
	tmp, err := os.CreateTemp(dir, ".SKILL-*.md")
	if err != nil {
		return Skill{}, err
	}
	_, werr := tmp.Write(b)
	cerr := tmp.Close()
	if err := errors.Join(werr, cerr); err != nil {
		os.Remove(tmp.Name())
		return Skill{}, err
	}
	_ = os.Chmod(tmp.Name(), 0o666)
	if err := os.Rename(tmp.Name(), filepath.Join(dir, File)); err != nil {
		os.Remove(tmp.Name())
		return Skill{}, err
	}
	return l.Get(name)
}

// Delete removes a skill's folder with everything in it.
func (l Library) Delete(name string) error {
	if _, err := l.Get(name); err != nil {
		return err
	}
	return os.RemoveAll(filepath.Join(l.Dir, name))
}
