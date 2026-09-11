package utils

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpdateJsonFilePreservesCommentsAndFormat(t *testing.T) {
	raw := `// This file is a configuration file for XyPriss.
// It is automatically loaded by the XyPriss when booting up your server.
// You can configure it like you want according to the docs.
// For more information, check the documentation at:
// https://xypriss.nehonix.com/docs/config/guide
{
    // Important internal comment
    "$vars": {
        "__name__": "&(pkg).name",
        "__description__": "&(pkg).description",
        "__version__": "&(pkg).version",
        "__author__": "&(pkg).author",
        "__PORT__": "&(env).PORT",
        "__alias__": "&(pkg).alias || app",
        "xems.data.path": "&(env).XEMS_DATA_PATH || ./data/vault.p.xems"
    },
    "manifest": {
        "name": "&(this).$vars.__name__",
        "description": "&(this).$vars.__description__",
        "version": "&(this).$vars.__version__",
        "author": "&(this).$vars.__author__"
    }
}
// Trailing footer comment
`
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "xypriss.config.jsonc")
	if err := os.WriteFile(cfgPath, []byte(raw), 0644); err != nil {
		t.Fatalf("failed to write tmp file: %v", err)
	}

	updates := map[string]interface{}{
		"$internal.xyphra.signature.author_key": "ed25519:da16560012a6baef17647360c791646ccf63ed6d03139e759",
		"$internal.xyphra.permissions.allowedHooks": []string{
			"XHS.HOOK.HTTP.REQUEST",
			"XHS.HOOK.LIFECYCLE.SERVER_START",
			"XHS.HOOK.HTTP.RESPONSE",
			"XHS.HOOK.METRICS.RESPONSE_TIME",
			"XHS.PERM.HTTP.GLOBAL_MIDDLEWARE",
			"XHS.PERM.HTTP.MIDDLEWARE",
		},
	}

	if err := UpdateJsonFile(cfgPath, updates); err != nil {
		t.Fatalf("UpdateJsonFile error: %v", err)
	}

	updatedBytes, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("failed to read updated file: %v", err)
	}
	res := string(updatedBytes)

	t.Logf("UPDATED CONTENT:\n%s", res)

	// Verify comments are kept
	if !strings.Contains(res, "// This file is a configuration file for XyPriss.") {
		t.Errorf("Header comment was removed!")
	}
	if !strings.Contains(res, "// Important internal comment") {
		t.Errorf("Internal comment was removed!")
	}
	if !strings.Contains(res, "// Trailing footer comment") {
		t.Errorf("Trailing comment was removed!")
	}

	// Verify & is NOT escaped to \u0026
	if strings.Contains(res, `\u0026`) {
		t.Errorf("Ampersands were escaped to \\u0026!")
	}
	if !strings.Contains(res, `"&(pkg).name"`) {
		t.Errorf("Original variable &(pkg).name was lost!")
	}
	if !strings.Contains(res, `"xems.data.path": "&(env).XEMS_DATA_PATH || ./data/vault.p.xems"`) {
		t.Errorf("xems.data.path was lost or altered!")
	}

	// Verify new internal config was added
	if !strings.Contains(res, "ed25519:da16560012a6baef17647360c791646ccf63ed6d03139e759") {
		t.Errorf("New author_key was not added!")
	}
	if !strings.Contains(res, "XHS.HOOK.HTTP.REQUEST") {
		t.Errorf("Allowed hooks were not added!")
	}

	// Test updating an existing internal plugin
	update2 := map[string]interface{}{
		"$internal.xyphra.signature.author_key": "ed25519:updated_key_123",
	}
	if err := UpdateJsonFile(cfgPath, update2); err != nil {
		t.Fatalf("UpdateJsonFile round 2 error: %v", err)
	}

	updatedBytes2, _ := os.ReadFile(cfgPath)
	res2 := string(updatedBytes2)
	t.Logf("UPDATED ROUND 2:\n%s", res2)
	if !strings.Contains(res2, "ed25519:updated_key_123") {
		t.Errorf("author_key was not updated in round 2!")
	}
}
