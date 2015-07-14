package main

import "go/ast"

func init() {
	register("servemux",
		"check that DefaultServeMux is only used by DebugServer",
		checkServeMux,
		funcDecl,
		callExpr)
}

func checkServeMux(f *File, node ast.Node) {
	switch node := node.(type) {
	case *ast.CallExpr:

	default:
		panic("nope" + node)
	}
}
