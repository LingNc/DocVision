package latex

import (
	"os"
	"path/filepath"
	"testing"
)

// T121: a re-entered verification session must keep the grouping it had already
// built — rewriting the prefill would silently discard the model's motif_groups
// edits every time an interrupted run is resumed.
func TestPrepareMotifScopeViewKeepsResumedGroups(t *testing.T) {
	root := t.TempDir()
	sc := &motifScope{}
	if err := prepareMotifScopeView(root, sc); err != nil {
		t.Fatal(err)
	}
	groupsPath := filepath.Join(root, "groups.json")
	custom := "{\n  \"groups\": [\n    {\"members\": [1, 2, 3], \"key\": 2}\n  ]\n}\n"
	if err := os.WriteFile(groupsPath, []byte(custom), 0644); err != nil {
		t.Fatal(err)
	}
	if err := prepareMotifScopeView(root, sc); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(groupsPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != custom {
		t.Fatalf("re-entry must preserve groups.json, got %q", got)
	}
}

// T121: the evidence gate is an in-memory set, so it is mirrored to disk.
// Without this a resumed session (which replays history instead of re-showing
// images) can never pass submit.
func TestMotifViewToolPersistsEvidence(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "originals"), 0755); err != nil {
		t.Fatal(err)
	}
	writePNG(t, filepath.Join(root, "originals", "000003.png"), 40, 40)
	seenFile := filepath.Join(root, "seen.json")
	tool := &motifViewTool{
		base:   &ViewImageTool{Root: root, BareSearch: true, SoftMax: 10, WarnRatio: 0.8},
		seen:   loadMotifSeen(seenFile),
		dbPath: seenFile,
	}
	// A cropped view is not evidence.
	if _, err := tool.Execute(`{"path":"originals/000003.png","left":10}`); err != nil {
		t.Fatal(err)
	}
	if len(tool.seen) != 0 {
		t.Fatalf("cropped view must not count as evidence: %#v", tool.seen)
	}
	if _, err := tool.Execute(`{"path":"originals/000003.png"}`); err != nil {
		t.Fatal(err)
	}
	if !tool.seen[3] {
		t.Fatalf("uncropped view must be recorded: %#v", tool.seen)
	}
	if got := loadMotifSeen(seenFile); !got[3] {
		t.Fatalf("evidence not persisted to %s: %#v", seenFile, got)
	}
}

// T121: replaying a recorded submit after a restart only succeeds when the
// evidence it validated before is restored from disk.
func TestMotifSubmitAcceptsEvidenceFromDisk(t *testing.T) {
	root := t.TempDir()
	groupsPath := filepath.Join(root, "groups.json")
	if err := os.WriteFile(groupsPath, []byte("{\"groups\":[{\"members\":[1,2],\"key\":1}]}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	seenFile := filepath.Join(root, "seen.json")
	groups := NewMotifGroupsTool(groupsPath, 3)

	empty := &motifSubmitTool{done: SubmitDoneTool{}, groups: groups, seen: loadMotifSeen(seenFile)}
	res, err := empty.Execute(`{}`)
	if err != nil {
		t.Fatal(err)
	}
	if res.Text == "" || res.Text[:8] != "REJECTED" {
		t.Fatalf("without evidence submit must be rejected, got %q", res.Text)
	}

	if err := os.WriteFile(seenFile, []byte("{\"inspected\":[1,2]}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	resumed := &motifSubmitTool{done: SubmitDoneTool{}, groups: groups, seen: loadMotifSeen(seenFile)}
	if _, err := resumed.Execute(`{}`); err != nil {
		t.Fatal(err)
	}
	if !resumed.done.Submitted {
		t.Fatal("restored evidence must let the replayed submit through")
	}
}
