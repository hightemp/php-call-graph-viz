package callgraph

import (
	"strings"

	"github.com/VKCOM/php-parser/pkg/ast"
	"github.com/hightemp/php-call-graph-viz/internal/phpparse"
)

// TypeResolver provides heuristic type resolution for PHP variables.
// It tracks:
//   - $this → current class
//   - $var = new ClassName() → $var is ClassName
//   - Type-hinted parameters: function foo(UserRepo $repo) → $repo is UserRepo
//   - Type-hinted properties: private UserRepo $repo → $this->repo is UserRepo
//   - Manual overrides from config.yaml type_mappings
type TypeResolver struct {
	symbols       *phpparse.SymbolTable
	resolvedNames map[ast.Vertex]string
	typeMappings  map[string]string // from config

	// Scoped variable types within the current method.
	// Reset on each new method entry.
	varTypes map[string]string // variable name (without $) → FQN

	currentClassFQN string
}

// NewTypeResolver creates a resolver with the given context.
func NewTypeResolver(
	symbols *phpparse.SymbolTable,
	resolvedNames map[ast.Vertex]string,
	typeMappings map[string]string,
) *TypeResolver {
	return &TypeResolver{
		symbols:       symbols,
		resolvedNames: resolvedNames,
		typeMappings:  typeMappings,
		varTypes:      make(map[string]string),
	}
}

// EnterClass sets the current class context.
func (tr *TypeResolver) EnterClass(fqn string) {
	tr.currentClassFQN = fqn
}

// LeaveClass clears the current class context.
func (tr *TypeResolver) LeaveClass() {
	tr.currentClassFQN = ""
}

// EnterMethod resets the variable scope and seeds it with parameter types.
func (tr *TypeResolver) EnterMethod(params []phpparse.ParamInfo) {
	tr.varTypes = make(map[string]string)

	// Seed with parameter type hints.
	for _, p := range params {
		if p.TypeFQN != "" && p.Name != "" {
			tr.varTypes[p.Name] = p.TypeFQN
		}
	}
}

// TrackAssignment records a variable assignment like $var = new ClassName().
func (tr *TypeResolver) TrackAssignment(varName string, exprTypeFQN string) {
	if varName != "" && exprTypeFQN != "" {
		tr.varTypes[varName] = exprTypeFQN
	}
}

// ResolveVariable tries to determine the FQN type of a variable.
func (tr *TypeResolver) ResolveVariable(varName string) string {
	// 1. $this → current class
	if varName == "this" && tr.currentClassFQN != "" {
		return tr.currentClassFQN
	}

	// 2. Check manual config mappings.
	if fqn, ok := tr.typeMappings["$"+varName]; ok {
		return fqn
	}
	if tr.currentClassFQN != "" {
		key := tr.currentClassFQN + "::$" + varName
		if fqn, ok := tr.typeMappings[key]; ok {
			return fqn
		}
	}

	// 3. Check tracked variable assignments (new, parameter types).
	if fqn, ok := tr.varTypes[varName]; ok {
		return fqn
	}

	return ""
}

// ResolvePropertyFetch resolves $this->prop to the property type.
func (tr *TypeResolver) ResolvePropertyFetch(receiverVar string, propName string) string {
	// Only resolve for $this-> since we know the class.
	if receiverVar != "this" || tr.currentClassFQN == "" {
		// Try resolving receiver type first.
		receiverType := tr.ResolveVariable(receiverVar)
		if receiverType != "" {
			return tr.resolvePropertyType(receiverType, propName)
		}
		return ""
	}

	return tr.resolvePropertyType(tr.currentClassFQN, propName)
}

// resolvePropertyType looks up a property type in the class hierarchy.
func (tr *TypeResolver) resolvePropertyType(classFQN, propName string) string {
	// Check manual mappings.
	key := classFQN + "::$" + propName
	if fqn, ok := tr.typeMappings[key]; ok {
		return fqn
	}

	ci, ok := tr.symbols.Classes[classFQN]
	if !ok {
		return ""
	}

	// Check typed properties.
	if typeFQN, ok := ci.Properties[propName]; ok {
		return typeFQN
	}

	// Check constructor parameter promotion (PHP 8.0+).
	if ctor, ok := ci.Methods["__construct"]; ok {
		for _, p := range ctor.Params {
			if p.Name == propName && p.TypeFQN != "" {
				return p.TypeFQN
			}
		}
	}

	// Check parent class.
	if ci.Extends != "" {
		return tr.resolvePropertyType(ci.Extends, propName)
	}

	return ""
}

// ResolveSelf resolves "self" to the current class FQN.
func (tr *TypeResolver) ResolveSelf() string {
	return tr.currentClassFQN
}

// ResolveParent resolves "parent" to the parent class FQN.
func (tr *TypeResolver) ResolveParent() string {
	if tr.currentClassFQN == "" {
		return ""
	}
	ci, ok := tr.symbols.Classes[tr.currentClassFQN]
	if !ok || ci.Extends == "" {
		return ""
	}
	return ci.Extends
}

// ResolveStaticKeyword resolves the "static" keyword.
// Since this is late static binding, we return the current class
// but mark it as potentially polymorphic.
func (tr *TypeResolver) ResolveStaticKeyword() string {
	return tr.currentClassFQN
}

// ResolveNewExprType resolves the type for a `new ClassName()` expression.
func (tr *TypeResolver) ResolveNewExprType(classNode ast.Vertex) string {
	if resolved, ok := tr.resolvedNames[classNode]; ok {
		return tr.resolveSpecialKeyword(resolved)
	}
	return phpparse.VertexToName(classNode)
}

// resolveSpecialKeyword handles self/static/parent in resolved names.
func (tr *TypeResolver) resolveSpecialKeyword(name string) string {
	lower := strings.ToLower(name)
	switch lower {
	case "self":
		return tr.ResolveSelf()
	case "static":
		return tr.ResolveStaticKeyword()
	case "parent":
		return tr.ResolveParent()
	default:
		return name
	}
}
