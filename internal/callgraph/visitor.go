package callgraph

import (
	"fmt"
	"log"
	"path/filepath"
	"strings"

	"github.com/VKCOM/php-parser/pkg/ast"
	"github.com/VKCOM/php-parser/pkg/position"
	"github.com/VKCOM/php-parser/pkg/visitor"
	"github.com/VKCOM/php-parser/pkg/visitor/traverser"
	"github.com/hightemp/php-call-graph-viz/internal/phpparse"
)

// BuildCallGraph constructs the call graph from all parsed files.
func BuildCallGraph(
	results []*phpparse.ParseResult,
	symbols *phpparse.SymbolTable,
	typeMappings map[string]string,
	resolveTypes bool,
	showFunctions bool,
) *CallGraph {
	cg := NewCallGraph()

	for _, res := range results {
		v := &callVisitor{
			cg:            cg,
			symbols:       symbols,
			resolvedNames: res.ResolvedNames,
			typeResolver:  NewTypeResolver(symbols, res.ResolvedNames, typeMappings),
			resolveTypes:  resolveTypes,
			showFunctions: showFunctions,
			file:          res.FilePath,
		}

		tv := traverser.NewTraverser(v)
		res.Root.Accept(tv)
	}

	log.Printf("[callgraph] Built graph: %s", cg.Stats())
	return cg
}

// callVisitor is the AST visitor that extracts call edges.
type callVisitor struct {
	visitor.Null

	cg            *CallGraph
	symbols       *phpparse.SymbolTable
	resolvedNames map[ast.Vertex]string
	typeResolver  *TypeResolver
	resolveTypes  bool
	showFunctions bool
	file          string

	// Current context.
	currentClassFQN  string
	currentClassName string
	currentClassKind string
	currentMethod    string
	currentCallerID  NodeID
}

// --- Class / Interface / Trait / Enum entry ---

func (v *callVisitor) StmtClass(n *ast.StmtClass) {
	fqn := v.resolvedNames[n]
	if fqn == "" && n.Name != nil {
		if id, ok := n.Name.(*ast.Identifier); ok {
			fqn = string(id.Value)
		}
	}
	v.enterClass(fqn, "class")
}

func (v *callVisitor) StmtInterface(n *ast.StmtInterface) {
	fqn := v.resolvedNames[n]
	if fqn == "" {
		if id, ok := n.Name.(*ast.Identifier); ok {
			fqn = string(id.Value)
		}
	}
	v.enterClass(fqn, "interface")
}

func (v *callVisitor) StmtTrait(n *ast.StmtTrait) {
	fqn := v.resolvedNames[n]
	if fqn == "" {
		if id, ok := n.Name.(*ast.Identifier); ok {
			fqn = string(id.Value)
		}
	}
	v.enterClass(fqn, "trait")
}

func (v *callVisitor) StmtEnum(n *ast.StmtEnum) {
	fqn := v.resolvedNames[n]
	if fqn == "" {
		if id, ok := n.Name.(*ast.Identifier); ok {
			fqn = string(id.Value)
		}
	}
	v.enterClass(fqn, "enum")
}

func (v *callVisitor) enterClass(fqn, kind string) {
	v.currentClassFQN = fqn
	v.currentClassName = shortName(fqn)
	v.currentClassKind = kind
	v.typeResolver.EnterClass(fqn)
}

// LeaveNode detects when we leave a class or method scope.
func (v *callVisitor) LeaveNode(n ast.Vertex) {
	switch n.(type) {
	case *ast.StmtClass, *ast.StmtInterface, *ast.StmtTrait, *ast.StmtEnum:
		v.currentClassFQN = ""
		v.currentClassName = ""
		v.currentClassKind = ""
		v.typeResolver.LeaveClass()
	case *ast.StmtClassMethod:
		v.currentMethod = ""
		v.currentCallerID = ""
	case *ast.StmtFunction:
		v.currentMethod = ""
		v.currentCallerID = ""
	}
}

// --- Method / Function entry ---

