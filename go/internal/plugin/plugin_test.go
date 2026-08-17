package plugin

import (
	"strings"
	"testing"
)

const mulibYAML = `
name: mutms/tool_mulib
title: MuTMS shared library
relpath: public/admin/tool/mulib
source:
  git:
    remotes:
      origin: https://github.com/mutms/moodle-tool_mulib.git
  composer: mutms/moodle-tool_mulib
requirements:
  MOODLE_405_STABLE:
    mdlbranches: ["405"]
  MOODLE_500_STABLE:
    mdlbranches: ["500", "501", "502"]
    plugins: ["mutms/tool_muprog"]
`

func TestParse(t *testing.T) {
	p, err := Parse([]byte(mulibYAML))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if p.Name != "mutms/tool_mulib" || p.Relpath != "public/admin/tool/mulib" {
		t.Errorf("unexpected plugin: %+v", p)
	}

	if p.Source.Git.Remotes["origin"] == "" || p.Source.Composer == "" {
		t.Errorf("source kinds not decoded: %+v", p.Source)
	}

	// The whole document is kept, so flattening into a live recipe preserves
	// fields mudev does not model.
	if p.Raw["title"] != "MuTMS shared library" {
		t.Errorf("Raw not populated: %v", p.Raw)
	}
}

func TestParseRejectsInvalidDocument(t *testing.T) {
	// relpath is required by the schema.
	_, err := Parse([]byte("name: mutms/tool_mulib\ntitle: x\nrequirements: {}\n"))
	if err == nil {
		t.Fatal("expected a schema validation error")
	}
}

func TestBranchFor(t *testing.T) {
	p, err := Parse([]byte(mulibYAML))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	// One git branch serves several Moodle branches — the reason the
	// catalogue is keyed the other way round.
	for _, mdlbranch := range []string{"500", "501", "502"} {
		branch, err := p.BranchFor(mdlbranch)
		if err != nil {
			t.Fatalf("BranchFor(%s): %v", mdlbranch, err)
		}

		if branch != "MOODLE_500_STABLE" {
			t.Errorf("BranchFor(%s) = %q", mdlbranch, branch)
		}
	}

	if _, err := p.BranchFor("401"); err == nil {
		t.Error("expected an error for an unserved Moodle branch")
	}
}

func TestBranchMapReportsDuplicates(t *testing.T) {
	p := &Plugin{
		Name: "mutms/example",
		Requirements: map[string]Requirement{
			"MOODLE_500_STABLE": {Mdlbranches: []string{"500"}},
			"MOODLE_501_STABLE": {Mdlbranches: []string{"500"}},
		},
	}

	_, err := p.BranchMap()
	if err == nil {
		t.Fatal("expected an error when two branches claim one Moodle branch")
	}

	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error should name the clashing branch: %v", err)
	}
}
