package moodle

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestVersionSuffix(t *testing.T) {
	cases := map[string]string{"503": "53", "504": "54", "600": "60"}

	for mdlbranch, want := range cases {
		got, err := VersionSuffix(mdlbranch)
		if err != nil || got != want {
			t.Errorf("VersionSuffix(%q) = %q, %v; want %q", mdlbranch, got, err, want)
		}
	}

	for _, bad := range []string{"", "53", "5030", "50x"} {
		if _, err := VersionSuffix(bad); err == nil {
			t.Errorf("VersionSuffix(%q) should fail", bad)
		}
	}
}

func TestDateVersion(t *testing.T) {
	day := time.Date(2026, time.November, 5, 23, 59, 0, 0, time.UTC)

	got, err := DateVersion(day, "503")
	if err != nil || got != "2026110553" {
		t.Errorf("DateVersion = %q, %v; want 2026110553", got, err)
	}
}

func TestCheckPluginVersion(t *testing.T) {
	if err := CheckPluginVersion("2026110553", "503"); err != nil {
		t.Errorf("a valid 503 stamp was rejected: %v", err)
	}

	bad := []string{
		"2026110552",    // another branch's suffix
		"2026023153",    // 31 February
		"2026130153",    // month 13
		"202611053",     // too short
		"20261105053",   // too long
		"2026110553.01", // the tool sets whole versions only
		"today",
	}

	for _, version := range bad {
		if err := CheckPluginVersion(version, "503"); err == nil {
			t.Errorf("CheckPluginVersion(%q, 503) should fail", version)
		}
	}

	err := CheckPluginVersion("2026110552", "503")
	if err == nil || !strings.Contains(err.Error(), "YYYYMMDD53") {
		t.Errorf("the error should show the expected shape: %v", err)
	}
}

func TestVersionNumber(t *testing.T) {
	cases := map[string]int64{"2026110553": 2026110553, "2026110553.01": 2026110553, "'2026110553'": 2026110553}

	for version, want := range cases {
		if got, err := VersionNumber(version); err != nil || got != want {
			t.Errorf("VersionNumber(%q) = %d, %v", version, got, err)
		}
	}

	if _, err := VersionNumber("ANY_VERSION"); err == nil {
		t.Error("a constant is not a version number")
	}
}

func TestSetPluginVersionKeepsAlignment(t *testing.T) {
	content := "<?php\n$plugin->component = 'tool_mulib';\n$plugin->version   = 2026103053;\n$plugin->requires  = 2026100600;\n"

	got, err := SetPluginVersion([]byte(content), "2026110553")
	if err != nil {
		t.Fatal(err)
	}

	want := "<?php\n$plugin->component = 'tool_mulib';\n$plugin->version   = 2026110553;\n$plugin->requires  = 2026100600;\n"
	if string(got) != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestSetPluginVersionQuotedAndDecimal(t *testing.T) {
	for _, value := range []string{"'2026103053'", "2026103053.01"} {
		got, err := SetPluginVersion([]byte("$plugin->version = "+value+";\n"), "2026110553")
		if err != nil {
			t.Fatal(err)
		}

		if string(got) != "$plugin->version = 2026110553;\n" {
			t.Errorf("%s became %q", value, got)
		}
	}
}

func TestSetPluginVersionNeedsExactlyOne(t *testing.T) {
	if _, err := SetPluginVersion([]byte("<?php\n$version = 2026042001.00;\n"), "2026110553"); err == nil {
		t.Error("core's version.php has no $plugin->version")
	}

	two := "$plugin->version = 1;\n$plugin->version = 2;\n"
	if _, err := SetPluginVersion([]byte(two), "2026110553"); err == nil {
		t.Error("two assignments are ambiguous")
	}
}

func TestSetDependencyVersions(t *testing.T) {
	content := `<?php
$plugin->component = 'tool_muprog';
$plugin->version   = 2026103053;
$plugin->dependencies = [
    'tool_mulib'   => 2026103053,
    "tool_mutrain" => '2026101053',
    'tool_other'   => 2026010100,
    'tool_any'     => ANY_VERSION,
];
$string['tool_mulib'] = 2026103053;
`

	got, changes, err := SetDependencyVersions([]byte(content), []string{"tool_mulib", "tool_mutrain", "tool_any"}, "2026110553")
	if err != nil {
		t.Fatal(err)
	}

	want := `<?php
$plugin->component = 'tool_muprog';
$plugin->version   = 2026103053;
$plugin->dependencies = [
    'tool_mulib'   => 2026110553,
    "tool_mutrain" => 2026110553,
    'tool_other'   => 2026010100,
    'tool_any'     => ANY_VERSION,
];
$string['tool_mulib'] = 2026103053;
`
	if string(got) != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}

	wantChanges := []DependencyChange{
		{Component: "tool_mulib", From: "2026103053", To: "2026110553"},
		{Component: "tool_mutrain", From: "2026101053", To: "2026110553"},
	}
	if !reflect.DeepEqual(changes, wantChanges) {
		t.Errorf("changes = %+v", changes)
	}
}

func TestSetDependencyVersionsArraySyntax(t *testing.T) {
	content := "$plugin->dependencies = array('tool_mulib' => 2026103053, 'tool_x' => 1);\n"

	got, changes, err := SetDependencyVersions([]byte(content), []string{"tool_mulib"}, "2026110553")
	if err != nil {
		t.Fatal(err)
	}

	if string(got) != "$plugin->dependencies = array('tool_mulib' => 2026110553, 'tool_x' => 1);\n" || len(changes) != 1 {
		t.Errorf("got %q, %+v", got, changes)
	}
}

func TestSetDependencyVersionsWithoutDependencies(t *testing.T) {
	content := "$plugin->version = 2026103053;\n"

	got, changes, err := SetDependencyVersions([]byte(content), []string{"tool_mulib"}, "2026110553")
	if err != nil || string(got) != content || len(changes) != 0 {
		t.Errorf("a file without dependencies must be left alone: %q, %+v, %v", got, changes, err)
	}
}

func TestSetDependencyVersionsUnclosed(t *testing.T) {
	if _, _, err := SetDependencyVersions([]byte("$plugin->dependencies = ['tool_mulib' => 1,\n"), []string{"tool_mulib"}, "2"); err == nil {
		t.Error("an unclosed array should be an error")
	}
}
