package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/alecthomas/kingpin/v2"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_c "github.com/tree-sitter/tree-sitter-c/bindings/go"
)

// Command line flags
var (
	pkg        = kingpin.Flag("pkg", "Package the generated files will be attributed").Required().String()
	outDir     = kingpin.Flag("outdir", "Output directory").Required().String()
	inputFiles = kingpin.Arg("files", "Files to generate bindings from").Required().Strings()
)

// Go reserved words that need to be prefixed when used as parameter names
var reservedWords = []string{
	"break", "default", "func", "interface", "select",
	"case", "defer", "go", "map", "struct",
	"chan", "else", "goto", "package", "switch",
	"const", "fallthrough", "if", "range", "type",
	"continue", "for", "import", "return", "var",
	"true", "false", "iota", "nil",
	"append", "cap", "close", "complex", "copy", "delete",
	"imag", "len", "make", "new", "panic",
	"print", "println", "real", "recover",
}

// Function represents a C function declaration to be converted
type Function struct {
	Type    string
	Comment string
	Params  []Param
	Name    string
}

// Param represents a parameter in a function declaration
type Param struct {
	Type string
	Name string
}

// GetFunctionsFromSource parses C source code and extracts function declarations
func GetFunctionsFromSource(source []byte) ([]Function, error) {
	// Initialize tree-sitter parser
	parser := tree_sitter.NewParser()
	defer parser.Close()
	language := tree_sitter.NewLanguage(tree_sitter_c.Language())
	err := parser.SetLanguage(language)
	if err != nil {
		return []Function{}, err
	}

	tree := parser.Parse(source, nil)
	defer tree.Close()

	// Create query to find function declarations in C code
	query, queryErr := tree_sitter.NewQuery(language, `
(declaration
  _* @function.type
  (ERROR)* @function.type.other
  declarator: [
    (function_declarator
      declarator: _ @function.name
      parameters: (parameter_list
        (parameter_declaration
          (_)* @param.type
          declarator: _ @param.name
        )
      )?
    ) @function
    (pointer_declarator
      declarator: (function_declarator
        declarator: _ @function.name
        parameters: (parameter_list
          (parameter_declaration
            (_)* @param.type
            declarator: _ @param.name
          )
        )?
      )
    ) @function.pointer
  ]) @function.declaration
  `)
	if queryErr != nil {
		return nil, queryErr
	}
	defer query.Close()

	// Execute the query and process the results
	qc := tree_sitter.NewQueryCursor()
	defer qc.Close()
	captures := qc.Captures(query, tree.RootNode(), source)
	functions := []Function{}
	functionType := []string{}
	var currentFunction *Function
	var currentParam *Param
	var previousCapture *tree_sitter.QueryCapture

	for match, index := captures.Next(); match != nil; match, index = captures.Next() {
		capture := match.Captures[index]
		captureName := query.CaptureNames()[capture.Index]
		nodeText := capture.Node.Utf8Text(source)

		if captureName == "function.declaration" {
			if currentFunction != nil && (previousCapture == nil || query.CaptureNames()[previousCapture.Index] != "function.declaration") && currentFunction.Name != "" {
				if currentParam != nil {
					currentFunction.Params = append(currentFunction.Params, *currentParam)
					currentParam = nil
				}

				currentFunction.Type = strings.TrimPrefix(strings.Join(slices.Compact(functionType), " "), " ")
				functions = append(functions, *currentFunction)
			}

			clear(functionType)
			currentFunction = &Function{}
			sibling := capture.Node.NextSibling()

			if sibling != nil && sibling.GrammarName() == "comment" {
				currentFunction.Comment = sibling.Utf8Text(source)
			}
		}

		if captureName == "function.type" || captureName == "function.type.other" {
			functionType = append(functionType, nodeText)
		}

		if captureName == "function.pointer" {
			functionType = append(functionType, "*")
		}

		if captureName == "function.name" {
			currentFunction.Name = nodeText
		}

		if captureName == "param.type" {
			if currentParam != nil && currentParam.Name != "" {
				currentFunction.Params = append(currentFunction.Params, *currentParam)
			}

			currentParam = &Param{}
			currentParam.Type = nodeText
		}

		if captureName == "param.name" {
			if currentParam == nil {
				currentParam = &Param{}
			}

			currentParam.Name = nodeText

			if strings.Contains(currentParam.Name, "*") {
				currentParam.Type = currentParam.Type + " *"
				currentParam.Name = strings.ReplaceAll(currentParam.Name, "*", "")
			}
		}

		previousCapture = &capture
	}

	// Add the last parameter and function if they exist
	if currentParam != nil && currentParam.Name != "" {
		currentFunction.Params = append(currentFunction.Params, *currentParam)
	}

	if currentFunction != nil && currentFunction.Name != "" {
		currentFunction.Type = strings.TrimPrefix(strings.Join(slices.Compact(functionType), " "), " ")
		functions = append(functions, *currentFunction)
	}

	return functions, nil
}

