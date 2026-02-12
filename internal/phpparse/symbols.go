package phpparse

import (
	"fmt"
	"log"
	"path/filepath"
	"strings"

	"github.com/VKCOM/php-parser/pkg/ast"
)

// MethodInfo describes a single class/interface/trait method.
type MethodInfo struct {
	Name       string // short name, e.g. "getUser"
	FQN        string // fully qualified: "App\\UserService::getUser"
	IsStatic   bool
	Params     []ParamInfo
	ReturnType string // FQN of return type if available
	Position   string // "file.php:42"
}

// ParamInfo describes a method/function parameter.
type ParamInfo struct {
	Name    string // e.g. "$repo"
	TypeFQN string // e.g. "App\\UserRepository" or "" if untyped
}

// ClassInfo describes a class, interface, trait, or enum.
type ClassInfo struct {
	FQN                string
	Kind               string // "class", "interface", "trait", "enum"
	File               string
	Extends            string   // parent FQN (single for class, empty for trait)
	Implements         []string // interface FQNs
	Traits             []string // trait FQNs used by this class
	Methods            map[string]*MethodInfo
	Properties         map[string]string // property name → type FQN (from type hints)
	HasMagicCall       bool              // has __call
	HasMagicCallStatic bool              // has __callStatic
}

// FunctionInfo describes a standalone (non-method) function.
type FunctionInfo struct {
	FQN        string
	Name       string
	Params     []ParamInfo
	ReturnType string
	File       string
	Position   string
}

// SymbolTable is the global symbol table built from all parsed files.
type SymbolTable struct {
	Classes   map[string]*ClassInfo    // FQN → ClassInfo
	Functions map[string]*FunctionInfo // FQN → FunctionInfo
}

// NewSymbolTable creates an empty symbol table.
func NewSymbolTable() *SymbolTable {
	return &SymbolTable{
		Classes:   make(map[string]*ClassInfo),
		Functions: make(map[string]*FunctionInfo),
	}
}

// BuildSymbolTable constructs a global symbol table from all parse results.
func BuildSymbolTable(results []*ParseResult) *SymbolTable {
	st := NewSymbolTable()

	for _, res := range results {
		collector := &symbolCollector{
			st:            st,
			resolvedNames: res.ResolvedNames,
			file:          res.FilePath,
		}
		collector.collect(res.Root)
	}

	// Resolve trait methods: copy trait methods into using classes.
	st.resolveTraits()

	log.Printf("[symbols] Collected %d classes/interfaces/traits, %d functions",
		len(st.Classes), len(st.Functions))

	return st
}

// LookupMethod finds a method by class FQN and method name,
// traversing the inheritance hierarchy if needed.
func (st *SymbolTable) LookupMethod(classFQN, methodName string) (*ClassInfo, *MethodInfo) {
	visited := make(map[string]bool)
	return st.lookupMethodRecursive(classFQN, methodName, visited)
}

func (st *SymbolTable) lookupMethodRecursive(classFQN, methodName string, visited map[string]bool) (*ClassInfo, *MethodInfo) {
	if visited[classFQN] {
		return nil, nil
	}
	visited[classFQN] = true

	ci, ok := st.Classes[classFQN]
	if !ok {
		return nil, nil
	}

	if mi, ok := ci.Methods[methodName]; ok {
		return ci, mi
	}

	// Check parent class.
	if ci.Extends != "" {
		if foundCI, foundMI := st.lookupMethodRecursive(ci.Extends, methodName, visited); foundMI != nil {
			return foundCI, foundMI
		}
	}

	// Check interfaces (for default implementations in the future).
	for _, iface := range ci.Implements {
		if foundCI, foundMI := st.lookupMethodRecursive(iface, methodName, visited); foundMI != nil {
			return foundCI, foundMI
		}
	}

	return nil, nil
}

// resolveTraits copies methods from traits into classes that use them.
func (st *SymbolTable) resolveTraits() {
	for _, ci := range st.Classes {
		for _, traitFQN := range ci.Traits {
			traitInfo, ok := st.Classes[traitFQN]
			if !ok {
				continue
			}
			for name, mi := range traitInfo.Methods {
				if _, exists := ci.Methods[name]; !exists {
					// Copy trait method into the class.
					copied := *mi
					copied.FQN = ci.FQN + "::" + name
					ci.Methods[name] = &copied
				}
			}
		}
	}
}

// symbolCollector walks the AST of a single file and populates the symbol table.
type symbolCollector struct {
	st            *SymbolTable
	resolvedNames map[ast.Vertex]string
	file          string

	currentClass *ClassInfo
}

func (sc *symbolCollector) collect(root ast.Vertex) {
	rootNode, ok := root.(*ast.Root)
	if !ok {
		return
	}
	for _, stmt := range rootNode.Stmts {
		sc.walkStmt(stmt)
	}
}