func (v *callVisitor) StmtClassMethod(n *ast.StmtClassMethod) {
	name := ""
	if id, ok := n.Name.(*ast.Identifier); ok {
		name = string(id.Value)
	}
	if name == "" || v.currentClassFQN == "" {
		return
	}

	v.currentMethod = name
	v.currentCallerID = MakeMethodID(v.currentClassFQN, name)

	// Check if static.
	isStatic := false
	for _, mod := range n.Modifiers {
		if id, ok := mod.(*ast.Identifier); ok {
			if strings.EqualFold(string(id.Value), "static") {
				isStatic = true
			}
		}
	}

	// Add caller node.
	v.cg.AddNode(&CallNode{
		ID:        v.currentCallerID,
		Label:     name,
		ClassFQN:  v.currentClassFQN,
		ClassName: v.currentClassName,
		Method:    name,
		IsStatic:  isStatic,
		Kind:      v.currentClassKind,
		File:      v.file,
	})

	// Seed type resolver with parameter types.
	if v.resolveTypes {
		if ci, ok := v.symbols.Classes[v.currentClassFQN]; ok {
			if mi, ok := ci.Methods[name]; ok {
				v.typeResolver.EnterMethod(mi.Params)
			}
		}
	}
}

func (v *callVisitor) StmtFunction(n *ast.StmtFunction) {
	if !v.showFunctions {
		return
	}

	fqn := v.resolvedNames[n]
	if fqn == "" {
		if id, ok := n.Name.(*ast.Identifier); ok {
			fqn = string(id.Value)
		}
	}
	if fqn == "" {
		return
	}

	v.currentMethod = fqn
	v.currentCallerID = MakeFunctionID(fqn)

	v.cg.AddNode(&CallNode{
		ID:     v.currentCallerID,
		Label:  shortName(fqn),
		Method: shortName(fqn),
		Kind:   "function",
		File:   v.file,
	})

	if v.resolveTypes {
		if fi, ok := v.symbols.Functions[fqn]; ok {
			v.typeResolver.EnterMethod(fi.Params)
		}
	}
}

// --- Call expressions ---

func (v *callVisitor) ExprAssign(n *ast.ExprAssign) {
	if !v.resolveTypes {
		return
	}

	// Track $var = new ClassName() assignments.
	varNode, ok := n.Var.(*ast.ExprVariable)
	if !ok {
		return
	}
	varName := phpparse.VariableName(varNode)
	if varName == "" {
		return
	}

	if newExpr, ok := n.Expr.(*ast.ExprNew); ok {
		typeFQN := v.typeResolver.ResolveNewExprType(newExpr.Class)
		if typeFQN != "" {
			v.typeResolver.TrackAssignment(varName, typeFQN)
		}
	}
}

func (v *callVisitor) ExprMethodCall(n *ast.ExprMethodCall) {
	if v.currentCallerID == "" {
		return
	}

	// Extract method name.
	methodName := identifierName(n.Method)
	if methodName == "" {
		// Dynamic method call: $obj->$method()
		v.addUnresolvedEdge("$dynamic", CallDynamic, n.GetPosition())
		return
	}

	// Resolve receiver type.
	receiverType := v.resolveReceiver(n.Var)

	if receiverType != "" {
		v.addMethodCallEdge(receiverType, methodName, CallInstance, n.GetPosition())
	} else {
		// Unresolved receiver.
		v.addUnresolvedEdge(methodName, CallUnresolved, n.GetPosition())
	}
}

func (v *callVisitor) ExprNullsafeMethodCall(n *ast.ExprNullsafeMethodCall) {
	if v.currentCallerID == "" {
		return
	}

	methodName := identifierName(n.Method)
	if methodName == "" {
		v.addUnresolvedEdge("$dynamic", CallDynamic, n.GetPosition())
		return
	}

	receiverType := v.resolveReceiver(n.Var)

	if receiverType != "" {
		v.addMethodCallEdge(receiverType, methodName, CallNullsafe, n.GetPosition())
	} else {
		v.addUnresolvedEdge(methodName, CallUnresolved, n.GetPosition())
	}
}

func (v *callVisitor) ExprStaticCall(n *ast.ExprStaticCall) {
	if v.currentCallerID == "" {
		return
	}

	methodName := identifierName(n.Call)
	if methodName == "" {
		v.addUnresolvedEdge("$dynamic_static", CallDynamic, n.GetPosition())
		return
	}

	// Resolve class.
	classFQN := ""
	if resolved, ok := v.resolvedNames[n.Class]; ok {
		classFQN = v.resolveSpecialClass(resolved)
	} else {
		classFQN = phpparse.VertexToName(n.Class)
		classFQN = v.resolveSpecialClass(classFQN)
	}

	if classFQN != "" {
		v.addMethodCallEdge(classFQN, methodName, CallStatic, n.GetPosition())
	} else {
		v.addUnresolvedEdge(methodName, CallUnresolved, n.GetPosition())
	}
}

