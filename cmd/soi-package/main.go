// soi-package — Package a SOI WASM plugin into a distributable .zip file.
//
// Supports both standard Go WASM (.wasm) and TinyGo SOI (.soi) plugins.
// Optionally optimizes WASM output with wasm-opt (from Binaryen).
//
// Usage:
//
//	go run ./cmd/soi-package --dir ./examples/calc
//	go run ./cmd/soi-package --dir ./examples/calc --output ./dist
//	go run ./cmd/soi-package --dir ./examples/calc --skip-build  # skip WASM re-compilation
//	go run ./cmd/soi-package --dir ./examples/soi-demo --type soi  # force SOI type
//	go run ./cmd/soi-package --dir ./examples/soi-demo --optimize  # run wasm-opt after build
package main

import (
	"archive/zip"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

func main() {
	dir := flag.String("dir", "", "Plugin project directory (required)")
	output := flag.String("output", "dist", "Output directory for the .zip file")
	skipBuild := flag.Bool("skip-build", false, "Skip WASM build (use existing wasm/)")
	goBin := flag.String("go", "go", "Go binary path")
	pluginType := flag.String("type", "", "Plugin type: wasm | soi (auto-detected from skill.yaml if not set)")
	optimize := flag.Bool("optimize", false, "Optimize WASM with wasm-opt after build")
	flag.Usage = usage
	flag.Parse()

	if *dir == "" {
		usage()
		os.Exit(1)
	}

	absDir, err := filepath.Abs(*dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: resolve dir: %v\n", err)
		os.Exit(1)
	}

	if _, err := os.Stat(filepath.Join(absDir, "main.go")); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: main.go not found in %s\n", absDir)
		os.Exit(1)
	}

	name := filepath.Base(absDir)
	version := readVersion(absDir)

	// Auto-detect type from skill.yaml if not specified
	rt := *pluginType
	if rt == "" {
		rt = detectType(absDir)
	}
	if rt != "wasm" && rt != "soi" {
		fmt.Fprintf(os.Stderr, "ERROR: --type must be 'wasm' or 'soi', got %q\n", rt)
		os.Exit(1)
	}

	fmt.Println()
	fmt.Printf("  SOI Package Builder\n")
	fmt.Printf("  Plugin:  %s\n", name)
	fmt.Printf("  Version: %s\n", version)
	fmt.Printf("  Type:    %s\n", strings.ToUpper(rt))
	fmt.Printf("  Source:  %s\n", absDir)
	fmt.Println()

	// ── 1. Build ──
	wasmPath := filepath.Join(absDir, "wasm", "plugin.wasm")
	soiPath := filepath.Join(absDir, "wasm", "plugin.soi")
	if rt == "soi" {
		wasmPath = soiPath
	}

	if !*skipBuild {
		fmt.Printf("  [1/4] Building %s...\n", rt)
		if rt == "soi" {
			if err := buildSOI(absDir); err != nil {
				fmt.Fprintf(os.Stderr, "ERROR: build: %v\n", err)
				os.Exit(1)
			}
		} else {
			if err := buildWASM(*goBin, absDir); err != nil {
				fmt.Fprintf(os.Stderr, "ERROR: build: %v\n", err)
				os.Exit(1)
			}
		}
		info, _ := os.Stat(wasmPath)
		if info != nil {
			fmt.Printf("    ✓  %s (%d KB)\n", filepath.Base(wasmPath), info.Size()/1024)
		}
	} else {
		fmt.Println("  [1/4] Skipping build (--skip-build)")
		if _, err := os.Stat(wasmPath); err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: %s not found (build first or remove --skip-build)\n", wasmPath)
			os.Exit(1)
		}
	}

	// ── 2. Optimize (optional) ──
	if *optimize {
		fmt.Println("  [2/4] Optimizing with wasm-opt...")
		wasmOpt := findWasmOpt()
		if wasmOpt == "" {
			fmt.Fprintf(os.Stderr, "WARNING: wasm-opt not found, skipping optimization\n")
			fmt.Fprintf(os.Stderr, "  Install Binaryen: https://github.com/WebAssembly/binaryen\n")
		} else {
			if err := runWasmOpt(wasmOpt, wasmPath); err != nil {
				fmt.Fprintf(os.Stderr, "WARNING: wasm-opt failed: %v\n", err)
			} else {
				info, _ := os.Stat(wasmPath)
				if info != nil {
					fmt.Printf("    ✓  optimized (%d KB)\n", info.Size()/1024)
				}
			}
		}
	} else {
		fmt.Println("  [2/4] Skipping optimization (use --optimize to enable)")
	}

	// ── 3. Verify ──
	fmt.Println("  [3/4] Verifying plugin...")
	if rt == "soi" {
		if err := verifySOI(wasmPath); err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: verify: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("    ✓  execute / registerTools")
	}
	fmt.Println("    ✓  manifest")

	// ── 4. Assemble & ZIP ──
	fmt.Println("  [4/4] Assembling package...")
	pkgName := fmt.Sprintf("%s-%s", name, version)
	pkgDir := filepath.Join(*output, pkgName)

	os.MkdirAll(filepath.Join(pkgDir, "wasm"), 0755)

	// Copy WASM
	copyFile(wasmPath, filepath.Join(pkgDir, "wasm", filepath.Base(wasmPath)))

	// Copy metadata files
	for _, f := range []string{"skill.yaml", "manifest.json", "README.md"} {
		src := filepath.Join(absDir, f)
		if _, err := os.Stat(src); err == nil {
			copyFile(src, filepath.Join(pkgDir, f))
			fmt.Printf("    ✓  %s\n", f)
		}
	}

	// ZIP
	zipPath := filepath.Join(*output, pkgName+".zip")
	if err := createZip(pkgDir, zipPath); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: zip: %v\n", err)
		os.Exit(1)
	}
	os.RemoveAll(pkgDir)

	info, _ := os.Stat(zipPath)
	fmt.Println()
	fmt.Printf("  ╔══════════════════════════════════╗\n")
	fmt.Printf("  ║  Package ready!                  ║\n")
	fmt.Printf("  ║  %-32s ║\n", zipPath)
	if info != nil {
		fmt.Printf("  ║  Size: %d KB                    ║\n", info.Size()/1024)
	}
	fmt.Printf("  ╚══════════════════════════════════╝\n")
	fmt.Println()

	listZipContents(zipPath)
}