func (sc *symbolCollector) walkStmt(node ast.Vertex) {
	switch n := node.(type) {
	case *ast.StmtNamespace:
		for _, s := range n.Stmts {
			sc.walkStmt(s)
		}

	case *ast.StmtClass:
		sc.processClass(n, "class")

	case *ast.StmtInterface:
		sc.processInterface(n)

	case *ast.StmtTrait:
		sc.processTrait(n)

	case *ast.StmtEnum:
		sc.processEnum(n)

	case *ast.StmtFunction:
		sc.processFunction(n)

	case *ast.StmtExpression:
		// skip

	case *ast.StmtStmtList:
		for _, s := range n.Stmts {
			sc.walkStmt(s)
		}
	}
}

func (sc *symbolCollector) processClass(n *ast.StmtClass, kind string) {
	fqn := sc.resolvedNames[n]
	if fqn == "" && n.Name != nil {
		if id, ok := n.Name.(*ast.Identifier); ok {
			fqn = string(id.Value)
		}
	}
	if fqn == "" {
		// Anonymous class — generate synthetic name.
		pos := n.GetPosition()
		if pos != nil {
			fqn = anonymousName(sc.file, pos.StartLine)
		} else {
			fqn = anonymousName(sc.file, 0)
		}
	}

	ci := &ClassInfo{
		FQN:        fqn,
		Kind:       kind,
		File:       sc.file,
		Methods:    make(map[string]*MethodInfo),
		Properties: make(map[string]string),
	}

	// Extends.
	if n.Extends != nil {
		if resolved, ok := sc.resolvedNames[n.Extends]; ok {
			ci.Extends = resolved
		} else {
			ci.Extends = VertexToName(n.Extends)
		}
	}

	// Implements.
	for _, iface := range n.Implements {
		if resolved, ok := sc.resolvedNames[iface]; ok {
			ci.Implements = append(ci.Implements, resolved)
		} else {
			ci.Implements = append(ci.Implements, VertexToName(iface))
		}
	}

	// Save to table before processing members (methods may reference class).
	sc.st.Classes[fqn] = ci

	prevClass := sc.currentClass
	sc.currentClass = ci

	for _, stmt := range n.Stmts {
		sc.processClassMember(stmt)
	}

	sc.currentClass = prevClass
}

func (sc *symbolCollector) processInterface(n *ast.StmtInterface) {
	fqn := sc.resolvedNames[n]
	if fqn == "" {
		if id, ok := n.Name.(*ast.Identifier); ok {
			fqn = string(id.Value)
		}
	}

	ci := &ClassInfo{
		FQN:        fqn,
		Kind:       "interface",
		File:       sc.file,
		Methods:    make(map[string]*MethodInfo),
		Properties: make(map[string]string),
	}

	for _, ext := range n.Extends {
		if resolved, ok := sc.resolvedNames[ext]; ok {
			ci.Implements = append(ci.Implements, resolved)
		} else {
			ci.Implements = append(ci.Implements, VertexToName(ext))
		}
	}

	sc.st.Classes[fqn] = ci

	prevClass := sc.currentClass
	sc.currentClass = ci

	for _, stmt := range n.Stmts {
		sc.processClassMember(stmt)
	}

	sc.currentClass = prevClass
}

func (sc *symbolCollector) processTrait(n *ast.StmtTrait) {
	fqn := sc.resolvedNames[n]
	if fqn == "" {
		if id, ok := n.Name.(*ast.Identifier); ok {
			fqn = string(id.Value)
		}
	}

	ci := &ClassInfo{
		FQN:        fqn,
		Kind:       "trait",
		File:       sc.file,
		Methods:    make(map[string]*MethodInfo),
		Properties: make(map[string]string),
	}

	sc.st.Classes[fqn] = ci

	prevClass := sc.currentClass
	sc.currentClass = ci

	for _, stmt := range n.Stmts {
		sc.processClassMember(stmt)
	}

	sc.currentClass = prevClass
}

func (sc *symbolCollector) processEnum(n *ast.StmtEnum) {
	fqn := sc.resolvedNames[n]
	if fqn == "" {
		if id, ok := n.Name.(*ast.Identifier); ok {
			fqn = string(id.Value)
		}
	}

	ci := &ClassInfo{
		FQN:        fqn,
		Kind:       "enum",
		File:       sc.file,
		Methods:    make(map[string]*MethodInfo),
		Properties: make(map[string]string),
	}

	for _, iface := range n.Implements {
		if resolved, ok := sc.resolvedNames[iface]; ok {
			ci.Implements = append(ci.Implements, resolved)
		} else {
			ci.Implements = append(ci.Implements, VertexToName(iface))
		}
	}

	sc.st.Classes[fqn] = ci

	prevClass := sc.currentClass
	sc.currentClass = ci

	for _, stmt := range n.Stmts {
		sc.processClassMember(stmt)
	}

	sc.currentClass = prevClass
}

func (sc *symbolCollector) processClassMember(node ast.Vertex) {
	switch n := node.(type) {
	case *ast.StmtClassMethod:
		sc.processMethod(n)

	case *ast.StmtTraitUse:
		for _, trait := range n.Traits {
			if resolved, ok := sc.resolvedNames[trait]; ok {
				sc.currentClass.Traits = append(sc.currentClass.Traits, resolved)
			} else {
				sc.currentClass.Traits = append(sc.currentClass.Traits, VertexToName(trait))
			}
		}

	case *ast.StmtPropertyList:
		sc.processPropertyList(n)
	}
}

