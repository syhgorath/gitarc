package gitengine

import (
	"testing"
)

func TestParseDiffOutput(t *testing.T) {
	raw := `diff --git a/main.go b/main.go
index 1111111..2222222 100644
--- a/main.go
+++ b/main.go
@@ -10,3 +10,5 @@ func main() {
-	oldCall()
+	newCall()
+	extraCall()
 }
diff --git a/new_file.txt b/new_file.txt
new file mode 100644
index 0000000..3333333
--- /dev/null
+++ b/new_file.txt
@@ -0,0 +1,2 @@
+line 1
+line 2
`
	res := ParseDiffOutput(raw, "feat/test", "branch", "Test Branch")
	if res.FilesChanged != 2 {
		t.Fatalf("expected 2 files changed, got %d", res.FilesChanged)
	}
	if res.TotalAdds != 4 {
		t.Errorf("expected 4 total adds, got %d", res.TotalAdds)
	}
	if res.TotalDels != 1 {
		t.Errorf("expected 1 total dels, got %d", res.TotalDels)
	}

	if res.Files[0].Path != "main.go" || res.Files[0].Status != "M" {
		t.Errorf("unexpected file 0: %+v", res.Files[0])
	}
	if res.Files[0].Additions != 2 || res.Files[0].Deletions != 1 {
		t.Errorf("unexpected file 0 stats: %+v", res.Files[0])
	}

	if res.Files[1].Path != "new_file.txt" || res.Files[1].Status != "A" {
		t.Errorf("unexpected file 1: %+v", res.Files[1])
	}
	if res.Files[1].Additions != 2 || res.Files[1].Deletions != 0 {
		t.Errorf("unexpected file 1 stats: %+v", res.Files[1])
	}
}

func TestGetDiffSimLocked(t *testing.T) {
	mgr := NewManager("")
	res, err := mgr.GetDiff("feat/mock", "branch")
	if err != nil {
		t.Fatalf("expected no error in sim diff, got: %v", err)
	}
	if res.FilesChanged == 0 {
		t.Errorf("expected at least 1 file changed in sim diff")
	}
	if res.RawDiff == "" {
		t.Errorf("expected non-empty raw diff in sim diff")
	}
}
