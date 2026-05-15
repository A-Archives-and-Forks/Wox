package system

import (
	"os"
	"path/filepath"
	"testing"
	"wox/util/filesearch"
)

func TestFileSearchPolicyUsesPolicyRootPathForDynamicRootGitIgnore(t *testing.T) {
	parentRoot := filepath.Join(t.TempDir(), "root-policy-parent")
	dynamicRoot := filepath.Join(parentRoot, "workspace", "content")
	ignoredFile := filepath.Join(dynamicRoot, "ignored.log")
	keptFile := filepath.Join(dynamicRoot, "kept.txt")

	mustWritePolicyTestFile(t, filepath.Join(parentRoot, ".gitignore"), "*.log\n")
	mustWritePolicyTestFile(t, ignoredFile, "ignored")
	mustWritePolicyTestFile(t, keptFile, "kept")

	policy := newFileSearchIndexPolicy()
	root := filesearch.RootRecord{
		ID:             "root-policy-dynamic",
		Path:           dynamicRoot,
		Kind:           filesearch.RootKindDynamic,
		PolicyRootPath: parentRoot,
	}

	if policy.shouldIndexPath(root, ignoredFile, false) {
		t.Fatalf("expected dynamic root to inherit parent gitignore for %q", ignoredFile)
	}
	if !policy.shouldIndexPath(root, keptFile, false) {
		t.Fatalf("expected dynamic root to keep non-ignored file %q", keptFile)
	}
}

func mustWritePolicyTestFile(t *testing.T, path string, contents string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %q: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write %q: %v", path, err)
	}
}
