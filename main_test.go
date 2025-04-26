package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGetFunctionsFromSource verifies that C function declarations are correctly parsed
func TestGetFunctionsFromSource(t *testing.T) {
	// Test input with various function signatures
	source := []byte(`
RLAPI void InitWindow(int width, int height, const char *title);  // Initialize window and OpenGL context
RLAPI bool WindowShouldClose(void);                               // Check if KEY_ESCAPE pressed or Close icon pressed
RLAPI void CloseWindow(void);                                     // Close window and unload OpenGL context
RLAPI const char *GetClipboardText(void);                         // Get clipboard text content
RLAPI Vector2 *GetMousePosition(void);                            // Get mouse position XY
RLAPI void DrawLine(int startPosX, int startPosY, int endPosX, int endPosY, Color color); // Draw a line
RLAPI int GetRandomValue(int min, int max);                       // Get a random value between min and max (both included)
	`)

	functions, err := GetFunctionsFromSource(source)
	if err != nil {
		t.Fatalf("Failed to parse source: %v", err)
	}

	// Verify number of functions parsed
	if len(functions) != 7 {
		t.Errorf("Expected 7 functions, got %d", len(functions))
	}

	// Check specific function details
	for _, f := range functions {
		switch f.Name {
		case "InitWindow":
			if f.Type != "RLAPI void" {
				t.Errorf("InitWindow: Expected return type 'RLAPI void', got '%s'", f.Type)
			}
			if len(f.Params) != 3 {
				t.Errorf("InitWindow: Expected 3 parameters, got %d", len(f.Params))
			}
			if !strings.Contains(f.Comment, "Initialize window and OpenGL context") {
				t.Errorf("InitWindow: Comment not properly extracted")
			}

		case "GetMousePosition":
			if f.Type != "RLAPI Vector2 *" {
				t.Errorf("GetMousePosition: Expected return type 'RLAPI Vector2 *', got '%s'", f.Type)
			}
			if len(f.Params) != 0 {
				t.Errorf("GetMousePosition: Expected no parameters")
			}

		case "DrawLine":
			if len(f.Params) != 5 {
				t.Errorf("DrawLine: Expected 5 parameters, got %d", len(f.Params))
			}
			// Check last parameter is Color type
			if f.Params[4].Type != "Color" {
				t.Errorf("DrawLine: Expected last parameter type 'Color', got '%s'", f.Params[4].Type)
			}
		}
	}
}

// TestMapCType verifies C to Go type conversion
func TestMapCType(t *testing.T) {
	testCases := []struct {
		cType    string
		isFunc   bool
		expected string
	}{
		{"void", true, ""},
		{"void", false, "unsafe.Pointer"},
		{"void *", false, "unsafe.Pointer"},
		{"char *", false, "string"},
		{"const char *", false, "string"},
		{"int", false, "int32"},
		{"unsigned int", false, "uint32"},
		{"float", false, "float32"},
		{"double", false, "float64"},
		{"bool", false, "bool"},
		{"char", false, "byte"},
		{"unsigned char", false, "uint8"},
		{"Vector2", false, "Vector2"},
		{"Vector2 *", false, "*Vector2"},
		{"RLAPI int", false, "int32"},
		{"const float", false, "float32"},
	}

	for _, tc := range testCases {
		result := mapCType(tc.cType, tc.isFunc)
		if result != tc.expected {
			t.Errorf("mapCType(%q, %v) = %q, expected %q", tc.cType, tc.isFunc, result, tc.expected)
		}
	}
}

// TestGeneratePureGoShim verifies Go code generation
func TestGeneratePureGoShim(t *testing.T) {
	// Create a temp directory for test output
	tempDir, err := os.MkdirTemp("", "purego-test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer func() {
		os.RemoveAll(tempDir)
	}()

	// Sample functions for code generation
	functions := []Function{
		{
			Name: "InitWindow",
			Type: "void",
			Params: []Param{
				{Type: "int", Name: "width"},
				{Type: "int", Name: "height"},
				{Type: "const char *", Name: "title"},
			},
			Comment: "// Initialize window and OpenGL context",
		},
		{
			Name: "DrawText",
			Type: "void",
			Params: []Param{
				{Type: "const char *", Name: "text"},
				{Type: "int", Name: "posX"},
				{Type: "int", Name: "posY"},
				{Type: "int", Name: "fontSize"},
				{Type: "Color", Name: "color"},
			},
			Comment: "// Draw text (using default font)",
		},
		{
			Name: "GetRandomValue",
			Type: "int",
			Params: []Param{
				{Type: "int", Name: "min"},
				{Type: "int", Name: "max"},
			},
			Comment: "// Get a random value between min and max (both included)",
		},
	}

	// Generate code for test.h
	err = GeneratePureGoShim(functions, tempDir, "test.h", "raylib")
	if err != nil {
		t.Fatalf("Failed to generate code: %v", err)
	}

	// Check if output file exists
	outputFile := filepath.Join(tempDir, "test.go")
	if _, err := os.Stat(outputFile); os.IsNotExist(err) {
		t.Fatalf("Output file not created: %s", outputFile)
	}

	// Read generated code
	codeBytes, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}
	code := string(codeBytes)

	// Verify generated code contains expected patterns
	expectedPatterns := []string{
		"package raylib",
		"import (",
		"\"unsafe\"",
		"\"github.com/ebitengine/purego\"",
		"var InitWindow func(width int32, height int32, title string)",
		"var DrawText func(text string, posX int32, posY int32, fontSize int32, color Color)",
		"var GetRandomValue func(min int32, max int32) int32",
		"func Inittest(handle uintptr) {",
		"purego.RegisterLibFunc(&InitWindow, handle, \"InitWindow\")",
		"purego.RegisterLibFunc(&DrawText, handle, \"DrawText\")",
		"purego.RegisterLibFunc(&GetRandomValue, handle, \"GetRandomValue\")",
	}

	for _, pattern := range expectedPatterns {
		if !strings.Contains(code, pattern) {
			t.Errorf("Generated code doesn't contain expected pattern: %s", pattern)
		}
	}
}

// TestReservedWords ensures reserved Go keywords are properly handled
func TestReservedWords(t *testing.T) {
	functions := []Function{
		{
			Name: "TestFunction",
			Type: "void",
			Params: []Param{
				{Type: "int", Name: "type"},    // "type" is a reserved Go keyword
				{Type: "float", Name: "range"}, // "range" is a reserved Go keyword
				{Type: "int", Name: "normal"},
			},
		},
	}

	// Create a temp directory for test output
	tempDir, err := os.MkdirTemp("", "purego-test-reserved")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer func() {
		os.RemoveAll(tempDir)
	}()

	err = GeneratePureGoShim(functions, tempDir, "reserved.h", "test")
	if err != nil {
		t.Fatalf("Failed to generate code: %v", err)
	}

	// Read generated code
	outputFile := filepath.Join(tempDir, "reserved.go")
	codeBytes, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}
	code := string(codeBytes)

	// Check that reserved keywords are prefixed with underscore
	if !strings.Contains(code, "_type int32") {
		t.Error("Reserved keyword 'type' not properly handled")
	}
	if !strings.Contains(code, "_range float32") {
		t.Error("Reserved keyword 'range' not properly handled")
	}
	if !strings.Contains(code, "normal int32") {
		t.Error("Normal parameter name was unnecessarily modified")
	}
}
