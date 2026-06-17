// Package sdk provides the SOI plugin development kit for WASM plugins.
package sdk

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// BuildWasm compiles the plugin using standard Go with wasip1 target.
// Output is written to wasm/plugin.wasm.
func BuildWasm(dir string) error {
	os.MkdirAll(filepath.Join(dir, "wasm"), 0755)
	output := filepath.Join(dir, "wasm", "plugin.wasm")
	cmd := exec.Command("go", "build", "-o", output, ".")
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

// BuildTinyGo compiles the plugin using TinyGo with wasi target.
// Output is written to wasm/plugin.soi.
func BuildTinyGo(dir string) error {
	os.MkdirAll(filepath.Join(dir, "wasm"), 0755)
	output := filepath.Join(dir, "wasm", "plugin.soi")
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
	cmd := exec.Command(tinygo, "build", "-target=wasi", "-o", output, ".")
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// BuildRust compiles the plugin using Cargo with wasm32-wasip1 target.
// Output is written to wasm/plugin.wasm.
func BuildRust(dir, name string) error {
	os.MkdirAll(filepath.Join(dir, "wasm"), 0755)
	output := filepath.Join(dir, "wasm", "plugin.wasm")

	// First ensure wasm32-wasip1 target is installed
	cmd := exec.Command("rustup", "target", "add", "wasm32-wasip1")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to add wasm32-wasip1 target: %w", err)
	}

	// Patch Cargo.toml: if a local soi-sdk-rs exists, prefer path dependency over git
	cargoPath := filepath.Join(dir, "Cargo.toml")
	// Try to find soi-sdk-rs in parent directories (walking up from plugin dir)
	var localSdkPath string
	for p := filepath.Dir(dir); p != filepath.Dir(p); p = filepath.Dir(p) {
		if _, err := os.Stat(filepath.Join(p, "soi-sdk-rs")); err == nil {
			localSdkPath = filepath.Join(p, "soi-sdk-rs")
			break
		}
	}
	if localSdkPath == "" {
		// Try next to soi-sdk (the package tool itself)
		if _, err := os.Stat(filepath.Join(filepath.Dir(filepath.Dir(dir)), "soi-sdk-rs")); err == nil {
			localSdkPath = filepath.Join(filepath.Dir(filepath.Dir(dir)), "soi-sdk-rs")
		}
	}
	if localSdkPath != "" {
		// Backup original
		origData, err := os.ReadFile(cargoPath)
		if err != nil {
			return fmt.Errorf("read Cargo.toml: %w", err)
		}
		cargoContent := string(origData)
		// Replace soi-sdk git dep with path dep
		cargoContent = strings.Replace(cargoContent,
			`soi-sdk = { git = "https://github.com/Source-of-Intelligence/soi-sdk-rs" }`,
			`soi-sdk = { path = "`+filepath.ToSlash(localSdkPath)+`" }`,
			-1)
		if err := os.WriteFile(cargoPath, []byte(cargoContent), 0644); err != nil {
			return fmt.Errorf("patch Cargo.toml: %w", err)
		}
		// Restore on exit
		defer func() {
			os.WriteFile(cargoPath, origData, 0644)
		}()
	}

	// Build the Rust project with host toolchain detection.
	// On Windows, the GNU toolchain (e.g. TDM-GCC) often ships without
	// libgcc_eh.a, which breaks linking of build scripts and proc macros.
	// These host-side binaries never end up in the WASM output — only the
	// wasm32-wasip1 target matters for the final plugin.  So when the GNU
	// host linker is broken we transparently retry under MSVC, which is
	// always available in a standard Rust installation on Windows.
	buildArgs := []string{"build", "--release", "--target", "wasm32-wasip1"}

	// Try with default toolchain first
	err := runCargoBuild(dir, buildArgs, nil)
	if err != nil {
		// Detect GNU linker failure and retry with MSVC host on Windows
		if isGNULinkerError(err) {
			fmt.Println()
			fmt.Println("  NOTICE: GNU linker (gcc_eh) missing — retrying with MSVC host toolchain...")
			fmt.Println()

			// Ensure MSVC target is installed
			msvcCheck := exec.Command("rustup", "target", "add", "x86_64-pc-windows-msvc")
			msvcCheck.Stdout = os.Stdout
			msvcCheck.Stderr = os.Stderr
			msvcCheck.Run() // best-effort; if it fails cargo will report

			// Force host to MSVC by using +stable-msvc or explicit HOST toolchain.
			// The cleanest approach is to override the default toolchain via rustup.
			// We set RUSTUP_TOOLCHAIN to select the MSVC host while still targeting wasm32-wasip1.
			// Fallback: try +x86_64-pc-windows-msvc toolchain prefix.
			msvcArgs := []string{"+stable-x86_64-pc-windows-msvc"}
			msvcArgs = append(msvcArgs, buildArgs...)

			// Ensure the MSVC toolchain itself is installed
			installCmd := exec.Command("rustup", "toolchain", "install", "stable-x86_64-pc-windows-msvc")
			installCmd.Stdout = os.Stdout
			installCmd.Stderr = os.Stderr
			if err := installCmd.Run(); err != nil {
				// Try without "stable-" prefix — just use the installed MSVC target
				msvcArgs = []string{"+x86_64-pc-windows-msvc"}
				msvcArgs = append(msvcArgs, buildArgs...)
			}

			if err := runCargoBuild(dir, msvcArgs, nil); err != nil {
				// Last-ditch: try the default cargo but force-host to MSVC via
				// CARGO_TARGET_*_LINKER and a RUSTUP override — most users
				// already have link.exe available through the Windows SDK.
				msvcEnv := append(os.Environ(),
					"CARGO_HOST_TARGET=x86_64-pc-windows-msvc",
				)
				if err := runCargoBuild(dir, buildArgs, msvcEnv); err != nil {
					return fmt.Errorf("cargo build failed (GNU linker broken, MSVC fallback also failed): %w\n  HINT: run: rustup default stable-x86_64-pc-windows-msvc", err)
				}
			}
		} else {
			return fmt.Errorf("cargo build failed: %w", err)
		}
	}

	// Copy the output to wasm/plugin.wasm
	// Try multiple naming conventions:
	// 1. original-name.wasm (e.g. rust-hello.wasm)
	// 2. underscore_name.wasm (e.g. rust_hello.wasm) — most common for cdylib
	// 3. libunderscore_name.wasm (e.g. librust_hello.wasm) — fallback for some platforms
	src := filepath.Join(dir, "target", "wasm32-wasip1", "release", name+".wasm")
	if _, err := os.Stat(src); err != nil {
		underscoreName := strings.ReplaceAll(name, "-", "_")
		src = filepath.Join(dir, "target", "wasm32-wasip1", "release", underscoreName+".wasm")
	}
	if _, err := os.Stat(src); err != nil {
		underscoreName := strings.ReplaceAll(name, "-", "_")
		src = filepath.Join(dir, "target", "wasm32-wasip1", "release", "lib"+underscoreName+".wasm")
	}
	return copyFile(src, output)
}

// runCargoBuild runs cargo build with the given args and optional env overrides.
func runCargoBuild(dir string, args []string, env []string) error {
	cmd := exec.Command("cargo", args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if env != nil {
		cmd.Env = env
	}
	return cmd.Run()
}

// isGNULinkerError reports whether err indicates a missing libgcc_eh / GNU
// linker failure (e.g. TDM-GCC without the exception handling library).
func isGNULinkerError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	matches := []string{"gcc_eh", "libgcc_eh", "cannot find -lgcc_eh", "ld.exe:", "x86_64-w64-mingw32-gcc"}
	for _, m := range matches {
		if strings.Contains(msg, m) {
			return true
		}
	}
	return false
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}
