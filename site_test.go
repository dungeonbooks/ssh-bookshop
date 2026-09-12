package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// The site under deploy/ is hand-written static files, so nothing stops it
// drifting from the code except these.

// TestOpenAPIDocumentsEveryRoute: a route the API serves that the contract does
// not mention is a route no agent will find.
func TestOpenAPIDocumentsEveryRoute(t *testing.T) {
	raw, err := os.ReadFile("deploy/site/openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Paths map[string]map[string]any `json:"paths"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("openapi.json: %v", err)
	}
	for _, route := range apiRoutes {
		method, path, _ := strings.Cut(route, " ")
		ops, ok := doc.Paths[path]
		if !ok {
			t.Errorf("%s: path %s is not in openapi.json", route, path)
			continue
		}
		if _, ok := ops[strings.ToLower(method)]; !ok {
			t.Errorf("%s: no %s operation on %s in openapi.json", route, method, path)
		}
	}
	for path, ops := range doc.Paths {
		for method := range ops {
			want := strings.ToUpper(method) + " " + path
			found := false
			for _, route := range apiRoutes {
				if route == want {
					found = true
				}
			}
			if !found {
				t.Errorf("openapi.json documents %s, which the API does not serve", want)
			}
		}
	}
}

// TestSkillCopiesMatch: the skill is one file served three ways (embedded in
// the CLI, on the site, in the repo), and the copies have to agree.
func TestSkillCopiesMatch(t *testing.T) {
	src, err := os.ReadFile("cmd/dungeon/skill.md")
	if err != nil {
		t.Fatal(err)
	}
	site, err := os.ReadFile("deploy/site/skill.md")
	if err != nil {
		t.Fatal(err)
	}
	if string(src) != string(site) {
		t.Error("deploy/site/skill.md differs from cmd/dungeon/skill.md; copy it over")
	}
}

// TestLLMsFullInlinesEveryPage: llms-full.txt is generated from the docs, and
// an edited page that was not regenerated would leave the two disagreeing.
func TestLLMsFullInlinesEveryPage(t *testing.T) {
	full, err := os.ReadFile("deploy/site/llms-full.txt")
	if err != nil {
		t.Fatal(err)
	}
	for _, page := range []string{"buying", "api", "cli", "ssh"} {
		body, err := os.ReadFile("deploy/site/docs/" + page + ".md")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(full), strings.TrimSpace(string(body))) {
			t.Errorf("llms-full.txt does not contain docs/%s.md as written; regenerate it (see deploy/README.md)", page)
		}
	}
	skill, err := os.ReadFile("deploy/site/skill.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(full), strings.TrimSpace(string(skill))) {
		t.Error("llms-full.txt does not contain skill.md as written; regenerate it")
	}
	index, err := os.ReadFile("deploy/site/llms.txt")
	if err != nil {
		t.Fatal(err)
	}
	for _, page := range []string{"docs/buying.md", "docs/api.md", "docs/cli.md", "docs/ssh.md", "openapi.json", "skill.md"} {
		if !strings.Contains(string(index), "https://shop.dungeonbooks.com/"+page) {
			t.Errorf("llms.txt does not link %s", page)
		}
	}
}