// mapCType converts C types to their Go equivalents
func mapCType(ctype string, isFunc bool) string {
	isPointerType := strings.Contains(ctype, "*")
	goType := strings.ReplaceAll(ctype, "RLAPI", "")
	goType = strings.ReplaceAll(goType, "*", "")
	goType = strings.ReplaceAll(goType, "const", "")
	goType = strings.TrimSpace(goType)

	// Handle special cases
	if goType == "void" && isPointerType {
		return "unsafe.Pointer"
	}

	if goType == "char" && isPointerType {
		return "string"
	}

	// Map C types to appropriate Go types
	mappedType := goType
	switch goType {
	// TODO(Kamefrede): Check sizeof long and int
	case "int", "long":
		mappedType = "int32"
	case "unsigned int":
		mappedType = "uint32"
	case "float":
		mappedType = "float32"
	case "double":
		mappedType = "float64"
	case "bool":
		mappedType = "bool"
	case "char":
		mappedType = "byte"
	case "unsigned char":
		mappedType = "uint8"
	case "void":
		if isFunc {
			return ""
		}
		return "unsafe.Pointer"
	}

	if !isPointerType {
		return mappedType
	}

	return "*" + mappedType
}

// GeneratePureGoShim creates Go bindings for C functions using purego
func GeneratePureGoShim(functions []Function, outdir, inputFilePath, packageName string) error {
	var builder strings.Builder

	// Write package declaration and imports
	builder.WriteString(fmt.Sprintf("package %s\n\n", packageName))
	builder.WriteString("import (\n")
	builder.WriteString("\t\"unsafe\"\n")
	builder.WriteString("\t\"github.com/ebitengine/purego\"\n")
	builder.WriteString(")\n\n")

	// Generate function declarations
	for _, f := range functions {
		argTypes := []string{}
		for _, p := range f.Params {
			if slices.Contains(reservedWords, p.Name) {
				p.Name = "_" + p.Name
			}
			argTypes = append(argTypes, fmt.Sprintf("%s %s", p.Name, mapCType(p.Type, false)))
		}
		ret := mapCType(f.Type, true)
		builder.WriteString(fmt.Sprintf("var %s func(%s)", f.Name, strings.Join(argTypes, ", ")))
		if ret != "" {
			builder.WriteString(fmt.Sprintf(" %s\n", ret))
		} else {
			builder.WriteString("\n")
		}
	}

	// Generate initialization function
	baseName := filepath.Base(inputFilePath)
	fileName := baseName[:len(baseName)-len(filepath.Ext(baseName))]

	builder.WriteString(fmt.Sprintf("\nfunc Init%s(handle uintptr) {\n", fileName))
	for _, f := range functions {
		builder.WriteString(fmt.Sprintf("\tpurego.RegisterLibFunc(&%s, handle, \"%s\")\n", f.Name, f.Name))
	}
	builder.WriteString("}\n")

	// Write output file
	outPath := filepath.Join(outdir, fileName+".go")
	if err := os.MkdirAll(outdir, 0o755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	if err := os.WriteFile(outPath, []byte(builder.String()), 0o644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

func main() {
	kingpin.Parse()

	for _, filename := range *inputFiles {
		func() {
			file, err := os.Open(filename)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to open file %s: %v\n", filename, err)
				os.Exit(1)
			}

			defer func() {
				_ = file.Close()
			}()

			fileBytes, err := io.ReadAll(file)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to read file %s: %v\n", filename, err)
				os.Exit(1)
			}

			functions, err := GetFunctionsFromSource(fileBytes)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to get functions from file source %s: %v\n", filename, err)
				os.Exit(1)
			}

			err = GeneratePureGoShim(functions, *outDir, filename, *pkg)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to generate purego shims for file %s: %v\n", filename, err)
				os.Exit(1)
			}
		}()
	}
}