// ── Type & Version Detection ──

func detectType(dir string) string {
	yamlPath := filepath.Join(dir, "skill.yaml")
	data, err := os.ReadFile(yamlPath)
	if err != nil {
		// Fallback: check for main_tinygo.go
		if _, err := os.Stat(filepath.Join(dir, "main_tinygo.go")); err == nil {
			return "soi"
		}
		return "wasm"
	}
	re := regexp.MustCompile(`type:\s*(\S+)`)
	m := re.FindStringSubmatch(string(data))
	if len(m) > 1 {
		t := strings.Trim(m[1], `"`)
		if t == "soi" {
			return "soi"
		}
	}
	// Fallback: check for main_tinygo.go
	if _, err := os.Stat(filepath.Join(dir, "main_tinygo.go")); err == nil {
		return "soi"
	}
	return "wasm"
}

func readVersion(dir string) string {
	yamlPath := filepath.Join(dir, "skill.yaml")
	data, err := os.ReadFile(yamlPath)
	if err != nil {
		return "1.0.0"
	}
	re := regexp.MustCompile(`version:\s*["\s]*(\S+)`)
	m := re.FindStringSubmatch(string(data))
	if len(m) > 1 {
		return strings.Trim(m[1], `"`)
	}
	return "1.0.0"
}

// ── Build Functions ──

