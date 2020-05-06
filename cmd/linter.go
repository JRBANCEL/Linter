package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

var (
	write = flag.Bool("w", false, "write result to (source) file instead of stdout")

	// The prefixes of the methods that will be linted
	allowed = []string{"Fatal", "Warn", "Sprint", "Print", "Info", "Debug", "Log", "Error", "Skip"}
)

func usage() {
	fmt.Fprintf(os.Stderr, "usage: linter [flags] [path ...]\n")
	flag.PrintDefaults()
}

func main() {
	flag.Usage = usage
	flag.Parse()

	for i := 0; i < flag.NArg(); i++ {
		path := flag.Arg(i)
		switch dir, err := os.Stat(path); {
		case err != nil:
			log.Fatal("Failed to stat ", path)
		case dir.IsDir():
			walkDir(path)
		default:
			log.Println("Ignoring non-directory ", path)
		}
	}
}

// walkDir walks a directory recursively and calls lintDir on each directory.
func walkDir(root string) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if !info.IsDir() {
			return nil
		}
		if info.Name() == "vendor" {
			return filepath.SkipDir
		}

		errors, err := lintDir(path)
		if err != nil {
			return fmt.Errorf("failed to parse %s: %w", path, err)
		}

		for fPath, fErrors := range errors {
			if *write {
				if err := fixFile(fPath, fErrors); err != nil {
					return fmt.Errorf("failed to fix %s: %w", fPath, err)
				}
			} else {
				for _, e := range fErrors {
					fmt.Println(e)
				}
			}
		}
		return nil
	})
}

// fixFile modifies a file to fix the linting errors.
func fixFile(path string, errors []LintError) error {
	bytes, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}

	var output []byte
	var current = 0
	for _, e := range errors {
		output = append(output, bytes[current:e.funcStart.Offset]...)
		output = append(output, []byte(e.fixFunc(string(bytes[e.funcStart.Offset:e.funcEnd.Offset])))...)
		output = append(output, bytes[e.funcEnd.Offset:e.arg0Start.Offset]...)
		output = append(output, []byte(e.fixArg(string(bytes[e.arg0Start.Offset:e.arg0End.Offset])))...)
		current = e.arg0End.Offset
	}
	output = append(output, bytes[current:]...)

	err = ioutil.WriteFile(path, output, os.ModePerm)
	if err != nil {
		return err
	}
	return nil
}

// LintError describe a linting error
type LintError struct {
	fset *token.FileSet

	funcStart token.Position
	funcEnd   token.Position

	arg0Start token.Position
	arg0End   token.Position

	arg1Start token.Position
	arg1End   token.Position

	fixFunc fixFunc
	fixArg  fixArg
}

func (e LintError) String() string {
	bytes, _ := ioutil.ReadFile(e.funcStart.Filename)
	return fmt.Sprintf("%s:%d -> %s", e.funcStart.Filename, e.funcStart.Line, string(bytes[e.funcStart.Offset:e.arg1End.Offset+1]))
}

type fixArg func(str string) string
type fixFunc func(str string) string

func lintDir(path string) (map[string][]LintError, error) {
	fset := &token.FileSet{}
	output := make(map[string][]LintError)

	pkgs, err := parser.ParseDir(fset, path, nil, parser.AllErrors)
	if err != nil {
		return nil, fmt.Errorf("failed to parse the AST: %w", err)
	}

	for _, pkg := range pkgs {
		for fileName, file := range pkg.Files {
			errors := make([]LintError, 0)
			for _, decl := range file.Decls {
				ast.Walk(visitor(func(node ast.Node) bool {
					return visit(node, &errors, fset)
				}), decl)
			}

			if len(errors) > 0 {
				output[fileName] = errors
			}
		}
	}
	return output, nil
}

// visitor adapts a function to satisfy the ast.Visitor interface.
// The function return whether the walk should proceed into the node's children.
type visitor func(ast.Node) bool

func (v visitor) Visit(node ast.Node) ast.Visitor {
	if v(node) {
		return v
	}
	return nil
}

func visit(node ast.Node, errors *[]LintError, fset *token.FileSet) bool {
	// Must be a function call with two parameters
	ce, ok := node.(*ast.CallExpr)
	if !ok || len(ce.Args) != 2 {
		return true
	}

	se, ok := ce.Fun.(*ast.SelectorExpr)
	if !ok || !shouldFix(se) {
		return true
	}

	// The first parameter must be a string
	a0, ok := ce.Args[0].(*ast.BasicLit)
	if !ok || a0.Kind != token.STRING {
		return true
	}

	// The second parameter must be a variable
	a1, ok := ce.Args[1].(*ast.Ident)
	if !ok {
		return true
	}

	var fixArg fixArg
	var fixFunc fixFunc
	fn := function(se)
	found := false
	for _, c := range allowed {
		if strings.HasPrefix(fn, c) {
			found = true
			break
		}
	}
	if !found || strings.HasSuffix(fn, "ln") {
		return true
	}

	// Hacky but true most of the time
	isTest := variable(se) == "t" || variable(se) == "b"

	if strings.HasSuffix(fn, "f") &&
		unicode.IsUpper([]rune(fn)[0]) &&
		(strings.HasSuffix(a0.Value, "%v\"") ||
			strings.HasSuffix(a0.Value, "%s\"")) {
		fixArg = func(str string) string {
			str = strings.TrimSuffix(str, "%v\"")
			str = strings.TrimSuffix(str, "%s\"")
			str = strings.TrimSuffix(str, " ")
			if !isTest {
				str = str + " "
			}
			return str + "\""
		}
		fixFunc = func(str string) string {
			return strings.TrimSuffix(str, "f")
		}
	} else if unicode.IsUpper([]rune(fn)[0]) {
		fixArg = func(str string) string {
			str = strings.TrimSuffix(str, "\"")
			str = strings.TrimSuffix(str, " ")
			if isTest {
				return str + "\""
			}
			return str + " \""
		}
		fixFunc = func(str string) string {
			return str
		}
	} else {
		return true
	}

	*errors = append(*errors, LintError{
		fset:      fset,
		funcStart: fset.Position(se.Pos()),
		funcEnd:   fset.Position(se.End()),
		arg0Start: fset.Position(a0.Pos()),
		arg0End:   fset.Position(a0.End()),
		arg1Start: fset.Position(a1.Pos()),
		arg1End:   fset.Position(a1.End()),
		fixFunc:   fixFunc,
		fixArg:    fixArg,
	})
	return true
}

func shouldFix(se *ast.SelectorExpr) bool {
	return se != nil &&
		function(se) != "Errorf" &&
		function(se) != "String" &&
		unicode.IsUpper([]rune(function(se))[0])
}

func name(expr ast.Expr) string {
	id, ok := expr.(*ast.Ident)
	if !ok {
		return ""
	}
	return id.Name
}

func variable(expr ast.Expr) string {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok || sel == nil {
		return ""
	}
	return name(sel.X)
}

func function(expr ast.Expr) string {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok || sel == nil {
		return ""
	}
	return name(sel.Sel)
}