func (sc *symbolCollector) processMethod(n *ast.StmtClassMethod) {
	if sc.currentClass == nil {
		return
	}

	name := ""
	if id, ok := n.Name.(*ast.Identifier); ok {
		name = string(id.Value)
	}
	if name == "" {
		return
	}

	mi := &MethodInfo{
		Name:   name,
		FQN:    sc.currentClass.FQN + "::" + name,
		Params: sc.extractParams(n.Params),
	}

	// Check if static.
	for _, mod := range n.Modifiers {
		if id, ok := mod.(*ast.Identifier); ok {
			if strings.EqualFold(string(id.Value), "static") {
				mi.IsStatic = true
			}
		}
	}

	// Return type.
	if n.ReturnType != nil {
		if resolved, ok := sc.resolvedNames[n.ReturnType]; ok {
			mi.ReturnType = resolved
		} else {
			mi.ReturnType = VertexToName(n.ReturnType)
		}
	}

	pos := n.GetPosition()
	if pos != nil {
		mi.Position = formatPosition(sc.file, pos.StartLine)
	}

	sc.currentClass.Methods[name] = mi

	// Detect magic methods.
	if name == "__call" {
		sc.currentClass.HasMagicCall = true
	}
	if name == "__callStatic" {
		sc.currentClass.HasMagicCallStatic = true
	}
}

func (sc *symbolCollector) processPropertyList(n *ast.StmtPropertyList) {
	if sc.currentClass == nil {
		return
	}

	typeFQN := ""
	if n.Type != nil {
		if resolved, ok := sc.resolvedNames[n.Type]; ok {
			typeFQN = resolved
		} else {
			typeFQN = VertexToName(n.Type)
		}
	}

	if typeFQN == "" {
		return
	}

	for _, prop := range n.Props {
		if p, ok := prop.(*ast.StmtProperty); ok {
			if v, ok := p.Var.(*ast.ExprVariable); ok {
				propName := VariableName(v)
				if propName != "" {
					sc.currentClass.Properties[propName] = typeFQN
				}
			}
		}
	}
}

func (sc *symbolCollector) processFunction(n *ast.StmtFunction) {
	fqn := sc.resolvedNames[n]
	if fqn == "" {
		if id, ok := n.Name.(*ast.Identifier); ok {
			fqn = string(id.Value)
		}
	}

	fi := &FunctionInfo{
		FQN:    fqn,
		Name:   fqn,
		Params: sc.extractParams(n.Params),
		File:   sc.file,
	}

	if n.ReturnType != nil {
		if resolved, ok := sc.resolvedNames[n.ReturnType]; ok {
			fi.ReturnType = resolved
		} else {
			fi.ReturnType = VertexToName(n.ReturnType)
		}
	}

	pos := n.GetPosition()
	if pos != nil {
		fi.Position = formatPosition(sc.file, pos.StartLine)
	}

	sc.st.Functions[fqn] = fi
}

func (sc *symbolCollector) extractParams(params []ast.Vertex) []ParamInfo {
	var result []ParamInfo
	for _, p := range params {
		param, ok := p.(*ast.Parameter)
		if !ok {
			continue
		}
		pi := ParamInfo{}
		if v, ok := param.Var.(*ast.ExprVariable); ok {
			pi.Name = VariableName(v)
		}
		if param.Type != nil {
			if resolved, ok := sc.resolvedNames[param.Type]; ok {
				pi.TypeFQN = resolved
			} else {
				pi.TypeFQN = VertexToName(param.Type)
			}
		}
		result = append(result, pi)
	}
	return result
}

// --- helpers ---

// VertexToName extracts a name string from a Name/Identifier AST node.
func VertexToName(v ast.Vertex) string {
	switch n := v.(type) {
	case *ast.Name:
		return concatNameParts(n.Parts)
	case *ast.NameFullyQualified:
		return concatNameParts(n.Parts)
	case *ast.NameRelative:
		return concatNameParts(n.Parts)
	case *ast.Identifier:
		return string(n.Value)
	case *ast.Nullable:
		return VertexToName(n.Expr)
	default:
		return ""
	}
}

func concatNameParts(parts []ast.Vertex) string {
	var sb strings.Builder
	for i, p := range parts {
		if np, ok := p.(*ast.NamePart); ok {
			if i > 0 {
				sb.WriteByte('\\')
			}
			sb.Write(np.Value)
		}
	}
	return sb.String()
}

// VariableName extracts the name from an ExprVariable node (without $).
func VariableName(v *ast.ExprVariable) string {
	if id, ok := v.Name.(*ast.Identifier); ok {
		return string(id.Value)
	}
	return ""
}

func anonymousName(file string, line int) string {
	return fmt.Sprintf("anonymous@%s:%d", filepath.Base(file), line)
}

func formatPosition(file string, line int) string {
	return fmt.Sprintf("%s:%d", filepath.Base(file), line)
}
