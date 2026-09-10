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

// CheckRebaseline applies rules that become mandatory for code introduced after
// the hosted V1 rebaseline. The superseded implementation remains executable
// during Phase 1, so callers opt new source into these rules until it is replaced.
func CheckRebaseline(files []SourceFile) []Violation {
	violations := Check(files)
	for _, file := range files {
		if strings.HasSuffix(file.Path, ".go") {
			violations = append(violations, checkRebaselineGo(file)...)
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
		if err == nil && looksLikeSQL(value) {
			if line := joinLine(value); line > 0 {
				violations = append(violations, Violation{Rule: "production SQL must not contain JOIN", File: file.Path, Line: parsedLine(file.Content, literal.Value)})
			}
		}
		return true
	})
	return violations
}

func looksLikeSQL(source string) bool {
	trimmed := strings.TrimSpace(strings.ToUpper(source))
	for _, prefix := range []string{"SELECT ", "INSERT ", "UPDATE ", "DELETE ", "CREATE ", "ALTER ", "DROP ", "WITH "} {
		if strings.HasPrefix(trimmed, prefix) {
			return true
		}
	}
	return false
}

func checkRebaselineGo(file SourceFile) []Violation {
	parsed, err := parser.ParseFile(token.NewFileSet(), file.Path, file.Content, parser.ParseComments)
	if err != nil {
		return nil
	}
	var violations []Violation
	ast.Inspect(parsed, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.Field:
			for _, name := range value.Names {
				if name.Name == "IsAdmin" || name.Name == "Admin" || name.Name == "PlatformOperator" {
					violations = append(violations, Violation{Rule: "admin flags are forbidden; use role bindings and typed actions", File: file.Path, Line: parsedLine(file.Content, name.Name)})
				}
			}
		case *ast.FuncDecl:
			if strings.HasSuffix(file.Path, "handler.go") && functionCallsAuthorization(value.Body) {
				violations = append(violations, Violation{Rule: "HTTP handlers must not authorize; use authentication and authorization filters", File: file.Path, Line: parsedLine(file.Content, "func "+value.Name.Name)})
			}
		case *ast.ForStmt:
			if blockCallsSQL(value.Body) {
				violations = append(violations, Violation{Rule: "per-resource SQL loops are forbidden; batch IDs before evaluating in Go", File: file.Path, Line: parsedLine(file.Content, "for")})
			}
		case *ast.RangeStmt:
			if blockCallsSQL(value.Body) {
				violations = append(violations, Violation{Rule: "per-resource SQL loops are forbidden; batch IDs before evaluating in Go", File: file.Path, Line: parsedLine(file.Content, "range")})
			}
		case *ast.BasicLit:
			if value.Kind != token.STRING {
				break
			}
			text, unquoteErr := strconv.Unquote(value.Value)
			if unquoteErr == nil && isWorldRoute(text) {
				violations = append(violations, Violation{Rule: "World routes are outside hosted V1", File: file.Path, Line: parsedLine(file.Content, value.Value)})
			}
		case *ast.TypeSpec:
			switch value.Name.Name {
			case "Plan", "PlanVersion", "Order", "Payment", "Entitlement":
				violations = append(violations, Violation{Rule: "legacy commerce model is forbidden in rebaseline production code", File: file.Path, Line: parsedLine(file.Content, value.Name.Name)})
			}
		}
		return true
	})
	return violations
}

func isWorldRoute(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	return strings.HasPrefix(normalized, "/v1/") && strings.Contains(normalized, "/world")
}

func functionCallsAuthorization(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		name := calledName(call.Fun)
		if strings.Contains(strings.ToLower(name), "authoriz") || strings.Contains(strings.ToLower(name), "requirepermission") {
			found = true
			return false
		}
		return true
	})
	return found
}

func blockCallsSQL(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch calledName(call.Fun) {
		case "Query", "QueryContext", "QueryRow", "QueryRowContext", "Exec", "ExecContext":
			found = true
			return false
		default:
			return true
		}
	})
	return found
}

func calledName(expression ast.Expr) string {
	switch value := expression.(type) {
	case *ast.Ident:
		return value.Name
	case *ast.SelectorExpr:
		return value.Sel.Name
	default:
		return ""
	}
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
