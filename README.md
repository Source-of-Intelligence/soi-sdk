# SOI SDK

**SOI (Source of Intelligence) Plugin Development Kit** for building WASM plugins.

## Requirements

- Go 1.22+
- (optional) TinyGo for export ABI (.soi)

## Quick Start

```go
package main

import "soi.dev/soi-sdk"

func init() {
    sdk.RegisterToolWithDef(sdk.ToolDef{
        Name:        "add",
        Description: "Add two numbers",
        Parameters:  []sdk.ParamDef{
            {Name: "A", Type: "number", Required: true},
            {Name: "B", Type: "number", Required: true},
        },
        Returns: `{"result": number}`,
    }, func(args json.RawMessage) (interface{}, error) {
        var p struct{ A, B float64 }
        json.Unmarshal(args, &p)
        return map[string]float64{"result": p.A + p.B}, nil
    })
}

func main() { sdk.Run() }
```

## Build

```powershell
# Standard Go WASM
$env:GOOS="wasip1"; $env:GOARCH="wasm"; $env:CGO_ENABLED="0"
go build -o wasm/plugin.wasm .

# TinyGo export ABI
tinygo build -target=wasi -o wasm/plugin.soi .
```

## Verification

```powershell
# Run tests
go test ./...

# CLI verification tool
go run ./cmd/soi-verify --tool add --args '{"A":3,"B":5}'
```

## Project Structure

- `core.go` — Core types, registry, and ExecuteTool
- `run_stdio.go` — Stdio ABI (standard Go wasip1/wasm)
- `host_tinygo.go` — Export ABI (TinyGo //go:build tinygo)
- `manifest.go` — Manifest serialization and skill.yaml generation
- `cmd/soi-package/` — Plugin packaging tool
- `cmd/soi-create/` — Project creation tool (scaffold/wrap)
- `cmd/soi-verify/` — CLI verification tool
- `examples/` — Example plugins
