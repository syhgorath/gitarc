package skills

import "testing"

func TestEmbeddedSkillsHaveNameDescriptionAndContent(t *testing.T) {
	all := All()
	if len(all) == 0 {
		t.Fatal("gömülü skill yok")
	}
	for _, s := range all {
		if s.Name == "" || s.Description == "" || len(s.Content) < 200 {
			t.Fatalf("eksik skill: %+v", s.Name)
		}
		if got, ok := Get(s.Name); !ok || got.Content != s.Content {
			t.Fatalf("Get(%q) tutarsız", s.Name)
		}
	}
	if _, ok := Get("ci-fix"); !ok {
		t.Fatal("ci-fix gömülü olmalı")
	}
}

func TestGetRefusesNamesThatCouldBePaths(t *testing.T) {
	for _, n := range []string{"", "../x", "ci-fix/..", "a/b", `a\b`, ".", "..", "yok-boyle-skill"} {
		if _, ok := Get(n); ok {
			t.Fatalf("%q reddedilmeli", n)
		}
	}
}
