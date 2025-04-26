# Tree-sitter PureGo Bindgen

A Go binding generator tool using Tree-sitter to parse C headers and generate [purego](https://github.com/ebitengine/purego)-based bindings without CGO dependencies.

## Overview

This tool parses C header files using the Tree-sitter parser and generates Go bindings that use purego to dynamically call C library functions. It allows you to create pure Go bindings for C libraries without requiring CGO, making your Go code more portable and easier to cross-compile.

## Features

- Parses C header files using Tree-sitter
- Automatically maps C types to appropriate Go equivalents
- Generates pure Go bindings using purego
- Handles complex function declarations with various parameter types
- Properly manages reserved Go keywords in parameter names

## Requirements

- Go 1.20 or higher
- The C library you want to bind to (as a shared library)

## Installation

```bash
go get github.com/kamefrede/c-purego-bindgen
```

Or clone the repository and build:

```bash
git clone https://github.com/kamefrede/c-purego-bindgen.git
cd c-purego-bindgen
go build
```

## Usage

```bash
./c-purego-bindgen --pkg yourpackagename --outdir ./output input.h
```

### Parameters

- `--pkg`: The Go package name for the generated files
- `--outdir`: The directory to output the generated Go files
- `files`: One or more C header files to generate bindings from

### Example

```bash
./c-purego-bindgen --pkg raylib --outdir ./raylib raylib.h
```

## Generated Code

The generated bindings provide:

1. Function declarations that match the C API
2. Type conversions between C and Go types
3. An initialization function to register the library functions using purego

Example of generated code:

```go
package raylib

import (
	"unsafe"
	"github.com/ebitengine/purego"
)

var InitWindow func(width int32, height int32, title string)
var CloseWindow func()
var WindowShouldClose func() bool

func InitRaylib(handle uintptr) {
	purego.RegisterLibFunc(&InitWindow, handle, "InitWindow")
	purego.RegisterLibFunc(&CloseWindow, handle, "CloseWindow")
	purego.RegisterLibFunc(&WindowShouldClose, handle, "WindowShouldClose")
}
```

## Using the Generated Bindings

```go
package main

import (
    "yourpackage/raylib"
    "github.com/ebitengine/purego"
)

func main() {
    // Load the library
    lib, err := purego.Dlopen("libraylib.so", purego.RTLD_NOW|purego.RTLD_GLOBAL)
    if err != nil {
        panic(err)
    }

    // Initialize the bindings
    raylib.Initraylib(lib)

    // Use the library functions
    raylib.InitWindow(800, 600, "Hello from PureGo!")
    defer raylib.CloseWindow()

    // The rest of the owl.
}
```

## How It Works

1. The tool parses C header files using Tree-sitter to extract function declarations
2. It converts C types to appropriate Go types with smart mapping
3. The generated bindings use purego's dynamic loading capabilities to call C functions

## Contributing

Contributions are welcome! Please feel free to submit issues or pull requests.

## License

This project is open source and available under the [MIT License](LICENSE.MD).
