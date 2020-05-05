package main

import (
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

const (
	//root = "/home/jrb/go/src/github.com/JRBANCEL/Experimental/FmtLinter/foo"
	root = "/home/jrb/go/src/knative.dev/serving"
)

var (
	funcs = map[string]bool{
		"fmt.Printf": true,
	}
)

func main() {
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if !info.IsDir() {
			return nil
		}
		if info.Name() == "vendor" {
			return filepath.SkipDir
		}

		//log.Println(info.Name(), path)
		errors, err := lintDir(path)
		if err != nil {
			log.Println(err)
		}

		for p, e := range errors {
			if err := FixFile(p, e); err != nil {
				log.Println(err)
			}
		}
		return nil
	})
}

func FixFile(path string, errors []LintError) error {
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

	fixFunc FixFunc
	fixArg  FixArg
}

func (e LintError) String() string {
	//pos := e.fset.Position(e.arg0Start)
	//fd, _ := os.Open(pos.Filename)
	//defer fd.Close()

	//fd.Seek(int64(e.arg0Start), io.SeekStart)
	//io.Re
	//bytes, _ := ioutil.ReadFile(pos.Filename)
	return "" //fmt.Sprintf("%s:%d -> %s", pos.Filename, pos.Line, string(bytes[pos.Offset:e.fset.Position(e.arg0End).Offset]))
}

func (e LintError) Modify() error {
	// TODO: this is super inefficient, it should be streamed

	return nil
}

type FixArg func(str string) string
type FixFunc func(str string) string

// func FixArg(str string) string {
// 	// Trim trailing "
// 	str = str[:len(str)-1]
//
// 	// Trim %v or %s if any
// 	if strings.HasSuffix(str, "%v") || strings.HasSuffix(str, "%s") {
// 		str = str[:len(str)-2]
// 	}
//
// 	// A space must be present at the end
// 	if str[len(str)-1] != ' ' {
// 		str = str + " "
// 	}
// 	return str + "\""
// }
//
// func FixFunc(str string) string {
// 	runes := []rune(str)
// 	if runes[len(runes)-1] == 'f' {
// 		runes = runes[:len(runes)-1]
// 	}
// 	return string(runes)
// }

func lintDir(path string) (map[string][]LintError, error) {
	fset := &token.FileSet{}
	output := make(map[string][]LintError)

	pkgs, err := parser.ParseDir(fset, path, nil, parser.AllErrors)
	if err != nil {
		return nil, fmt.Errorf("failed to parse the AST in %q: %w", path, err)
	}

	for _, pkg := range pkgs {
		for fileName, file := range pkg.Files {
			//conf := types.Config{Importer: importer.Default()}
			//info := types.Info{Types: make(map[ast.Expr]types.TypeAndValue)}
			//_, err = conf.Check(pkgName, fset, []*ast.File{file}, &info)
			//if err != nil {
			//	return nil, fmt.Errorf("failed to parse the types in %q: %w", fileName, err)
			//}

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
	_, ok = ce.Args[1].(*ast.Ident)
	if !ok {
		return true
	}

	var fixArg FixArg
	var fixfunc FixFunc
	fn := function(se)
	check := []string{"Fatal", "Warn", "Sprint", "Print", "Info", "Debug", "Log", "Error", "Skip"}
	found := false
	for _, c := range check {
		if strings.HasPrefix(fn, c) {
			found = true
			break
		}
	}
	if !found || strings.HasSuffix(fn, "ln") {
		return true
	}

	//if pkg(se) == "" {
	//	log.Println(fset.Position(se.Pos()).Filename, fset.Position(se.Pos()).Line)
	//}

	isTest := pkg(se) == "t" || pkg(se) == "b"

	//fmt.Println(pkg(se), fn)
	if strings.HasSuffix(fn, "f") &&
		unicode.IsUpper([]rune(fn)[0]) &&
		(strings.HasSuffix(a0.Value, "%v\"") ||
			strings.HasSuffix(a0.Value, "%s\"")) {
		fixArg = func(str string) string {
			str = strings.TrimSuffix(str, "%v\"")
			str = strings.TrimSuffix(str, "%s\"")
			if isTest {
				str = strings.TrimSuffix(str, " ")
			}
			return str + "\""
		}
		fixfunc = func(str string) string {
			return strings.TrimSuffix(str, "f")
		}
	} else if !strings.HasSuffix(fn, "f") &&
		unicode.IsUpper([]rune(fn)[0]) {
		fixArg = func(str string) string {
			str = strings.TrimSuffix(str, "\"")
			str = strings.TrimSuffix(str, " ")
			if isTest {
				return  str + "\""
			}
			return str + " \""
		}
		fixfunc = func(str string) string {
			return str
		}
	} else {
		return true
	}

	//t, ok := info.Types[a1].Type.(*types.Basic)
	//isString := ok && t.Kind() == types.String

	//log.Println(ce)
	//isErrorsNew := isPkgDot(ce.Fun, "errors", "New")
	//var isTestingError bool
	// TODO: check pkg and method name

	//pos := fset.Position(a0.Pos())
	//end := fset.Position(a0.End())
	//log.Printf("File: %s, Line: %d, Char: %d->%d, Pkg: %s, Name: %s, String: %t", pos.Filename, pos.Line, pos.Column, end.Column, pkg2(se), function(se), isString)
	//pos = fset.Position(a1.Pos())
	//end = fset.Position(a1.End())
	//log.Printf("File: %s, Line: %d, Char: %d->%d, Pkg: %s, Name: %s, String: %t", pos.Filename, pos.Line, pos.Column, end.Column, pkg2(se), function(se), isString)
	*errors = append(*errors, LintError{
		fset:      fset,
		funcStart: fset.Position(se.Pos()),
		funcEnd:   fset.Position(se.End()),
		arg0Start: fset.Position(a0.Pos()),
		arg0End:   fset.Position(a0.End()),
		fixFunc:   fixfunc,
		fixArg:    fixArg,
		//arg1Start: fset.Position(a1.Pos()),
		//arg1End:   fset.Position(a1.End()),
		//isString:  isString,
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

func pkg(expr ast.Expr) string {
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