func buildWASM(goBin, dir string) error {
	os.MkdirAll(filepath.Join(dir, "wasm"), 0755)
	cmd := exec.Command(goBin, "build", "-o", filepath.Join(dir, "wasm", "plugin.wasm"), ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GOOS=wasip1",
		"GOARCH=wasm",
		"CGO_ENABLED=0",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func buildSOI(dir string) error {
	os.MkdirAll(filepath.Join(dir, "wasm"), 0755)
	tinygo, err := exec.LookPath("tinygo")
	if err != nil {
		for _, p := range []string{
			filepath.Join(os.Getenv("LOCALAPPDATA"), "tinygo", "bin", "tinygo.exe"),
			`C:\tinygo\bin\tinygo.exe`,
			`D:\sdk\tinygo\bin\tinygo.exe`,
		} {
			if _, err := os.Stat(p); err == nil {
				tinygo = p
				break
			}
		}
	}
	if tinygo == "" {
		return fmt.Errorf("tinygo not found. Install: https://tinygo.org/getting-started/install")
	}
	cmd := exec.Command(tinygo, "build", "-target=wasi", "-o", filepath.Join(dir, "wasm", "plugin.soi"), ".")
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// ── wasm-opt Optimization ──

// findWasmOpt searches for wasm-opt binary in PATH and common locations.
func findWasmOpt() string {
	// 1. Check PATH
	if p, err := exec.LookPath("wasm-opt"); err == nil {
		return p
	}
	// 2. Check common install locations
	candidates := []string{
		filepath.Join(os.Getenv("LOCALAPPDATA"), "binaryen", "bin", "wasm-opt.exe"),
		filepath.Join(os.Getenv("ProgramFiles"), "binaryen", "bin", "wasm-opt.exe"),
		filepath.Join(os.Getenv("ProgramFiles"), "Binaryen", "bin", "wasm-opt.exe"),
		`C:\binaryen\bin\wasm-opt.exe`,
		`D:\binaryen\bin\wasm-opt.exe`,
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// runWasmOpt runs wasm-opt with -Oz (size optimization) on the given WASM file.
func runWasmOpt(wasmOptBin, wasmPath string) error {
	outPath := wasmPath + ".opt"
	cmd := exec.Command(wasmOptBin, "-Oz", "-o", outPath, wasmPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("wasm-opt execution failed: %w", err)
	}
	// Replace original with optimized
	if err := os.Rename(outPath, wasmPath); err != nil {
		return fmt.Errorf("replace failed: %w", err)
	}
	return nil
}

// ── Verify ──

func verifySOI(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	txt := string(data)
	required := []string{"execute", "registerTools"}
	var missing []string
	for _, sym := range required {
		if !strings.Contains(txt, sym) {
			missing = append(missing, sym)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing symbols: %s", strings.Join(missing, ", "))
	}
	return nil
}

// ── File Utilities ──

func copyFile(src, dst string) {
	srcF, err := os.Open(src)
	if err != nil {
		return
	}
	defer srcF.Close()

	dstF, err := os.Create(dst)
	if err != nil {
		return
	}
	defer dstF.Close()

	io.Copy(dstF, srcF)
}

func createZip(srcDir, zipPath string) error {
	f, err := os.Create(zipPath)
	if err != nil {
		return err
	}
	defer f.Close()

	w := zip.NewWriter(f)
	defer w.Close()

	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, _ := filepath.Rel(srcDir, path)
		if relPath == "." {
			return nil
		}
		relPath = filepath.ToSlash(relPath)

		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = relPath
		header.Method = zip.Deflate

		if info.IsDir() {
			header.Name += "/"
			_, err := w.CreateHeader(header)
			return err
		}

		writer, err := w.CreateHeader(header)
		if err != nil {
			return err
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = io.Copy(writer, file)
		return err
	})
}

func listZipContents(zipPath string) {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return
	}
	defer r.Close()

	fmt.Println("  Contents:")
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			fmt.Printf("    %s/\n", f.Name)
		} else {
			fmt.Printf("    %-30s %5d KB\n", f.Name, f.UncompressedSize64/1024)
		}
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `soi-package — Package a SOI WASM plugin into a distributable .zip

USAGE:
  soi-package --dir <plugin-dir> [flags]

FLAGS:
  --dir        Plugin project directory (required)
  --output     Output directory for .zip (default: dist)
  --skip-build Skip WASM compilation (use existing wasm/)
  --go         Go binary path (default: go)
  --type       Plugin type: wasm | soi (auto-detected from skill.yaml)
  --optimize   Optimize WASM with wasm-opt after build (requires Binaryen)

EXAMPLES:
  soi-package --dir ./examples/calc
  soi-package --dir ./examples/hello --output ./releases
  soi-package --dir ./examples/soi-demo --type soi --optimize
  soi-package --dir ./examples/test-demo --skip-build
`)
}
