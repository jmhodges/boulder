package main

import (
	"go/ast"
	"go/token"

	"golang.org/x/tools/go/types"
)

// imports is the canonical map of imported packages we need for typechecking.
// It is created during initialization.
var imports = make(map[string]*types.Package)

func (pkg *Package) check(fs *token.FileSet, astFiles []*ast.File) error {
	pkg.defs = make(map[*ast.Ident]types.Object)
	pkg.uses = make(map[*ast.Ident]types.Object)
	pkg.selectors = make(map[*ast.SelectorExpr]*types.Selection)
	pkg.spans = make(map[types.Object]Span)
	pkg.types = make(map[ast.Expr]types.TypeAndValue)
	config := types.Config{
		// We provide the same packages map for all imports to ensure
		// that everybody sees identical packages for the given paths.
		Packages: imports,
		// By providing a Config with our own error function, it will continue
		// past the first error. There is no need for that function to do anything.
		Error: func(error) {},
	}
	info := &types.Info{
		Selections: pkg.selectors,
		Types:      pkg.types,
		Defs:       pkg.defs,
		Uses:       pkg.uses,
	}
	typesPkg, err := config.Check(pkg.path, fs, astFiles, info)
	pkg.typesPkg = typesPkg
	// update spans
	for id, obj := range pkg.defs {
		pkg.growSpan(id, obj)
	}
	for id, obj := range pkg.uses {
		pkg.growSpan(id, obj)
	}
	return err
}

// growSpan expands the span for the object to contain the instance represented
// by the identifier.
func (pkg *Package) growSpan(ident *ast.Ident, obj types.Object) {
	pos := ident.Pos()
	end := ident.End()
	span, ok := pkg.spans[obj]
	if ok {
		if span.min > pos {
			span.min = pos
		}
		if span.max < end {
			span.max = end
		}
	} else {
		span = Span{pos, end}
	}
	pkg.spans[obj] = span
}

// Span stores the minimum range of byte positions in the file in which a
// given variable (types.Object) is mentioned. It is lexically defined: it spans
// from the beginning of its first mention to the end of its last mention.
// A variable is considered shadowed (if *strictShadowing is off) only if the
// shadowing variable is declared within the span of the shadowed variable.
// In other words, if a variable is shadowed but not used after the shadowed
// variable is declared, it is inconsequential and not worth complaining about.
// This simple check dramatically reduces the nuisance rate for the shadowing
// check, at least until something cleverer comes along.
//
// One wrinkle: A "naked return" is a silent use of a variable that the Span
// will not capture, but the compilers catch naked returns of shadowed
// variables so we don't need to.
//
// Cases this gets wrong (TODO):
// - If a for loop's continuation statement mentions a variable redeclared in
// the block, we should complain about it but don't.
// - A variable declared inside a function literal can falsely be identified
// as shadowing a variable in the outer function.
//
type Span struct {
	min token.Pos
	max token.Pos
}