func (v *callVisitor) ExprNew(n *ast.ExprNew) {
	if v.currentCallerID == "" {
		return
	}

	classFQN := ""
	if resolved, ok := v.resolvedNames[n.Class]; ok {
		classFQN = v.resolveSpecialClass(resolved)
	} else {
		classFQN = phpparse.VertexToName(n.Class)
	}

	if classFQN != "" {
		v.addMethodCallEdge(classFQN, "__construct", CallNew, n.GetPosition())
	}
}

func (v *callVisitor) ExprFunctionCall(n *ast.ExprFunctionCall) {
	if v.currentCallerID == "" || !v.showFunctions {
		return
	}

	funcFQN := ""
	if resolved, ok := v.resolvedNames[n.Function]; ok {
		funcFQN = resolved
	} else {
		funcFQN = phpparse.VertexToName(n.Function)
	}

	if funcFQN == "" {
		return
	}

	targetID := MakeFunctionID(funcFQN)

	v.cg.AddNode(&CallNode{
		ID:     targetID,
		Label:  shortName(funcFQN),
		Method: shortName(funcFQN),
		Kind:   "function",
	})

	v.cg.AddEdge(&CallEdge{
		From:     v.currentCallerID,
		To:       targetID,
		Type:     CallFunction,
		Label:    shortName(funcFQN) + "()",
		FileLine: posToString(v.file, n.GetPosition()),
	})
}

// --- Internal helpers ---

func (v *callVisitor) resolveReceiver(varNode ast.Vertex) string {
	if !v.resolveTypes {
		// Without type resolution, only resolve $this.
		if exprVar, ok := varNode.(*ast.ExprVariable); ok {
			varName := phpparse.VariableName(exprVar)
			if varName == "this" && v.currentClassFQN != "" {
				return v.currentClassFQN
			}
		}
		return ""
	}

	switch n := varNode.(type) {
	case *ast.ExprVariable:
		varName := phpparse.VariableName(n)
		return v.typeResolver.ResolveVariable(varName)

	case *ast.ExprPropertyFetch:
		// $this->repo->method() — resolve chain.
		if innerVar, ok := n.Var.(*ast.ExprVariable); ok {
			receiverVar := phpparse.VariableName(innerVar)
			propName := identifierName(n.Prop)
			if propName != "" {
				return v.typeResolver.ResolvePropertyFetch(receiverVar, propName)
			}
		}
	}

	return ""
}

func (v *callVisitor) resolveSpecialClass(name string) string {
	lower := strings.ToLower(name)
	switch lower {
	case "self":
		return v.typeResolver.ResolveSelf()
	case "static":
		return v.typeResolver.ResolveStaticKeyword()
	case "parent":
		return v.typeResolver.ResolveParent()
	default:
		return name
	}
}

func (v *callVisitor) addMethodCallEdge(classFQN, methodName string, callType CallType, pos *position.Position) {
	targetID := MakeMethodID(classFQN, methodName)

	// Ensure target node exists with accurate info.
	ci, mi := v.symbols.LookupMethod(classFQN, methodName)
	if mi != nil {
		targetID = MakeMethodID(ci.FQN, mi.Name)
		v.cg.AddNode(&CallNode{
			ID:        targetID,
			Label:     mi.Name,
			ClassFQN:  ci.FQN,
			ClassName: shortName(ci.FQN),
			Method:    mi.Name,
			IsStatic:  mi.IsStatic,
			Kind:      ci.Kind,
			File:      ci.File,
		})
	} else {
		v.cg.AddNode(&CallNode{
			ID:        targetID,
			Label:     methodName,
			ClassFQN:  classFQN,
			ClassName: shortName(classFQN),
			Method:    methodName,
			Kind:      "class",
		})
	}

	v.cg.AddEdge(&CallEdge{
		From:     v.currentCallerID,
		To:       targetID,
		Type:     callType,
		Label:    methodName + "()",
		FileLine: posToString(v.file, pos),
	})
}

func (v *callVisitor) addUnresolvedEdge(label string, callType CallType, pos *position.Position) {
	targetID := NodeID("?::" + label)

	v.cg.AddNode(&CallNode{
		ID:     targetID,
		Label:  label,
		Method: label,
		Kind:   "unresolved",
	})

	v.cg.AddEdge(&CallEdge{
		From:     v.currentCallerID,
		To:       targetID,
		Type:     callType,
		Label:    label,
		FileLine: posToString(v.file, pos),
	})
}

func identifierName(v ast.Vertex) string {
	if id, ok := v.(*ast.Identifier); ok {
		return string(id.Value)
	}
	return ""
}

func posToString(file string, pos *position.Position) string {
	if pos == nil {
		return ""
	}
	return fmt.Sprintf("%s:%d", filepath.Base(file), pos.StartLine)
}
