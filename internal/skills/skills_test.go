package skills

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLibrary(t *testing.T) {
	lib := Library{Dir: filepath.Join(t.TempDir(), "skills")}
	if list, err := lib.List(); err != nil || len(list) != 0 {
		t.Fatalf("empty library = %v, %v", list, err)
	}

	s, err := lib.Save("weekly-report", "Write the weekly report.\nUse on Fridays.", "1. Gather the numbers.\n2. Run report.py.\n")
	if err != nil {
		t.Fatal(err)
	}
	if s.Description != "Write the weekly report.\nUse on Fridays." || s.Instructions != "1. Gather the numbers.\n2. Run report.py.\n" {
		t.Fatalf("saved = %+v", s)
	}
	// Files next to SKILL.md are listed, and kept when it's rewritten.
	if err := os.WriteFile(filepath.Join(lib.Dir, "weekly-report", "report.py"), []byte("print(1)"), 0o644); err != nil {
		t.Fatal(err)
	}
	if s, err = lib.Save("weekly-report", "Write the weekly report.", "Run report.py."); err != nil {
		t.Fatal(err)
	}
	if strings.Join(s.Files, ",") != "report.py" || s.Instructions != "Run report.py.\n" {
		t.Fatalf("resaved = %+v", s)
	}

	// Hand-made folders: a standard SKILL.md, and broken ones listed with why.
	write := func(dir, content string) {
		if err := os.MkdirAll(filepath.Join(lib.Dir, dir), 0o755); err != nil {
			t.Fatal(err)
		}
		if content != "" {
			if err := os.WriteFile(filepath.Join(lib.Dir, dir, File), []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	write("pdf-tools", "---\nname: pdf-tools\ndescription: >\n  Fill and merge PDFs.\nlicense: MIT\n---\n# PDF tools\n\nUse pdftk.\n")
	write("empty", "")
	write("Bad_Name", "---\nname: Bad_Name\ndescription: x\n---\nbody")
	write("mismatch", "---\nname: other\ndescription: x\n---\nbody")
	write("no-front", "# Just text")
	list, err := lib.List()
	if err != nil {
		t.Fatal(err)
	}
	problems := map[string]string{}
	for _, s := range list {
		problems[s.Name] = s.Problem
	}
	for name, want := range map[string]string{
		"weekly-report": "", "pdf-tools": "", "empty": "no SKILL.md", "Bad_Name": "folder name",
		"mismatch": "doesn't match", "no-front": "frontmatter",
	} {
		if got, ok := problems[name]; !ok || (want == "" && got != "") || (want != "" && !strings.Contains(got, want)) {
			t.Errorf("%s: problem %q, want %q", name, got, want)
		}
	}
	pdf, err := lib.Get("pdf-tools")
	if err != nil || pdf.Description != "Fill and merge PDFs." || pdf.Instructions != "# PDF tools\n\nUse pdftk.\n" {
		t.Fatalf("pdf-tools = %+v, %v", pdf, err)
	}

	// Validation and deletion.
	for _, bad := range [][3]string{{"Nope", "d", "i"}, {"ok", "", "i"}, {"ok", "d", " "}, {"../x", "d", "i"}} {
		if _, err := lib.Save(bad[0], bad[1], bad[2]); err == nil {
			t.Errorf("Save%q should fail", bad)
		}
	}
	if _, err := lib.Get("../skills"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get(../skills) = %v", err)
	}
	if err := lib.Delete("weekly-report"); err != nil {
		t.Fatal(err)
	}
	if _, err := lib.Get("weekly-report"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted skill: %v", err)
	}
}
