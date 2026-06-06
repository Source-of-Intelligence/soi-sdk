// soi-verify — Verify SOI WASM plugins.
//
// Generates test harnesses and runs tests for plugin projects.
//
// Usage:
//
//	soi-verify --gen --dir ./my-plugin          # Generate test harness
//	soi-verify --test --dir ./my-plugin          # Run plugin tests
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func main() {
	gen := flag.Bool("gen", false, "Generate test harness (main_test.go)")
	test := flag.Bool("test", false, "Run plugin tests")
	dir := flag.String("dir", ".", "Plugin directory")
	flag.Parse()

	if !*gen && !*test {
		fmt.Fprintf(os.Stderr, "Usage: soi-verify --gen|--test --dir <path>\n")
		os.Exit(1)
	}

	absDir, _ := filepath.Abs(*dir)

	if *gen {
		if err := genHarness(absDir); err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Generated main_test.go")
	}

	if *test {
		if err := runTests(absDir); err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
			os.Exit(1)
		}
	}
}

func genHarness(dir string) error {
	content := `package main

import (
	"encoding/json"
	"testing"

	"github.com/Source-of-Intelligence/soi-sdk"
)

func TestManifest(t *testing.T) {
	manifest := sdk.GetManifest()
	if manifest.SDKVersion == "" {
		t.Fatal("SDK version is empty")
	}
	t.Logf("Manifest: SDK=%s ABI=%s Tools=%d BuildTag=%s",
		manifest.SDKVersion, manifest.ABIVersion, len(manifest.Tools), manifest.BuildTag)
}

func TestAllTools(t *testing.T) {
	tools := sdk.GetTools()
	if len(tools) == 0 {
		t.Fatal("no tools registered")
	}
	for name := range tools {
		t.Run(name, func(t *testing.T) {
			argsJSON, _ := json.Marshal(map[string]interface{}{})
			resp := sdk.CallTool(name, argsJSON, "", nil)
			if resp.Error != "" {
				t.Fatalf("tool %s error: %s", name, resp.Error)
			}
			t.Logf("%s: %s", name, string(resp.Output))
		})
	}
}
`
	return os.WriteFile(filepath.Join(dir, "main_test.go"), []byte(content), 0644)
}

func runTests(dir string) error {
	cmd := exec.Command("go", "test", "-v", "./...")
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
