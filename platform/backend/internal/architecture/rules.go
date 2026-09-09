package architecture

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
)

type Violation struct {
	Rule string
	File string
	Line int
}

func (v Violation) Error() string {
	return fmt.Sprintf("%s:%d: %s", v.File, v.Line, v.Rule)
}

type SourceFile struct {
	Path    string
	Content string
}

func Check(files []SourceFile) []Violation {
	var violations []Violation
	for _, file := range files {
		switch {
		case strings.HasSuffix(file.Path, ".go"):
			violations = append(violations, checkGo(file)...)
		case strings.HasSuffix(file.Path, ".sql"):
			if line := joinLine(file.Content); line > 0 {
				violations = append(violations, Violation{Rule: "production SQL must not contain JOIN", File: file.Path, Line: line})
			}
		}
	}
	return violations
}

func checkGo(file SourceFile) []Violation {
	parsed, err := parser.ParseFile(token.NewFileSet(), file.Path, file.Content, parser.ParseComments)
	if err != nil {
		return []Violation{{Rule: "Go source must parse: " + err.Error(), File: file.Path, Line: 1}}
	}
	var violations []Violation
	owner := contextFromPath(file.Path)
	for _, spec := range parsed.Imports {
		importPath, _ := strconv.Unquote(spec.Path.Value)
		if strings.Contains(importPath, "/apps/") {
			violations = append(violations, Violation{Rule: "legacy apps import is forbidden", File: file.Path, Line: parsedLine(file.Content, spec.Path.Value)})
		}
		if owner != "" && strings.Contains(importPath, "/internal/") && strings.Contains(importPath, "/repository") {
			importedOwner := contextFromPath(importPath)
			if importedOwner != "" && importedOwner != owner {
				violations = append(violations, Violation{Rule: "cross-context repository import is forbidden", File: file.Path, Line: parsedLine(file.Content, spec.Path.Value)})
			}
		}
	}
	ast.Inspect(parsed, func(node ast.Node) bool {
		literal, ok := node.(*ast.BasicLit)
		if !ok || literal.Kind != token.STRING {
			return true
		}
		value, err := strconv.Unquote(literal.Value)
		if err == nil {
			if line := joinLine(value); line > 0 {
				violations = append(violations, Violation{Rule: "production SQL must not contain JOIN", File: file.Path, Line: parsedLine(file.Content, literal.Value)})
			}
		}
		return true
	})
	return violations
}

func contextFromPath(path string) string {
	normalized := strings.ReplaceAll(path, "\\", "/")
	marker := "/internal/"
	index := strings.Index(normalized, marker)
	if index < 0 {
		return ""
	}
	remainder := normalized[index+len(marker):]
	contextName, _, _ := strings.Cut(remainder, "/")
	return contextName
}

func joinLine(source string) int {
	scanner := bufio.NewScanner(strings.NewReader(source))
	for line := 1; scanner.Scan(); line++ {
		for _, word := range strings.FieldsFunc(scanner.Text(), func(r rune) bool {
			return !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z')
		}) {
			if strings.EqualFold(word, "join") {
				return line
			}
		}
	}
	return 0
}

func parsedLine(source, fragment string) int {
	index := strings.Index(source, fragment)
	if index < 0 {
		return 1
	}
	return strings.Count(source[:index], "\n") + 1
}
